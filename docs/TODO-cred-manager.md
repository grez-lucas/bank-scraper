# TODO: Credential Manager Module

## Overview

Web UI for C-level executives to securely manage bank credentials with TOTP 2FA. Lives in the same monolith as the scraper engine, shares the database layer.

**PRD Reference:** Section 4.3 (FR-1001 through FR-1307)

## Architectural Decisions

| Decision | Choice |
|----------|--------|
| Router | `github.com/gin-gonic/gin` |
| Interface | Full web UI (Go HTML templates) |
| DB layout | Shared `internal/store/` (reused by future API gateway) |
| User seeding | CLI `seed-admin` command (interactive, prints TOTP QR) |
| Encryption | Env var master key (`ENCRYPTION_KEY`), envelope encryption (AES-256-GCM), per-record DEKs |
| Sessions | DB-backed in PostgreSQL, 15min inactivity timeout |
| Credential testing | Wired in v1 — calls scraper `Login()` / `Logout()` (FR-1204) |
| Migrations | `golang-migrate/migrate`, embedded SQL files |
| Infrastructure | Docker Compose (PostgreSQL), Dockerfile (multi-stage build) |

## Testing Strategy

- **TDD (tests first):** Crypto layer, auth service, credential service — pure logic, no DB
- **Integration tests (after impl):** Repositories (against Docker Compose Postgres), HTTP handlers (against running Gin + test DB)
- **No mock repositories** — test logic in isolation, test infra against real Postgres

## Dependency Graph

```
M0: Foundation (Docker Compose, config, DB, migrations)
 |
 +-- M1: Crypto (parallel, no DB dependency)
 |
 +-- M2: User + Session repos
 |     |
 |     +-- M3: Auth service (depends on M1 + M2)
 |
 +-- M4: Audit + Credential repos (parallel with M2/M3)
       |
       +-- M5: Credential service (depends on M1 + M4)
             |
             +-- M6: HTTP layer (depends on M3 + M5)
                   |
                   +-- M7: CLI + integration (depends on M6)
```

Critical path: M0 → M2 → M3 → M6 → M7

---

## M0: Foundation — Config + Docker + DB + Migrations

**Status:** DONE

### Deliverables

- `docker-compose.yml` — PostgreSQL 15 for dev
- `Dockerfile` — Multi-stage build (builder + runtime)
- `internal/config/config.go` — Shared config struct via `envconfig`
- `internal/store/store.go` — pgxpool wrapper (connect, close, health check)
- `internal/store/migrations/` — 4 SQL migration pairs (up/down)
- Makefile targets: `make db-up`, `make db-down`, `make migrate`, `make migrate-down`

### Config Struct

```go
type Config struct {
    DatabaseURL   string        `envconfig:"DATABASE_URL" required:"true"`
    EncryptionKey string        `envconfig:"ENCRYPTION_KEY" required:"true"` // 64-char hex → 32 bytes
    CredMgrPort   int           `envconfig:"CREDMGR_PORT" default:"8081"`
    APIPort       int           `envconfig:"API_PORT" default:"8080"`
    SessionTTL    time.Duration `envconfig:"SESSION_TTL" default:"15m"`
}
```

### Migrations

**000001_create_users.up.sql**
```sql
CREATE TABLE users (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username        VARCHAR(50) UNIQUE NOT NULL,
    password_hash   TEXT NOT NULL,
    totp_secret_enc BYTEA NOT NULL,
    totp_secret_dek BYTEA NOT NULL,
    is_active       BOOLEAN NOT NULL DEFAULT true,
    failed_attempts INT NOT NULL DEFAULT 0,
    locked_until    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

**000002_create_sessions.up.sql**
```sql
CREATE TABLE sessions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id),
    token_hash  TEXT UNIQUE NOT NULL,
    ip_address  INET NOT NULL,
    user_agent  TEXT NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    last_active TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_sessions_token_hash ON sessions(token_hash);
