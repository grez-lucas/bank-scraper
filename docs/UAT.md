# User Acceptance Testing (UAT)

> Test plan for validating the Bank Scraper Platform against PRD requirements.
> Each scenario maps to a user story and its acceptance criteria.

## Prerequisites

Before starting UAT, ensure:

| Requirement | How to verify |
|---|---|
| PostgreSQL running | `make db-up` then `docker compose ps` |
| Migrations applied | `go run ./cmd/api migrate` |
| Admin user seeded | `go run ./cmd/credmgr seed-admin --username=admin` |
| API key created | `go run ./cmd/api create-key --client-id=uat-tester` — save the key |
| Credential Manager running | `make credmgr-serve` (port 8081) |
| API Gateway running | `make api-serve` (port 8080) |
| BBVA credentials | Added via Credential Manager UI |
| Bruno CLI installed | `npm install -g @usebruno/cli` (for automated E2E) |

---

## 1. API Gateway Tests

### UAT-001: Query Account Balance (US-001)

**User Story**: As the AyniFX platform, I want to query the current balance of a specific bank, to show available balances for FX operations.

| # | Scenario | Steps | Expected Result | Pass? |
|---|----------|-------|-----------------|-------|
| 1.1 | Balance in native currency | `curl -H "X-API-Key: <key>" localhost:8080/api/v1/accounts/<pen_account_id>/balance` | Response contains `currency: "PEN"`, `available_balance` as decimal string (e.g., `"1234.56"`) | |
| 1.2 | Balance includes timestamp | Same as 1.1 | Response contains `fetched_at` in ISO 8601 format | |
| 1.3 | Response time < 30s | Time the request: `time curl ...` | Total time under 30 seconds (first request may be slower due to login) | |
| 1.4 | Invalid account ID | `curl -H "X-API-Key: <key>" localhost:8080/api/v1/accounts/not-a-uuid/balance` | HTTP 400 with `{"status":"error","message":"invalid account_id"}` | |
| 1.5 | Unknown account ID | `curl -H "X-API-Key: <key>" localhost:8080/api/v1/accounts/00000000-0000-0000-0000-000000000000/balance` | HTTP 404 with `{"status":"error","message":"account not found"}` | |
| 1.6 | USD account balance | `curl -H "X-API-Key: <key>" localhost:8080/api/v1/accounts/<usd_account_id>/balance` | Response contains `currency: "USD"` | |

### UAT-002: Query Transaction History (US-002)

**User Story**: As the AyniFX platform, I want to query recent transactions for a specific bank account, to reconcile payments and track movements.

| # | Scenario | Steps | Expected Result | Pass? |
|---|----------|-------|-----------------|-------|
| 2.1 | Default 7-day range | `curl -H "X-API-Key: <key>" localhost:8080/api/v1/accounts/<id>/transactions` | `from_date` and `to_date` span approximately 7 days | |
| 2.2 | Reverse chronological order | Same as 2.1 | If multiple transactions returned, dates are newest → oldest | |
| 2.3 | Custom date range | `curl ... /transactions?from_date=2026-03-01&to_date=2026-03-20` | `from_date`=`2026-03-01`, `to_date`=`2026-03-20` in response | |
| 2.4 | Transaction fields | Same as 2.1 | Each transaction has: `id`, `date`, `description`, `amount`, `type` (CREDIT/DEBIT) | |
| 2.5 | Balance after (optional) | Same as 2.1 | Some transactions include `balance_after` as decimal string | |
| 2.6 | Date range > 90 days rejected | `curl ... /transactions?from_date=2025-01-01&to_date=2026-03-24` | HTTP 400 with message about "90 days" | |
| 2.7 | Pagination | `curl ... /transactions?page=1&page_size=5` | `pagination.page_size`=5, at most 5 transactions returned, `has_more` reflects whether more exist | |

### UAT-003: List Available Accounts (US-003)

**User Story**: As the AyniFX platform, I want to list all configured bank accounts, to discover what accounts are available to query.

