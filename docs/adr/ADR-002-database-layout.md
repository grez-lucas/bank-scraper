# ADR-002: Shared Database Layer

### **Context** 💭
The platform has two modules that need database access: the Credential Manager (users, sessions, credentials, audit logs) and the future API Gateway (accounts, cached balances, health status). Both connect to the same PostgreSQL instance. We need to decide whether to share a single database package or let each module own its own.

### **Alternatives** ⚖️
1. **Shared `internal/store/` package:** Single store package with repository interfaces. Both Credential Manager and API Gateway share the same connection pool, migration directory, and repository pattern.
2. **Self-contained `internal/credmgr/store/`:** Credential Manager owns its own database layer. More isolated, but duplicates migration infrastructure and connection management when the API Gateway is added later.

### **Decision** 💪
We will use a **shared `internal/store/` package**. Both modules will share the same `pgxpool.Pool`, migration directory (`internal/store/migrations/`), and repository pattern. This matches the CLAUDE.md project structure and avoids duplication when the API Gateway is built.

### **Consequences**

✅ **Positivas:**
* Single migration directory — one source of truth for schema
* Shared connection pool reduces resource usage
* Repository interfaces can be reused across modules
* Consistent patterns and conventions across the codebase

❌ **Negativas:**
* Tighter coupling between modules at the database layer
* Schema changes for one module can affect the other's migration ordering
* Must be careful about transaction boundaries across module concerns
