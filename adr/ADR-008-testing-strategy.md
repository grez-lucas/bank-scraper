# ADR-008: Testing Strategy — TDD for Logic, Integration for Infrastructure

### **Context** 💭
The Credential Manager has distinct layers: pure logic (crypto, auth service, credential service), infrastructure (database repositories, HTTP handlers), and integration points (scraper connection). We need a testing strategy that gives confidence without over-investing in mock infrastructure. The scraper codebase already uses testify with table-driven tests.

### **Alternatives** ⚖️
1. **TDD everywhere with mocks:** Write tests first for all layers. Mock database repositories when testing services. Mock services when testing handlers. Maximum isolation but high mock maintenance cost.
2. **TDD for logic, integration for infrastructure:** Write tests first for pure logic (crypto, services). Test repositories and handlers against real PostgreSQL (via Docker Compose). No mock repositories.
3. **Integration tests only:** Write all tests after implementation, all against real infrastructure. Fastest to write but misses the design benefits of TDD for complex logic.

### **Decision** 💪
We will use **TDD for logic layers and integration tests for infrastructure**:

- **TDD (write tests first):** Crypto functions (`Seal`/`Open`, key parsing), auth service (login flow, lockout, session validation), credential service (CRUD orchestration, encryption workflow). These are pure logic that can be tested with in-memory fakes or simple stubs.
- **Integration tests (after implementation):** Repository layer tested against a real PostgreSQL instance (Docker Compose). HTTP handlers tested against a running Gin server with a test database. No mock repositories.

### **Consequences**

✅ **Positivas:**
* TDD drives clean API design for the logic layers (crypto, services)
* No mock repository maintenance — repository tests hit real Postgres, catching SQL bugs
* Integration tests for handlers catch real routing, middleware, and serialization issues
* Matches existing project patterns (scraper tests use real browser via replay mode)

❌ **Negativas:**
* Repository and handler tests require Docker Compose Postgres running — slower CI setup
* Service TDD tests need simple stubs/fakes for repository interfaces (lighter than full mocks but still some overhead)
* Cannot run the full test suite without Docker — `go test ./...` will skip DB-dependent tests without Postgres
