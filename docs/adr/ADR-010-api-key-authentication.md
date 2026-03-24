# ADR-010: API Key Authentication

### **Context** 💭

The API Gateway needs to authenticate incoming requests from AyniFX (PRD FR-101 through FR-106). Currently there is only one consumer, but the design must support key rotation without downtime (FR-105) and associate each key with a client identifier for audit purposes (FR-106). OAuth 2.0/JWT is explicitly out of scope (PRD NG-02).

### **Alternatives** ⚖️

1. **Environment variable:** Single API key stored in config. Simple to implement but requires a service restart to rotate keys. Cannot support multiple active keys during rotation windows. No built-in audit trail for which key was used.
2. **Database table with hashed keys:** A dedicated `api_keys` table stores SHA-256 hashed keys with `client_id`, `created_at`, `revoked_at`, and `last_used_at`. Supports zero-downtime rotation by creating a new key before revoking the old one. Each request is attributable to a client.
3. **OAuth 2.0 / JWT:** Industry standard for multi-tenant APIs. Significant implementation overhead (token issuance, refresh, validation). Explicitly out of scope per PRD NG-02 since there is only one consumer.

### **Decision** 💪

We will use a **database-backed `api_keys` table** with SHA-256 hashed keys. Keys are transmitted via the `X-API-Key` HTTP header. A Gin middleware extracts the header, hashes it, and looks it up in the database. Revoked keys are rejected. The `client_id` is injected into the request context for downstream audit logging.

Key management is handled via a CLI command (`api create-key --client-id=aynifx`) that generates a cryptographically random 32-byte key, stores the hash, and prints the raw key exactly once.

### **Consequences**

✅ **Positivas:**

* Zero-downtime key rotation — create new key, migrate consumer, revoke old key
* Full audit trail — `client_id` + `last_used_at` per key
* Simple implementation — SHA-256 hash comparison, no token parsing or crypto verification
* Extensible — easy to add more consumers or rate limits per key in the future

❌ **Negativas:**

* Database lookup on every request (mitigable with caching if needed, but unnecessary at current scale)
* Raw key is only shown once at creation — if lost, must generate a new one
* No expiration-based rotation — must be manually revoked (acceptable for single-consumer scenario)
