# ADR-004: Session Storage for Credential Manager

### **Context** 💭
The Credential Manager requires authenticated sessions with a 15-minute inactivity timeout (FR-1006). Sessions must be invalidatable (for logout and lockout), auditable, and support tracking IP address and user agent. We need to decide where session state lives.

### **Alternatives** ⚖️
1. **Database-backed sessions (PostgreSQL):** Store sessions in a `sessions` table. Session token is a `crypto/rand` 32-byte value; only the SHA-256 hash is stored. Expired sessions cleaned up periodically.
2. **Cookie-only (signed + encrypted):** All session state stored in the cookie itself. No server-side state. Uses AES-GCM encryption with HMAC signing.
3. **Redis:** In-memory session store with native TTL support. Fast lookups, automatic expiry.

### **Decision** 💪
We will use **database-backed sessions in PostgreSQL**. Session tokens are generated with `crypto/rand` (32 bytes), and only the SHA-256 hash is stored in the `sessions` table. The table tracks `user_id`, `ip_address`, `user_agent`, `last_active`, and `expires_at`. Expired sessions are cleaned up by a periodic `DELETE WHERE expires_at < now()` query.

### **Consequences**

✅ **Positivas:**
* No additional infrastructure — reuses the existing PostgreSQL instance
* Sessions can be explicitly invalidated (logout, lockout) by deleting the row
* Full audit trail — session metadata (IP, user agent, last active) stored for compliance
* Consistent with the rest of the data layer (same pool, same patterns)

❌ **Negativas:**
* Slightly higher latency than Redis for session lookups (mitigated by index on `token_hash`)
* Requires periodic cleanup of expired sessions (simple cron query, not a major burden)
* Database load increases with every request (session touch updates `last_active`)
