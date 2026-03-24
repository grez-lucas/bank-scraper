# TODO: API Gateway Epic

> Exposes bank scraper data to AyniFX via a REST API with API key auth, account discovery, session management, and resilience.

## Architecture Decision Records

- [x] ADR-010: API Key Authentication (`docs/adr/ADR-010-api-key-authentication.md`)
- [x] ADR-011: Account Discovery and Persistence (`docs/adr/ADR-011-account-discovery.md`)
- [x] ADR-012: Scraper Session Management — Lazy Singleton (`docs/adr/ADR-012-scraper-session-management.md`)
- [x] ADR-013: Scraper Interface — Generic Credentials (`docs/adr/ADR-013-scraper-interface-generic-credentials.md`)
- [x] ADR-014: Resilience — Retry and Circuit Breaker (`docs/adr/ADR-014-resilience-retry-circuit-breaker.md`)

---

## M1 — Database: Accounts + API Keys ✅

- [x] Migration 000006: `accounts` table (id, bank_code, account_number, currency, account_type, status, credential_id FK, last_synced_at)
- [x] Migration 000007: `api_keys` table (id, key_hash, client_id, description, created_at, revoked_at, last_used_at)
- [x] `internal/store/account.go` — AccountRepository (Create, GetByID, List with filters, UpsertBatch, UpdateLastSynced)
- [x] `internal/store/apikey.go` — APIKeyRepository (Create, GetByKeyHash, Revoke, UpdateLastUsed, List)
- [x] Integration tests for both repositories (17 tests, all pass)

## M2 — Scraper Interface Redesign ✅

- [x] Redesign `bank.Scraper` interface: Login(ctx, map[string]string), GetBalance(ctx), GetTransactions(ctx, accountID, count), Logout(ctx), Close()
- [x] Adapt BBVA scraper Login to accept `map[string]string` (maps to internal `credentials` struct)
- [x] Add compile-time interface check: `var _ bank.Scraper = (*Scraper)(nil)`
- [x] Update `bbva_tester.go` — pass fields map directly (removed `bbva.Credentials` dependency)
- [x] Update all scraper tests — 15 calls migrated from struct to map
- [x] Verify: full project compiles, all parser + credmgr tests pass

## M3 — Session Manager (Lazy Singleton) ✅

- [x] `internal/api/session/manager.go` — Manager with GetScraper, Invalidate, Shutdown, SessionStatus
- [x] CredentialProvider interface (satisfied by CredentialService.GetCredentials)
- [x] ScraperFactory type (BBVA only for now)
- [x] Unit tests with mock scraper + mock credential provider (10 tests, all pass)

## M4 — API Key Middleware ✅

- [x] `internal/api/middleware/apikey.go` — Gin middleware: X-API-Key header → SHA-256 → DB lookup → revocation check → context injection
- [x] `GetClientID(c)` helper to retrieve client_id from Gin context
- [x] Unit tests: valid key, missing header, invalid key, revoked key, GetClientID (5 tests, all pass)
- [x] CLI command: `api create-key --client-id=<id> --description=<desc>`

## M5 — Account Discovery Service ✅

- [x] `internal/api/service/discovery.go` — DiscoveryService.Discover (dedicated scraper lifecycle: create → login → GetBalance → map → UpsertBatch → logout → close)
- [x] `balancesToAccounts()` helper maps `[]bank.Balance` → `[]store.Account`
- [x] Unit tests with mock scraper + mock account repo (6 tests, all pass)
- [ ] DiscoveryTrigger wiring into credmgr handler (follow-up: requires modifying credmgr handler constructor)

## M6 — API Handlers + Router ✅

- [x] `internal/api/handler/response.go` — ErrorJSON, FormatAmount, MaskAccountNumber helpers + all response DTOs
- [x] `internal/api/handler/account.go` — GET /api/v1/accounts (DB query, masked account numbers) — 4 tests
- [x] `internal/api/handler/health.go` — GET /api/v1/health (per-bank status, DB ping, no scraping) — 4 tests
- [x] `internal/api/handler/balance.go` — GET /api/v1/accounts/:account_id/balance — 5 tests
- [x] `internal/api/handler/transaction.go` — GET /api/v1/accounts/:account_id/transactions (date filtering, pagination) — 5 tests
- [x] `internal/api/handler/discovery.go` — POST /api/v1/admin/discover/:bank_code — 3 tests
- [x] `internal/api/router.go` — Gin router with API key middleware, all 5 routes
- [x] All 23 handler tests + 2 helper tests pass

## M7 — Retry + Circuit Breaker ✅

- [x] `internal/api/resilience/retry.go` — Generic `Retry[T]` with exponential backoff (3 attempts, 1s→30s) + `IsRetryable` error classification
- [x] `internal/api/resilience/circuitbreaker.go` — `BreakerRegistry` (per-bank gobreaker, 5 failures → open, 5min reset) + `ResilientProvider` wrapper
- [x] Integration: `ResilientProvider` wraps `ScraperProvider` transparently — handlers get resilience without code changes
- [x] Unit tests: error classification (9 cases), retry (5 tests), circuit breaker (2 tests), provider (2 tests) — 9 total, all pass

## M8 — API Entrypoint + Integration ✅

- [x] `cmd/api/main.go` — CLI: serve, create-key, discover, migrate, migrate-down, version
- [x] Config additions: RetryMaxAttempts, RetryInitialDelay, RetryMaxDelay, CBMaxFailures, CBResetTimeout
- [x] Wiring: config → DB → repos → credential service → scraper factory → session manager → resilient provider → handlers → router
- [x] Graceful shutdown: SIGINT/SIGTERM → session manager shutdown → HTTP server shutdown → DB close
- [x] Makefile targets: api-serve, api-create-key, api-discover + build target includes api binary
- [ ] Manual E2E test against live BBVA (separate session)

---

## Dependency Graph

```
M1 (DB) ──────────┬──── M4 (API key middleware)
                   │
M2 (Interface) ──┬── M3 (Session manager)
                  ├── M5 (Discovery)
                  └─────── M6 (Handlers) ← M1, M3, M4, M5
                                │
                          M7 (Resilience)
                                │
                          M8 (Integration)
```

Parallelizable: M1 ∥ M2, then M3 ∥ M4 ∥ M5.
