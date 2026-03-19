# ADR-009: Development Infrastructure — Docker Compose + Dockerfile

### **Context** 💭
The Credential Manager requires PostgreSQL as a runtime dependency. Developers need a consistent way to run Postgres locally, and the application needs to be deployable as a container. The scraper codebase currently has no Docker infrastructure — it runs locally with a `.env` file and `godotenv`.

### **Alternatives** ⚖️
1. **Docker Compose + Dockerfile:** `docker-compose.yml` defines PostgreSQL for development. A multi-stage `Dockerfile` builds the `credmgr` binary into a minimal Alpine image. Developers run `docker compose up -d` for the database and `go run` for the app.
2. **Local PostgreSQL only:** Developers install Postgres natively. No Docker. Simpler for developers who already have Postgres, but inconsistent versions and configurations across machines.
3. **Full Docker stack:** Both the application and database run in Docker Compose. Developers don't run `go run` locally — everything is containerized. Slower development loop (rebuild image on every change).

### **Decision** 💪
We will use **Docker Compose for PostgreSQL** (development dependency) and a **multi-stage Dockerfile** for the application image. Developers run `docker compose up -d` to start Postgres, then `go run ./cmd/credmgr serve` locally for fast iteration. The Dockerfile is used for CI and production deployment.

### **Consequences**

✅ **Positivas:**
* Consistent PostgreSQL version across all developer machines
* Fast development loop — `go run` locally, no container rebuild needed
* Dockerfile enables CI/CD and production deployment as a single binary image
* Integration tests can use the same Docker Compose Postgres

❌ **Negativas:**
* Requires Docker installed on developer machines
* Two-step startup: `docker compose up -d` then `go run` (vs. single `docker compose up` for full stack)
* Dockerfile must be kept in sync with Go version and build flags