| # | Scenario | Steps | Expected Result | Pass? |
|---|----------|-------|-----------------|-------|
| 3.1 | List all accounts | `curl -H "X-API-Key: <key>" localhost:8080/api/v1/accounts` | Array of accounts with `account_id`, `bank_code`, `currency` | |
| 3.2 | No sensitive data exposed | Same as 3.1 | Account numbers are masked (e.g., `XXXXXXXXXXXXXXXX4607`). No passwords, credentials, or encryption keys in response | |
| 3.3 | Filter by bank | `curl ... /accounts?bank_code=BBVA` | All returned accounts have `bank_code: "BBVA"` | |
| 3.4 | Filter by currency | `curl ... /accounts?currency=PEN` | All returned accounts have `currency: "PEN"` | |
| 3.5 | Account fields complete | Same as 3.1 | Each account has: `account_id`, `bank_code`, `currency`, `account_type`, `status` | |

### UAT-004: Health Check (US-004)

**User Story**: As the AyniFX platform, I want to check the health of bank connections, to know if a bank integration is operational.

| # | Scenario | Steps | Expected Result | Pass? |
|---|----------|-------|-----------------|-------|
| 4.1 | System healthy | `curl -H "X-API-Key: <key>" localhost:8080/api/v1/health` | `status: "healthy"`, `timestamp` in ISO 8601 | |
| 4.2 | Per-bank status | Same as 4.1, after a successful balance query | `banks.BBVA.status: "healthy"`, `banks.BBVA.last_successful_connection` is a timestamp | |
| 4.3 | No scraping triggered | Monitor server logs during health check | No scraper login or page navigation logged | |
| 4.4 | Degraded status | Stop the database, then call health | `status: "degraded"` | |

---

## 2. Credential Manager Tests

### UAT-101: Add Bank Credentials (US-101)

**User Story**: As a C-level executive, I want to securely add bank credentials, to allow the scraper to access the bank.

| # | Scenario | Steps | Expected Result | Pass? |
|---|----------|-------|-----------------|-------|
| 101.1 | 2FA required | Navigate to `localhost:8081/credentials` without logging in | Redirected to `/login` | |
| 101.2 | TOTP authentication | Login with username/password, then enter TOTP code | Access granted to credentials page | |
| 101.3 | Create credential | Click "Add Credential", fill bank code (BBVA), label, company code, user code, password. Submit | Success flash: "Credential created successfully", credential appears in list | |
| 101.4 | Encrypted at rest | Query DB: `SELECT credentials_enc FROM bank_credentials` | Data is binary (encrypted), not readable as JSON | |
| 101.5 | Action audited | Navigate to Audit Log page | Entry shows `credential_created` with user ID and timestamp | |
| 101.6 | One credential per bank | Try creating a second BBVA credential | Error: "credential already exists for this bank" | |

### UAT-102: View Configured Accounts (US-102)

| # | Scenario | Steps | Expected Result | Pass? |
|---|----------|-------|-----------------|-------|
| 102.1 | List shows credentials | Navigate to `/credentials` | Table with bank code, label, version, last updated | |
| 102.2 | No passwords shown | Inspect page source and network tab | No password, company_code, user_code, or encryption keys in HTML | |
| 102.3 | Access logged | Check audit log after viewing | Entry for `credentials_listed` | |

### UAT-103: Update Bank Credentials (US-103)

| # | Scenario | Steps | Expected Result | Pass? |
|---|----------|-------|-----------------|-------|
| 103.1 | Edit form | Click "Edit" on a credential | Form shows bank code and label (pre-filled), password fields are empty | |
| 103.2 | Old credentials not shown | Same as 103.1 | No pre-filled password, company code, or user code values | |
| 103.3 | Version bumped | Submit update with new values | Version incremented (e.g., v1 → v2) | |
| 103.4 | Change audited | Check audit log | Entry for `credential_updated` with credential ID | |
| 103.5 | Test before save | Click "Test" on a credential | Flash message: "Credential test passed!" (or failure message if invalid) | |

### UAT-104: Delete Bank Credentials (US-104)

