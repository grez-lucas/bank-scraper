// Package crypto provides AES-256-GCM envelope encryption for the credential manager.
//
// Envelope encryption uses two layers of keys:
//   - A master key (MasterKey) loaded from the ENCRYPTION_KEY environment variable
//   - Per-record data encryption keys (DEKs) generated randomly for each encrypted value
//
// The DEK encrypts the actual data. The master key encrypts the DEK. Both the
// encrypted data and encrypted DEK are stored together in the database. This
// isolates records — compromising one DEK doesn't expose other records.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
)

const keySize = 32 // AES-256

// MasterKey is a 32-byte AES-256 key used to encrypt/decrypt DEKs.
type MasterKey [keySize]byte

// ParseMasterKey decodes a 64-character hex string into a MasterKey.
func ParseMasterKey(hexKey string) (MasterKey, error) {
	if len(hexKey) != keySize*2 {
		return MasterKey{}, fmt.Errorf("master key must be 64 hex characters (32 bytes), got %d", len(hexKey))
	}
	decoded, err := hex.DecodeString(hexKey)
	if err != nil {
		return MasterKey{}, fmt.Errorf("decode hex master key: %w", err)
	}
	var mk MasterKey
	copy(mk[:], decoded)
	return mk, nil
}

// GenerateDEK creates a random 32-byte data encryption key.
func GenerateDEK() ([]byte, error) {
	dek := make([]byte, keySize)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		return nil, fmt.Errorf("generate DEK: %w", err)
	}
	return dek, nil
}

// EncryptDEK encrypts a DEK with the master key using AES-256-GCM.
func EncryptDEK(mk MasterKey, dek []byte) ([]byte, error) {
	return encrypt(mk[:], dek)
}

// DecryptDEK decrypts a DEK with the master key.
func DecryptDEK(mk MasterKey, encryptedDEK []byte) ([]byte, error) {
	return decrypt(mk[:], encryptedDEK)
}

// encrypt encrypts plaintext with a key using AES-256-GCM.
// The nonce is prepended to the ciphertext: [nonce (12 bytes) | ciphertext + tag].
func encrypt(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	// Seal appends ciphertext+tag to nonce
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// decrypt decrypts ciphertext produced by encrypt.
// Expects format: [nonce (12 bytes) | ciphertext + tag].
func decrypt(key, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}
	if len(ciphertext) < gcm.NonceSize()+gcm.Overhead() {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce := ciphertext[:gcm.NonceSize()]
	encrypted := ciphertext[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, encrypted, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return plaintext, nil
}

// Seal is a convenience function that generates a new DEK, encrypts the
// plaintext with it, then encrypts the DEK with the master key.
// Returns (encryptedData, encryptedDEK, error).
func Seal(mk MasterKey, plaintext []byte) (encData, encDEK []byte, err error) {
	dek, err := GenerateDEK()
	if err != nil {
		return nil, nil, err
	}
	defer zeroBytes(dek)

	encData, err = encrypt(dek, plaintext)
	if err != nil {
		return nil, nil, fmt.Errorf("encrypt data: %w", err)
	}
	encDEK, err = EncryptDEK(mk, dek)
	if err != nil {
		return nil, nil, fmt.Errorf("encrypt DEK: %w", err)
	}
	return encData, encDEK, nil
}

// Open is a convenience function that decrypts the DEK with the master key,
// then decrypts the data with the DEK.
func Open(mk MasterKey, encData, encDEK []byte) ([]byte, error) {
	dek, err := DecryptDEK(mk, encDEK)
	if err != nil {
		return nil, fmt.Errorf("decrypt DEK: %w", err)
	}
	defer zeroBytes(dek)

	plaintext, err := decrypt(dek, encData)
	if err != nil {
		return nil, fmt.Errorf("decrypt data: %w", err)
	}
	return plaintext, nil
}

// zeroBytes overwrites a byte slice with zeros to limit key exposure in memory.
func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
