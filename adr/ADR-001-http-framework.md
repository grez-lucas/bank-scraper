# ADR-001: HTTP Framework Selection

### **Context** 💭
The bank scraper platform needs an HTTP framework for two modules: the Credential Manager (web UI) and the future API Gateway (REST API). The CLAUDE.md originally specified Echo, but no HTTP code has been written yet, making this the right time to evaluate options. The framework will be used for routing, middleware, template rendering, and request handling across the entire application.

### **Alternatives** ⚖️
1. **Echo (`github.com/labstack/echo/v4`):** Originally specified in CLAUDE.md. Full-featured framework with its own `echo.Context` abstraction. Opinionated, wraps `net/http` entirely.
2. **Chi (`github.com/go-chi/chi/v5`):** Thin router on top of `net/http`. Handlers are plain `http.HandlerFunc`, middleware is standard `net/http` middleware. Most idiomatic option.
3. **Gin (`github.com/gin-gonic/gin`):** Widely adopted, high-performance router with built-in middleware ecosystem (recovery, logging, CORS). Uses its own `gin.Context` but has strong community support, built-in template rendering, and extensive middleware libraries (sessions, CSRF).

### **Decision** 💪
We will use **Gin** (`github.com/gin-gonic/gin`). While Chi is more idiomatic (stdlib compatible), Gin offers a richer built-in middleware ecosystem that accelerates development of the Credential Manager's web UI (session management, CSRF, template rendering). The team is comfortable with Gin's `gin.Context` pattern.

### **Consequences**

✅ **Positivas:**
* Large ecosystem of middleware plugins (`gin-contrib/sessions`, CSRF, etc.)
* Built-in HTML template rendering support
* High performance router with radix tree
* Extensive documentation and community resources
* Built-in recovery and logging middleware

❌ **Negativas:**
* `gin.Context` is not stdlib `net/http` compatible — handlers are not portable to other routers without adaptation
* Slightly more opinionated than Chi — framework lock-in for handler signatures
* CLAUDE.md references Echo patterns that will need to be updated
