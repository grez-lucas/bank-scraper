# TODO: BBVA Scraper — Next Steps

## Roadmap

1. ~~**GetBalance** — scraper method to fetch account balances~~ **DONE**
2. **GetTransactions** — scraper method to fetch transaction history (this doc)
3. **API layer** — REST API exposing Login, GetBalance, GetTransactions to AyniFX

## What's Already Done

| Component | Status |
|-----------|--------|
| `Login` | Working — HAR replay tested (success, 403, invalid creds, re-login) |
| `GetBalance` | Working — navigates to accounts page, flattens shadow DOM, delegates to parser. Integration test ready (needs HAR recording). |
| `ParseAccountBalances` | Working — unit tested against `accounts_list` and `accounts_tile` fixtures |
| `ParseTransactions` | Working — unit tested against `transactions` fixture (50 rows) |
| `DetectAnnouncementModal` | Working — unit tested against 6 fixtures + edge cases |
| `dismissAnnouncementModal` | Working — called after login and in `navigateTo` |
| `navigateTo` helper | Working — navigate + wait load + DOM stable + dismiss modal |
| Page lifecycle | Working — page stored on `BBVAScraper`, cleaned up on failure/re-login/close |
| Hijacker lifetime | Working — router stored on `BBVAScraper`, kept alive between operations, stopped on close/re-login/failure |
| `FlattenShadowDOM` | Working — in `browser/shadow.go`, tested |
| Selectors | All defined in `selectors.go` for accounts (list + tile) and transactions |

---

## Pending: Capture GetBalance HAR recording

The `GetBalance` integration test is implemented but skips because `get-balance.har.json` doesn't exist yet.

```bash
# Record a session that includes login + accounts page navigation
go run ./scripts/record-session -bank=bbva -scenario=get-balance
```

The recording must include all HTTP traffic from login through accounts page load. The test calls Login first, then GetBalance — both served from the same HAR.

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
