package crypto

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMasterKey(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errMsg  string
	}{
		{
			name:  "valid 64-char hex",
			input: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		},
		{
			name:    "too short",
			input:   "0123456789abcdef",
			wantErr: true,
			errMsg:  "must be 64 hex characters",
		},
		{
			name:    "too long",
			input:   "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef00",
			wantErr: true,
			errMsg:  "must be 64 hex characters",
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
			errMsg:  "must be 64 hex characters",
		},
		{
			name:    "invalid hex characters",
			input:   "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz",
			wantErr: true,
			errMsg:  "decode hex",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mk, err := ParseMasterKey(tc.input)
			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errMsg)
				assert.Equal(t, MasterKey{}, mk)
			} else {
				require.NoError(t, err)
				// Verify the parsed key matches the hex input
				assert.Equal(t, tc.input, hex.EncodeToString(mk[:]))
			}
		})
	}
}

func TestGenerateDEK(t *testing.T) {
	dek1, err := GenerateDEK()
	require.NoError(t, err)
	assert.Len(t, dek1, 32, "DEK should be 32 bytes")

	dek2, err := GenerateDEK()
	require.NoError(t, err)
	assert.Len(t, dek2, 32, "DEK should be 32 bytes")

	assert.False(t, bytes.Equal(dek1, dek2), "two generated DEKs should differ")
}

func TestEncryptDecryptDEK(t *testing.T) {
	mk := testMasterKey(t)
	dek, err := GenerateDEK()
	require.NoError(t, err)

	encrypted, err := EncryptDEK(mk, dek)
	require.NoError(t, err)
	assert.NotEmpty(t, encrypted)
	assert.False(t, bytes.Equal(dek, encrypted), "encrypted DEK should differ from plaintext")

	decrypted, err := DecryptDEK(mk, encrypted)
	require.NoError(t, err)
	assert.Equal(t, dek, decrypted)
}

func TestDecryptDEK_WrongMasterKey(t *testing.T) {
	mk1 := testMasterKey(t)
	mk2 := testMasterKeyAlt(t)

	dek, err := GenerateDEK()
	require.NoError(t, err)

	encrypted, err := EncryptDEK(mk1, dek)
	require.NoError(t, err)

	_, err = DecryptDEK(mk2, encrypted)
	require.Error(t, err, "decrypting with wrong master key should fail")
}

func TestDecryptDEK_TamperedCiphertext(t *testing.T) {
	mk := testMasterKey(t)
	dek, err := GenerateDEK()
	require.NoError(t, err)

	encrypted, err := EncryptDEK(mk, dek)
	require.NoError(t, err)

	// Flip a byte in the ciphertext
	encrypted[len(encrypted)-1] ^= 0xff

	_, err = DecryptDEK(mk, encrypted)
	require.Error(t, err, "tampered ciphertext should fail GCM authentication")
}

func TestEncryptDecrypt(t *testing.T) {
	dek, err := GenerateDEK()
	require.NoError(t, err)

	plaintext := []byte("secret bank credentials JSON blob")
	ciphertext, err := encrypt(dek,plaintext)
	require.NoError(t, err)
	assert.NotEmpty(t, ciphertext)
	assert.False(t, bytes.Equal(plaintext, ciphertext))

	decrypted, err := decrypt(dek, ciphertext)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)
}

func TestDecrypt_TamperedCiphertext(t *testing.T) {
	dek, err := GenerateDEK()
	require.NoError(t, err)

	ciphertext, err := encrypt(dek,[]byte("hello"))
	require.NoError(t, err)

	ciphertext[len(ciphertext)-1] ^= 0xff

	_, err = decrypt(dek, ciphertext)
	require.Error(t, err, "tampered ciphertext should fail GCM authentication")
}

func TestDecrypt_WrongKey(t *testing.T) {
	dek1, err := GenerateDEK()
	require.NoError(t, err)
	dek2, err := GenerateDEK()
	require.NoError(t, err)

	ciphertext, err := encrypt(dek1, []byte("hello"))
	require.NoError(t, err)

	_, err = decrypt(dek2, ciphertext)
	require.Error(t, err, "decrypting with wrong key should fail")
}

func TestSealOpen_RoundTrip(t *testing.T) {
	mk := testMasterKey(t)
	plaintext := []byte(`{"company_code":"12345","user":"admin","password":"secret"}`)

	encData, encDEK, err := Seal(mk, plaintext)
	require.NoError(t, err)
	assert.NotEmpty(t, encData)
	assert.NotEmpty(t, encDEK)

	decrypted, err := Open(mk, encData, encDEK)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)
}

func TestSealOpen_ZeroLengthPlaintext(t *testing.T) {
	mk := testMasterKey(t)

	encData, encDEK, err := Seal(mk, []byte{})
	require.NoError(t, err)

	decrypted, err := Open(mk, encData, encDEK)
	require.NoError(t, err)
	assert.Empty(t, decrypted)
}

func TestOpen_WrongMasterKey(t *testing.T) {
	mk1 := testMasterKey(t)
	mk2 := testMasterKeyAlt(t)

	encData, encDEK, err := Seal(mk1, []byte("secret"))
	require.NoError(t, err)

	_, err = Open(mk2, encData, encDEK)
	require.Error(t, err, "opening with wrong master key should fail")
}

func TestOpen_TamperedData(t *testing.T) {
	mk := testMasterKey(t)

	encData, encDEK, err := Seal(mk, []byte("secret"))
	require.NoError(t, err)

	// Tamper with the encrypted data
	encData[len(encData)-1] ^= 0xff

	_, err = Open(mk, encData, encDEK)
	require.Error(t, err, "opening tampered data should fail")
}

func TestOpen_TamperedDEK(t *testing.T) {
	mk := testMasterKey(t)

	encData, encDEK, err := Seal(mk, []byte("secret"))
	require.NoError(t, err)

	// Tamper with the encrypted DEK
	encDEK[len(encDEK)-1] ^= 0xff

	_, err = Open(mk, encData, encDEK)
	require.Error(t, err, "opening with tampered DEK should fail")
}

func TestSeal_ProducesDifferentOutputEachCall(t *testing.T) {
	mk := testMasterKey(t)
	plaintext := []byte("same input")

	encData1, encDEK1, err := Seal(mk, plaintext)
	require.NoError(t, err)

	encData2, encDEK2, err := Seal(mk, plaintext)
	require.NoError(t, err)

	// Different DEKs each time
	assert.False(t, bytes.Equal(encDEK1, encDEK2), "each Seal should use a new DEK")
	// Different ciphertext (different DEK + different nonce)
	assert.False(t, bytes.Equal(encData1, encData2), "ciphertext should differ across calls")
}

// --- test helpers ---

func testMasterKey(t *testing.T) MasterKey {
	t.Helper()
	mk, err := ParseMasterKey("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	require.NoError(t, err)
	return mk
}

func testMasterKeyAlt(t *testing.T) MasterKey {
	t.Helper()
	mk, err := ParseMasterKey("fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210")
	require.NoError(t, err)
	return mk
}
