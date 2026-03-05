# TODO: BBVA Scraper — Next Steps

## Roadmap

1. ~~**GetBalance** — scraper method to fetch account balances~~ **DONE**
2. **GetTransactions** — scraper method to fetch transaction history (this doc)
3. **API layer** — REST API exposing Login, GetBalance, GetTransactions to AyniFX

## What's Already Done

| Component | Status |
|-----------|--------|
| `Login` | Working — HAR replay tested (success, invalid creds, re-login). Uses Senda `#enviarSenda` flow. |
| `Login` replay (Senda probe) | Working — separate-tab `probeSendaAPI` bypasses broken iframe postMessage chain in replay mode. Classifies grantingTicket 200→success, 403→error with error-code. |
| `waitForLoginOutcome` | Working — dual-path: probe for replay mode, DOM polling (URL redirect + span#error-message) for live mode. |
| `classifySendaError` | Working — handles both UI text (live) and API probe format `"senda API error-code NNN"` (replay). Unit tested. |
| `classifySendaErrorCode` | Working — maps Senda error codes (160, 162) to typed errors. Unit tested. |
| HijackRouter context fix | Working — router created before `page.Timeout()` to prevent `CancelTimeout()` from killing router's event context. |
| `GetBalance` | Working — navigates to accounts page, flattens shadow DOM, delegates to parser. |
| `ParseAccountBalances` | Working — unit tested against `accounts_list` and `accounts_tile` fixtures |
| `ParseTransactions` | Working — unit tested against `transactions` fixture (50 rows) |
| `DetectAnnouncementModal` | Working — unit tested against 6 fixtures + edge cases |
| `dismissAnnouncementModal` | Working — called after login and in `navigateTo`. Skipped in replay mode. |
| `navigateTo` helper | Working — navigate + wait load + DOM stable + dismiss modal |
| Page lifecycle | Working — page stored on `BBVAScraper`, cleaned up on failure/re-login/close |
| Hijacker lifetime | Working — router stored on `BBVAScraper`, kept alive between operations, stopped on close/re-login/failure |
| `FlattenShadowDOM` | Working — in `browser/shadow.go`, tested |
| Selectors | All defined in `selectors.go` for accounts (list + tile) and transactions |

## Test Status

| Test | Mode | Status |
|------|------|--------|
| `TestBBVAScraper_Login_ReplaySuccess_Integration` | replay | PASS |
| `TestBBVAScraper_Login_ReplayErrorInvalidCredentials_Integration` | replay | PASS |
| `TestBBVAScraper_Login_ReplayRelogin_Integration` | replay | PASS |
| `TestBBVAScraper_Login_ReplayError403BotDetection_Integration` | replay | SKIP — needs re-recorded HAR with `#enviarSenda` |
| `TestBBVAScraper_GetBalance_Replay_Integration` | replay | SKIP — portal SPA can't initialize in CDP Fetch replay (see below) |
| `TestClassifySendaError` | mock | PASS (9 cases) |
| `TestClassifySendaErrorCode` | mock | PASS (4 cases) |

---

## Blocked: GetBalance / GetTransactions Replay Tests

The portal SPA (Cells/Polymer framework) **cannot initialize in CDP Fetch replay mode**. CDP Fetch's `FetchFulfillRequest` bypasses cookie processing — `Set-Cookie` headers from replayed responses are NOT stored in the browser. The Cells framework's bootstrap chain requires auth cookies and `tsec` session tokens set during the real login redirect, so Custom Elements never register and the page renders empty shells with no shadow DOM content.

**Attempted and failed:**
- Injecting `tsec` into `sessionStorage`/`localStorage` — Cells framework stores session data through its own mechanisms
- Navigating to `portalURL` after probe success — resources load but framework doesn't bootstrap

**Possible approaches:**
1. **Direct API probe** (like login) — identify the internal API endpoints the portal calls for balance/transaction data and probe them directly via the separate-tab technique. Most promising.
2. **Re-architect replay** to use a local HTTP proxy instead of CDP Fetch, so cookies are processed normally. High effort.
3. **Accept live-only integration tests** for GetBalance/GetTransactions, rely on unit-tested parsers for correctness. Pragmatic fallback.

---

## Pending: Re-record Bot Detection HAR

`login-bot-detection.har.json` was recorded with the legacy `#aceptar` button. Needs re-recording with `#enviarSenda` to match the current scraper flow. The test is skipped until then.

---

## Next: Implement `GetTransactions`

### Overview

`GetTransactions` navigates to the transactions page for a specific account, flattens shadow DOM, and delegates to the existing `ParseTransactions` parser.

```
s.page  →  navigateTo(txURL)  →  FlattenShadowDOM(page)  →  ParseTransactions(html)  →  []bank.Transaction
```

### 1. Add transactions URL pattern

**File:** `scraper.go`

```go
// transactionsURL builds the URL for a specific account's movements page
func transactionsURL(accountID string) string {
    return baseURL + "/nextgenempresas/portal/index.html#!/bbva-btge-accounts-solution/account/" + accountID + "/movements"
}
```

### 2. Add `GetTransactions` method

**File:** `scraper.go`

```go
func (s *BBVAScraper) GetTransactions(ctx context.Context, accountID string) ([]bank.Transaction, error)
```

Implementation:
- Guard: check `s.page != nil`, return `ErrSessionExpired` if nil
- Call `navigateTo(s.page, transactionsURL(accountID))`
- Call `browser.FlattenShadowDOM(s.page)`
- Call `ParseTransactions(html)` — already tested
- Wrap errors in `bank.ScraperError{BankCode: BankBBVA, Operation: "GetTransactions", ...}`

### 3. Pagination: "Ver mas" button

The transactions page shows 50 rows at a time. To get all transactions, we need to:
1. Parse current page
2. Check if "Ver mas" button exists
3. Click it, wait for DOM stable
4. Re-flatten and re-parse
5. Repeat until no more "Ver mas"

**Open question:** Should pagination be handled inside `GetTransactions` or as a separate concern? Consider:
- Simple: loop inside GetTransactions until all pages loaded
- Complex: return a page token and let caller decide

### 4. Integration test

**File:** `scraper_test.go`

```go
func TestBBVAScraper_GetTransactions_Replay_Integration(t *testing.T) {
    skipUnlessMode(t, TestModeReplay)
    // Loads get-transactions.har.json (login + navigate to transactions)
    // Calls Login, then GetTransactions
    // Asserts transactions returned
}
```

### 5. Capture HAR recording

```bash
go run ./scripts/record-session -bank=bbva -scenario=get-transactions
```

### Verification

```bash
# Unit tests (already passing)
go test ./internal/scraper/bank/bbva/... -v -run TestParseTransactions

# Integration test (after HAR is captured)
SCRAPER_TEST_MODE=replay go test ./internal/scraper/bank/bbva/... -v -run TestBBVAScraper_GetTransactions

# Full suite
SCRAPER_TEST_MODE=replay go test ./internal/scraper/bank/bbva/... -v
go build ./...
go vet ./...
```