| # | Scenario | Steps | Expected Result | Pass? |
|---|----------|-------|-----------------|-------|
| 104.1 | Confirmation required | Click "Delete" on a credential | Browser confirmation dialog appears | |
| 104.2 | Soft delete | Confirm deletion | Credential disappears from list. DB record has `status='deleted'` and `deleted_at` set | |
| 104.3 | Deletion audited | Check audit log | Entry for `credential_deleted` | |

### UAT-105: View Audit Logs (US-105)

| # | Scenario | Steps | Expected Result | Pass? |
|---|----------|-------|-----------------|-------|
| 105.1 | Comprehensive log | Navigate to `/audit` | Shows all credential operations, login attempts, and system events | |
| 105.2 | Filterable | Use date/action filters on audit page | Results filtered accordingly | |
| 105.3 | Export to CSV | Click export as CSV | CSV file downloads with all audit entries | |
| 105.4 | Export to JSON | Click export as JSON | JSON file downloads with all audit entries | |
| 105.5 | No sensitive data in logs | Inspect exported data | No passwords, encryption keys, or credential values in any log entry | |

---

## 3. Account Discovery Tests

| # | Scenario | Steps | Expected Result | Pass? |
|---|----------|-------|-----------------|-------|
| D.1 | Discover via CredMgr UI | Navigate to credential → Accounts → click "Discover Accounts" | Success flash: "Discovered N account(s) for BBVA", accounts table populated | |
| D.2 | Discover via API | `curl -X POST -H "X-API-Key: <key>" localhost:8080/api/v1/admin/discover/BBVA` | HTTP 200 with `accounts` array containing BBVA accounts | |
| D.3 | Discover via CLI | `go run ./cmd/api discover --bank=BBVA` | Output: "Discovered N account(s)" with masked account numbers | |
| D.4 | No credential configured | `curl -X POST -H "X-API-Key: <key>" localhost:8080/api/v1/admin/discover/UNKNOWN` | HTTP 404 with error message about missing credential | |
| D.5 | Discovery error feedback | Temporarily misconfigure BBVA credential, then trigger discover | Error flash/response indicating login failure | |

---

## 4. End-to-End Integration Flow

This flow validates the full system from credential setup through balance query.

| Step | Action | Expected Result |
|------|--------|-----------------|
| 1 | Start services | `make db-up && make migrate && make credmgr-serve` (background) + `make api-serve` (background) |
| 2 | Create admin user | `go run ./cmd/credmgr seed-admin --username=admin` — save TOTP secret |
| 3 | Create API key | `go run ./cmd/api create-key --client-id=e2e-test` — save the key |
| 4 | Login to CredMgr | Open `localhost:8081`, login with admin + TOTP | Dashboard loads |
| 5 | Add BBVA credential | Create credential with valid BBVA company code, user code, password | Success flash |
| 6 | Test credential | Click "Test" on the BBVA credential | "Credential test passed!" |
| 7 | Discover accounts | Navigate to Accounts page, click "Discover Accounts" | Accounts table shows PEN + USD accounts |
| 8 | Health check (API) | `curl -H "X-API-Key: <key>" localhost:8080/api/v1/health` | `status: "healthy"` |
| 9 | List accounts (API) | `curl -H "X-API-Key: <key>" localhost:8080/api/v1/accounts` | Returns BBVA accounts with masked numbers |
| 10 | Get PEN balance | `curl -H "X-API-Key: <key>" localhost:8080/api/v1/accounts/<pen_id>/balance` | PEN balance with `available_balance` + `fetched_at` |
| 11 | Get USD balance | `curl -H "X-API-Key: <key>" localhost:8080/api/v1/accounts/<usd_id>/balance` | USD balance returned |
| 12 | Get transactions | `curl -H "X-API-Key: <key>" localhost:8080/api/v1/accounts/<pen_id>/transactions` | Transaction list with pagination |
| 13 | Verify audit trail | Navigate to CredMgr → Audit Log | Shows credential creation, test, and discovery events |
| 14 | Graceful shutdown | Send SIGTERM to API process | Logs: "shutting down scraper", "API gateway stopped" — no errors |