CREATE INDEX idx_sessions_expires_at ON sessions(expires_at);
```

**000003_create_bank_credentials.up.sql**
```sql
CREATE TABLE bank_credentials (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    bank_code       VARCHAR(20) NOT NULL,
    account_label   VARCHAR(100) NOT NULL,
    credentials_enc BYTEA NOT NULL,
    credentials_dek BYTEA NOT NULL,
    version         INT NOT NULL DEFAULT 1,
    status          VARCHAR(20) NOT NULL DEFAULT 'active',
    deleted_at      TIMESTAMPTZ,
    created_by      UUID NOT NULL REFERENCES users(id),
    updated_by      UUID NOT NULL REFERENCES users(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

**000004_create_audit_logs.up.sql**
```sql
CREATE TABLE audit_logs (
    id          BIGSERIAL PRIMARY KEY,
    timestamp   TIMESTAMPTZ NOT NULL DEFAULT now(),
    user_id     UUID REFERENCES users(id),
    action      VARCHAR(50) NOT NULL,
    target_type VARCHAR(50),
    target_id   VARCHAR(100),
    ip_address  INET NOT NULL,
    user_agent  TEXT NOT NULL,
    details     JSONB,
    success     BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_audit_logs_timestamp ON audit_logs(timestamp);
CREATE INDEX idx_audit_logs_user_id ON audit_logs(user_id);
CREATE INDEX idx_audit_logs_action ON audit_logs(action);

-- Immutable: prevent UPDATE and DELETE (FR-1303)
CREATE RULE audit_logs_no_update AS ON UPDATE TO audit_logs DO INSTEAD NOTHING;
CREATE RULE audit_logs_no_delete AS ON DELETE TO audit_logs DO INSTEAD NOTHING;
```

### New Dependencies

```
github.com/gin-gonic/gin
github.com/jackc/pgx/v5
github.com/golang-migrate/migrate/v4
github.com/kelseyhightower/envconfig
github.com/pquerna/otp
golang.org/x/crypto
```

### Acceptance Criteria

- [x] `docker compose up -d` starts PostgreSQL
- [x] Config loads from env vars with defaults
- [x] `go run ./cmd/credmgr migrate` creates all 4 tables (users, sessions, bank_credentials, audit_logs)
- [x] `go run ./cmd/credmgr migrate-down` rolls back cleanly
- [x] `go build ./...` compiles
- [x] `go vet ./...` passes
- [x] Existing scraper tests still pass

---

## M1: Crypto Layer — Envelope Encryption

**Status:** DONE

### Deliverables

- `internal/credmgr/crypto/crypto.go` — AES-256-GCM envelope encryption
- `internal/credmgr/crypto/crypto_test.go` — TDD tests (written first)

### API

```go
type MasterKey [32]byte

func ParseMasterKey(hex string) (MasterKey, error)
func GenerateDEK() ([]byte, error)
func EncryptDEK(mk MasterKey, dek []byte) ([]byte, error)
func DecryptDEK(mk MasterKey, encryptedDEK []byte) ([]byte, error)
func Encrypt(dek, plaintext []byte) ([]byte, error)
func Decrypt(dek, ciphertext []byte) ([]byte, error)

// Convenience wrappers
func Seal(mk MasterKey, plaintext []byte) (encData, encDEK []byte, err error)
func Open(mk MasterKey, encData, encDEK []byte) ([]byte, error)
```

### TDD Test Cases (write first)

- [x] Round-trip: `Seal` then `Open` returns original plaintext
- [x] Wrong master key → error on `Open`
- [x] Tampered ciphertext → GCM authentication error
- [x] Tampered DEK → error
- [x] Zero-length plaintext → works
- [x] `ParseMasterKey` rejects wrong length / non-hex
- [x] `GenerateDEK` returns 32 bytes, different each call
- [x] Each `Seal` produces different output (unique DEK + nonce per call)

### Acceptance Criteria

- [x] All 14 crypto tests pass
- [x] No plaintext in error messages
- [x] Uses `crypto/rand` for all randomness (no `math/rand`)

---

## M2: User + Session Repositories

**Status:** DONE

### Deliverables

- `internal/store/user.go` — `UserRepository` interface + pgx impl
- `internal/store/session.go` — `SessionRepository` interface + pgx impl
- `internal/store/errors.go` — `ErrNotFound` sentinel
- `internal/store/testutil_test.go` — Test DB helper (connect, migrate, truncate)
- Integration tests for both repos

### Key Types

```go
type User struct {
    ID             uuid.UUID
    Username       string
    PasswordHash   string
    TOTPSecretEnc  []byte
    TOTPSecretDEK  []byte
    IsActive       bool
    FailedAttempts int
    LockedUntil    *time.Time
    CreatedAt      time.Time
    UpdatedAt      time.Time
}

type UserRepository interface {
    GetByUsername(ctx context.Context, username string) (*User, error)
    GetByID(ctx context.Context, id uuid.UUID) (*User, error)
    Create(ctx context.Context, u *User) error
    IncrementFailedAttempts(ctx context.Context, id uuid.UUID) (int, error)
    LockUntil(ctx context.Context, id uuid.UUID, until time.Time) error
    ResetFailedAttempts(ctx context.Context, id uuid.UUID) error
}

type Session struct {
    ID         uuid.UUID
    UserID     uuid.UUID
    TokenHash  string
    IPAddress  string
    UserAgent  string
    ExpiresAt  time.Time
    LastActive time.Time
    CreatedAt  time.Time
}

type SessionRepository interface {
    Create(ctx context.Context, s *Session) error
    GetByTokenHash(ctx context.Context, hash string) (*Session, error)
    TouchLastActive(ctx context.Context, id uuid.UUID, now time.Time) error
    Delete(ctx context.Context, id uuid.UUID) error
    DeleteExpired(ctx context.Context) (int64, error)
}
```

### Integration Tests

- [x] User: create, get by username, get by ID, not found → `ErrNotFound`
- [x] User: duplicate username → error
- [x] User: increment failed attempts, lock, reset
- [x] Session: create, get by token hash, touch, delete
- [x] Session: `DeleteExpired` cleans up stale rows, preserves valid ones

### Acceptance Criteria

- [x] All 17 integration tests pass against Docker Compose Postgres
- [x] `ErrNotFound` returned for missing records
- [x] `go build ./...` compiles, `go vet ./...` clean

---

## M3: Auth Service — Login + TOTP + Sessions

**Status:** DONE

### Deliverables

- `internal/credmgr/service/auth.go` — Auth service
- `internal/credmgr/service/auth_test.go` — TDD tests
- `cmd/credmgr/main.go` — CLI with `seed-admin` subcommand

### API

```go
type AuthService struct {
    users    store.UserRepository
    sessions store.SessionRepository
    audit    store.AuditLogRepository
    mk       crypto.MasterKey
    ttl      time.Duration
    logger   *slog.Logger
}

func (s *AuthService) Login(ctx, username, password, ip, ua string) (totpRequired bool, pendingToken string, err error)
func (s *AuthService) VerifyTOTP(ctx, pendingToken, code string) (sessionToken string, err error)
func (s *AuthService) ValidateSession(ctx, token string) (*store.User, error)
func (s *AuthService) Logout(ctx, token string) error
```

### seed-admin Command

```
credmgr seed-admin --username=admin
```
- Prompts for password (stdin, no echo)
- Generates TOTP secret, encrypts with envelope encryption
- Creates user in DB
- Prints `otpauth://` URI and optionally QR code for Google Authenticator

### TDD Test Cases (write first)

- [x] Login with correct password → returns `totpRequired=true`
- [x] Login with wrong password → increments failed attempts, returns error
- [x] 5th failed attempt → locks account for 30 min
- [x] Locked account → rejects login even with correct password
- [x] Login user not found → `ErrInvalidCredentials` (no user enumeration)
- [x] `VerifyTOTP` with valid code → returns session token
- [x] `VerifyTOTP` with invalid code → `ErrInvalidTOTP`
- [x] `VerifyTOTP` with invalid pending token → `ErrInvalidCredentials`
- [x] `ValidateSession` with valid token → returns user
- [x] `ValidateSession` with expired session (>15min inactive) → `ErrSessionExpired`
- [x] `Logout` → deletes session, subsequent validate fails
- [x] Failed attempts reset after successful TOTP verification
- [ ] Audit logging (deferred to M4 — audit repo not yet implemented)

### Acceptance Criteria

- [x] Full login flow: password → TOTP → session token (11 TDD tests pass)
- [x] Account lockout after 5 failures (FR-1004)
- [x] Session expires after 15 min inactivity (FR-1006)
- [x] `seed-admin` CLI creates user and prints TOTP URI (FR-1002)
- [x] Failed logins logged via slog (FR-1003)
- [x] `go build ./...` compiles, `go vet ./...` clean

### Implementation Notes

- Pending login state stored in-memory (`map[string]*pendingLogin`, 5min TTL) — no DB table needed
- Audit logging deferred to M4/M5 when `AuditLogRepository` exists; currently using slog
- `VerifyTOTP` signature includes `ip` and `ua` params (needed for session creation)

---

## M4: Audit + Credential Repositories

**Status:** DONE

### Deliverables

- `internal/store/audit.go` — `AuditLogRepository` interface + pgx impl
- `internal/store/credential.go` — `CredentialRepository` interface + pgx impl
- Integration tests for both

### Key Types

```go
type AuditLog struct {
    ID         int64
    Timestamp  time.Time
    UserID     *uuid.UUID
    Action     string
    TargetType string
    TargetID   string
    IPAddress  string
    UserAgent  string
    Details    map[string]any
    Success    bool
}

type AuditFilter struct {
    UserID   *uuid.UUID
    Action   string
    FromDate *time.Time
    ToDate   *time.Time
    Limit    int
    Offset   int
}

type AuditLogRepository interface {
    Create(ctx context.Context, log *AuditLog) error
    List(ctx context.Context, filter AuditFilter) ([]AuditLog, int64, error)
}

type BankCredential struct {
    ID             uuid.UUID
    BankCode       string
    AccountLabel   string
    CredentialsEnc []byte
    CredentialsDEK []byte
    Version        int
    Status         string
    DeletedAt      *time.Time
    CreatedBy      uuid.UUID
    UpdatedBy      uuid.UUID
    CreatedAt      time.Time
    UpdatedAt      time.Time
}

type CredentialRepository interface {
    Create(ctx context.Context, c *BankCredential) error
    GetByID(ctx context.Context, id uuid.UUID) (*BankCredential, error)
    List(ctx context.Context) ([]BankCredential, error)
    Update(ctx context.Context, c *BankCredential) error
    SoftDelete(ctx context.Context, id, deletedBy uuid.UUID) error
    HardDeleteExpired(ctx context.Context, retentionDays int) (int64, error)
}
```

### Integration Tests

- [x] Audit: create (with user, without user)
- [x] Audit: list with no filter, by action, by user, by date range, combined filters
- [x] Audit: pagination (limit/offset with total count)
- [x] Audit: immutability (UPDATE/DELETE do nothing via PostgreSQL RULE)
- [x] Audit: JSONB details round-trip
- [x] Audit: INET ip_address round-trip
- [x] Credential: create, get by ID, not found
- [x] Credential: update bumps version (1→2→3)
- [x] Credential: soft delete sets `deleted_at` + `status='deleted'`
- [x] Credential: double soft-delete returns `ErrNotFound`
- [x] Credential: `List` excludes soft-deleted
- [x] Credential: `HardDeleteExpired` only removes past retention period

### Acceptance Criteria

- [x] All 19 integration tests pass (11 audit + 8 credential)
- [x] Audit logs are append-only (immutability verified)
- [x] Credential versioning works
- [x] Soft/hard delete lifecycle correct
- [x] `go build ./...` compiles, `go vet ./...` clean

---

## M5: Credential Service — Business Logic

**Status:** DONE

### Deliverables

- `internal/credmgr/service/credential.go` — Credential service
- `internal/credmgr/service/credential_test.go` — TDD tests

### API

```go
type PlaintextCredential struct {
    BankCode string
    Label    string
    Fields   map[string]string // bank-specific: company_code, user_code, password, etc.
}

type CredentialSummary struct {
    ID        uuid.UUID
    BankCode  string
    Label     string
    Version   int
    Status    string
    CreatedAt time.Time
    UpdatedAt time.Time
}

func (s *CredentialService) Create(ctx, cred PlaintextCredential, userID uuid.UUID, ip, ua string) (uuid.UUID, error)
func (s *CredentialService) List(ctx, userID uuid.UUID, ip, ua string) ([]CredentialSummary, error)
func (s *CredentialService) Update(ctx, id uuid.UUID, cred PlaintextCredential, userID uuid.UUID, ip, ua string) error
func (s *CredentialService) SoftDelete(ctx, id uuid.UUID, userID uuid.UUID, ip, ua string) error
func (s *CredentialService) Test(ctx, cred PlaintextCredential) error
```

### Credential Testing (FR-1204)

`Test()` instantiates the appropriate bank scraper and calls `Login()` + `Logout()`:
- BBVA: `bbva.NewScraper()` → `Login(ctx, bbva.Credentials{...})` → `Logout(ctx)` → `Close()`
- Other banks: return `unsupported bank` error (until implemented)

### TDD Test Cases (write first)

- [x] Create: encrypts data, stores via repo, audit logs creation
- [x] Create: encrypted data can be decrypted back to original fields
- [x] List: returns summaries only (no encrypted data), audit logs access
- [x] Update: bumps version, re-encrypts, audit logs
- [x] SoftDelete: sets status, audit logs, removed from list
- [x] SoftDelete not found → error
- [x] Test with valid creds → nil (fake tester)
- [x] Test with invalid creds → `ErrInvalidCredentials` (fake tester)

### Acceptance Criteria

- [x] Credentials encrypted as JSON before storage (envelope encryption)
- [x] Version increments on update
- [x] Audit trail for every operation (create, list, update, delete — success and failure)
- [x] `Test` delegates to `CredentialTester` interface (wires to scraper `Login()`/`Logout()`)
- [x] All 8 TDD tests pass
- [x] `go build ./...` compiles, `go vet ./...` clean

### Implementation Notes

- `CredentialTester` interface decouples from scraper — testable without browser
- `auditLog` helper is best-effort: failures logged via slog, don't block caller
- `CredentialSummary` strips encrypted fields — safe for UI display

---

## M6: HTTP Layer — Gin Router + Handlers + Templates

**Status:** DONE

### Deliverables

- `internal/credmgr/handler/auth.go` — Login, TOTP, logout
- `internal/credmgr/handler/credential.go` — CRUD handlers
- `internal/credmgr/handler/audit.go` — Audit log viewing + export
- `internal/credmgr/handler/routes.go` — Gin router setup
- `internal/credmgr/middleware/session.go` — Session validation
- `internal/credmgr/middleware/csrf.go` — CSRF protection
- `internal/credmgr/templates/*.html` — Go HTML templates

### Routes

```
GET  /login                  -- Login form
POST /login                  -- Submit username+password
GET  /login/totp             -- TOTP form
POST /login/totp             -- Submit TOTP code
POST /logout                 -- Destroy session

-- Protected (require valid session) --
GET  /credentials            -- List credentials
GET  /credentials/new        -- Add credential form
POST /credentials            -- Create credential
GET  /credentials/:id/edit   -- Edit credential form
PUT  /credentials/:id        -- Update credential
POST /credentials/:id/test   -- Test credential (calls scraper Login)
DELETE /credentials/:id      -- Soft-delete credential

GET  /audit                  -- Audit log list (filterable)
GET  /audit/export           -- Export CSV/JSON (?format=csv|json)
```

### Middleware Stack

1. `gin.Recovery()` — panic recovery
2. `RequestID` — unique ID per request
3. `SessionMiddleware` — validate cookie, inject user into `gin.Context`, touch `last_active`
4. `CSRFMiddleware` — generate/validate CSRF tokens for state-changing requests

### Session Cookie

- HTTP-only, Secure, SameSite=Strict
- Token: `crypto/rand` 32 bytes, stored as SHA-256 hash in DB
- 15min sliding expiry (touch on each request)

### Templates

- `layout.html` — Base layout with nav
- `login.html` — Username + password form
- `totp.html` — TOTP code input
- `credentials_list.html` — Table of credentials (masked)
- `credential_form.html` — Add/edit form (bank-specific fields)
- `audit_logs.html` — Filterable log table + export button

### Integration Tests

- [ ] Login flow: GET login → POST login → GET totp → POST totp → redirect (manual browser test)
- [x] Session middleware rejects unauthenticated requests (redirects to /login)
- [x] CSRF middleware validates double-submit cookie pattern
- [ ] Credential CRUD via HTTP (manual browser test)
- [ ] Audit export returns CSV/JSON (manual browser test)

### Acceptance Criteria

- [x] All routes registered (15 routes)
- [x] Templates parse without errors at startup
- [x] `go build ./...` compiles, `go vet ./...` clean
- [x] All existing tests pass (33 credmgr + 36 store)
- [x] AuditWriter refactor: auth + credential services share DB audit logging
- [x] CSRF protection on all POST routes
- [x] Session middleware with sliding expiry
- [x] Accessing audit logs is itself audited (FR-1307)
- [ ] Full browser test (deferred to M7 integration)

### Implementation Notes

- Templates embedded via `//go:embed` in `templates/templates.go` for single-binary deployment
- Flash messages via short-lived cookies (`credmgr_flash`)
- Pending TOTP token via cookie (`credmgr_pending`, 5min max-age)
- `AuditWriter` extracted to shared struct — used by AuthService, CredentialService, and AuditHandler
- Credential test handler (`POST /credentials/:id/test`) wired but tester=nil in serve (scraper integration deferred)

---

## M7: CLI Entrypoint + Integration + Dockerfile

**Status:** DONE

### Deliverables

- `cmd/credmgr/main.go` — Full CLI with subcommands
- `Dockerfile` — Multi-stage build
- End-to-end integration test
- Makefile targets

### CLI Subcommands

```
credmgr serve         -- Start Gin HTTP server
credmgr seed-admin    -- Create initial admin user (interactive)
credmgr migrate       -- Run database migrations
credmgr migrate-down  -- Rollback last migration
```

### Dockerfile

```dockerfile
# Builder
FROM golang:1.22 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o credmgr ./cmd/credmgr

# Runtime
FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/credmgr /usr/local/bin/
ENTRYPOINT ["credmgr"]
CMD ["serve"]
```

### Makefile Targets

```makefile
db-up:           docker compose up -d postgres
db-down:         docker compose down
migrate:         go run ./cmd/credmgr migrate
migrate-down:    go run ./cmd/credmgr migrate-down
seed-admin:      go run ./cmd/credmgr seed-admin
credmgr-serve:   go run ./cmd/credmgr serve
credmgr-build:   go build -o bin/credmgr ./cmd/credmgr
docker-build:    docker build -t bank-scraper-credmgr .
```

### End-to-End Test

- [x] Start server against test DB (`make credmgr-serve`)
- [x] Seed admin user (`make seed-admin --username=admin`)
- [x] Login (password + TOTP) — verified via browser
- [ ] Create credential (browser test)
- [ ] List credentials (browser test)
- [ ] Update credential (browser test)
- [ ] Test credential (requires live bank — skip in CI)
- [ ] Soft-delete credential (browser test)
- [ ] View audit logs (browser test)
- [ ] Export audit logs as CSV (browser test)

### Acceptance Criteria

- [x] `docker compose up` + `credmgr migrate` + `credmgr seed-admin` + `credmgr serve` → working web UI
- [ ] Full CRUD flow works in browser (manual verification)
- [x] Docker image builds and runs (`docker build` succeeds, binary executes)
- [x] All existing tests pass (`go build ./...`, `go vet ./...`, `go test ./internal/credmgr/...`)
- [x] BBVA credential tester wired (calls `bbva.NewScraper().Login().Logout()`)
- [x] Makefile targets: `seed-admin`, `credmgr-serve`, `docker-build`

---

## Notes

### Key Files to Reference

- `internal/scraper/bank/bbva/scraper.go` — `Login()` / `Logout()` for credential testing
- `internal/scraper/bank/errors.go` — `ErrInvalidCredentials`, `ErrBotDetection` for test result classification
- `internal/scraper/bank/interface.go` — `bank.Code` constants (BankBBVA, BankInterbank, BankBCP)

### Credential Testing Timeout

`bbva.NewScraper()` launches a browser. The `Test` endpoint needs a generous timeout (60s). Show a loading indicator in the UI — consider making it an async operation if UX is poor.

### TOTP Setup UX

`pquerna/otp` provides `key.Image()` for PNG QR and `key.URL()` for `otpauth://` URI. The `seed-admin` CLI should print both the URI (for manual entry) and optionally save a QR PNG.

### Security Checklist

- [x] Credentials NEVER logged in plaintext (FR-1104) — only encrypted blobs in DB, slog never logs fields
- [x] Credentials NEVER in API responses (FR-1105) — `CredentialSummary` strips encrypted data
- [x] TOTP secrets encrypted at rest (FR-1005) — envelope encryption via `crypto.Seal`
- [x] Session cookie: HttpOnly, SameSite=Strict
- [x] CSRF protection on all POST routes (double-submit cookie)
- [x] Password: bcrypt cost 12
- [x] Session token: `crypto/rand` 32 bytes, SHA-256 hash in DB
- [x] Error messages: generic to user, details logged server-side
- [x] Audit log access audited (FR-1307)
- [ ] Session cookie: HttpOnly, Secure, SameSite=Strict
- [ ] Session token: `crypto/rand`, stored as SHA-256 hash
- [ ] Password: bcrypt with cost 12+
- [ ] CSRF protection on all state-changing requests
- [ ] Audit log access is audited (FR-1307)
