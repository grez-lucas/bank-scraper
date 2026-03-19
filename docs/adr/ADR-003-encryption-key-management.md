# ADR-003: Encryption Key Management

### **Context** 💭
The Credential Manager stores bank credentials encrypted at rest (FR-1101). The PRD requires envelope encryption (FR-1103): a master key encrypts per-record data encryption keys (DEKs), which in turn encrypt the actual credential data. We need to decide where the master key lives.

### **Alternatives** ⚖️
1. **Environment variable:** Master key loaded from `ENCRYPTION_KEY` env var as a 64-character hex string (32 bytes). Simple, works for single-server deployment. DEKs stored encrypted in the database alongside credential records.
2. **File-based key:** Master key read from a file path (e.g., `/etc/bank-scraper/master.key`) with restricted filesystem permissions. Slightly better separation from environment.
3. **AWS KMS / HashiCorp Vault:** Integrate with a proper Key Management Service for key wrapping. Production-grade, supports key rotation, audit trails on key usage. Adds external infrastructure dependency.

### **Decision** 💪
We will use an **environment variable** (`ENCRYPTION_KEY`) for the master key in v1. The key is a 64-character hex string decoded to 32 bytes for AES-256-GCM. Per-record DEKs are generated with `crypto/rand`, encrypted with the master key, and stored alongside the encrypted data in PostgreSQL.

This is appropriate for our single-server, internal-only deployment. The crypto layer is designed with a `MasterKey` type abstraction, so migrating to KMS later requires changing only the key loading code, not the encryption logic.

### **Consequences**

✅ **Positivas:**
* Zero external dependencies — no KMS infrastructure to manage
* Simple deployment — just set an env var
* Envelope encryption still provides per-record key isolation (compromising one DEK doesn't expose other records)
* `MasterKey` type abstraction makes future KMS migration straightforward

❌ **Negativas:**
* Master key visible in process environment (`/proc/<pid>/environ` on Linux)
* No built-in key rotation — rotating the master key requires re-encrypting all DEKs
* No audit trail on key access (unlike KMS)
* Operator must generate and securely distribute the key manually