**Automated alternative**: Run `make test-e2e` (steps 8-12 automated via Bruno collection).

---

## 5. Security Validation

| # | Check | Steps | Expected Result | Pass? |
|---|-------|-------|-----------------|-------|
| S.1 | API key required | `curl localhost:8080/api/v1/accounts` (no header) | HTTP 401: `"missing API key"` | |
| S.2 | Invalid API key rejected | `curl -H "X-API-Key: fake-key" localhost:8080/api/v1/accounts` | HTTP 401: `"invalid API key"` | |
| S.3 | Revoked key rejected | Revoke a key in DB (`UPDATE api_keys SET revoked_at=now()`) then use it | HTTP 401: `"API key revoked"` | |
| S.4 | Credentials encrypted at rest | `SELECT credentials_enc FROM bank_credentials` | Binary data, not readable JSON | |
| S.5 | No credentials in API responses | Query any API endpoint | No `password`, `company_code`, `user_code`, or `encryption_key` in any response | |
| S.6 | Account numbers masked | `GET /api/v1/accounts` | Account numbers show as `XXXXXXXXXXXXXXXX4607` | |
| S.7 | Session timeout (CredMgr) | Login to CredMgr, wait 15+ minutes, try an action | Redirected to login page | |
| S.8 | Login lockout | Enter wrong password 5 times in CredMgr | Account locked, further attempts rejected | |
| S.9 | Error responses don't leak internals | Trigger a 500 error (e.g., stop DB mid-request) | Error message is generic ("internal error"), no stack traces or SQL | |

---

## 6. Traceability Matrix

Maps each PRD acceptance criterion to a UAT scenario.

| PRD Criterion | UAT Scenario(s) |
|---|---|
| **US-001**: Balance in native currency (USD/PEN) | 1.1, 1.6 |
| **US-001**: Response includes fetched_at timestamp | 1.2 |
| **US-001**: Response time < 30 seconds | 1.3 |
| **US-001**: Invalid account IDs return error codes | 1.4, 1.5 |
| **US-002**: Reverse chronological order | 2.2 |
| **US-002**: Default range 7 days | 2.1 |
| **US-002**: Configurable date range | 2.3 |
| **US-002**: Transaction fields: date, description, amount, type, balance_after | 2.4, 2.5 |
| **US-003**: Returns account IDs with bank + currency | 3.1, 3.5 |
| **US-003**: No credentials exposed | 3.2 |
| **US-003**: Filterable by bank/currency | 3.3, 3.4 |
| **US-004**: Per-bank status (healthy/degraded/unavailable) | 4.1, 4.2, 4.4 |
| **US-004**: Last successful connection timestamp | 4.2 |
| **US-004**: No scraping triggered | 4.3 |
| **US-101**: 2FA required | 101.1, 101.2 |
| **US-101**: Encrypted at rest | 101.4 |
| **US-101**: Action audited | 101.5 |
| **US-102**: Shows bank, masked account, currency, status | 102.1 |
| **US-102**: No plaintext passwords | 102.2 |
| **US-102**: Access logged | 102.3 |
| **US-103**: Old credentials never shown | 103.2 |
| **US-103**: Change audited (before/after metadata) | 103.4 |
| **US-103**: Test credentials before save | 103.5 |
| **US-104**: Confirmation required | 104.1 |
| **US-104**: Soft-delete with retention | 104.2 |
| **US-105**: Filterable by date, user, action | 105.2 |
| **US-105**: Exportable to CSV/JSON | 105.3, 105.4 |
| **US-105**: Includes all API calls and credential events | 105.1 |
| **FR-101–106**: API key authentication | S.1, S.2, S.3 |
| **FR-1101**: AES-256-GCM encryption at rest | S.4 |
| **FR-1104–1105**: Credentials never in logs/responses | S.5, S.9 |
| **FR-1006**: 15-minute session timeout | S.7 |
| **FR-1004**: Account lockout after 5 failures | S.8 |
