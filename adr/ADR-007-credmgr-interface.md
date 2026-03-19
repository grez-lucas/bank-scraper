# ADR-007: Credential Manager Interface — Full Web UI

### **Context** 💭
The Credential Manager needs a user-facing interface for C-level executives to manage bank credentials. The PRD specifies a web UI with login, CRUD forms, and audit log viewing. However, we could start with a simpler interface and layer the web UI on top later. The target audience is 3-4 non-technical users who need a secure, straightforward experience.

### **Alternatives** ⚖️
1. **Full web UI (PRD spec):** Go HTML templates served by Gin. Login page with 2FA, credential CRUD forms, audit log viewer with filtering and CSV/JSON export. Matches the PRD exactly.
2. **CLI tool first:** Command-line tool for credential CRUD operations. Simpler to build, still requires the full crypto + DB + audit stack. Web UI added as a later milestone.
3. **REST API first:** Build the Credential Manager as a protected REST API. Web UI or CLI can be layered on top afterward. Most flexible but requires building a client anyway.

### **Decision** 💪
We will build the **full web UI** from the start, using Go HTML templates rendered by Gin. This includes a login flow (username/password → TOTP), credential management forms, and an audit log viewer with export.

### **Consequences**

✅ **Positivas:**
* Matches the PRD specification exactly — no compromises on requirements
* Accessible to non-technical C-level users who need to manage credentials
* Self-contained — no separate frontend build step or SPA framework needed
* Go templates are fast and server-rendered — no JavaScript complexity

❌ **Negativas:**
* More upfront work than CLI or API-first approaches
* Go HTML templates are less flexible than a modern SPA for complex interactions
* Template testing requires HTTP-level integration tests (not pure unit tests)
* Credential test operation (browser automation, 30-60s) needs special UX handling (loading state)
