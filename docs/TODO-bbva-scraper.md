# TODO: BBVA Scraper — Next Steps

## Roadmap

1. **GetBalance** — scraper method to fetch account balances (this doc)
2. **GetTransactions** — scraper method to fetch transaction history
3. **API layer** — REST API exposing Login, GetBalance, GetTransactions to AyniFX

## What's Already Done

| Component | Status |
|-----------|--------|
| `Login` | Working — HAR replay tested (success, 403, invalid creds, re-login) |
| `ParseAccountBalances` | Working — unit tested against `accounts_list` and `accounts_tile` fixtures |
| `ParseTransactions` | Working — unit tested against `transactions` fixture (50 rows) |
| `DetectAnnouncementModal` | Working — unit tested against 6 fixtures + edge cases |
| `dismissAnnouncementModal` | Working — called after login and in `navigateTo` |
| `navigateTo` helper | Working — navigate + wait load + DOM stable + dismiss modal |
| Page lifecycle | Working — page stored on `BBVAScraper`, cleaned up on failure/re-login/close |
| `FlattenShadowDOM` | Working — in `browser/shadow.go`, tested |
| Selectors | All defined in `selectors.go` for accounts (list + tile) and transactions |

---

## Next: Implement `GetBalance`

### Overview

`GetBalance` navigates to the accounts page, flattens shadow DOM, and delegates to the existing `ParseAccountBalances` parser.

```
s.page  →  navigateTo(accountsURL)  →  FlattenShadowDOM(page)  →  ParseAccountBalances(html)  →  []bank.Balance
```

### 1. Add accounts URL constant

**File:** `scraper.go`

```go
accountsURL = baseURL + "/nextgenempresas/portal/index.html#!/bbva-btge-accounts-solution/"
```

### 2. Add `GetBalance` method

**File:** `scraper.go`

```go
func (s *BBVAScraper) GetBalance(ctx context.Context) ([]bank.Balance, error)
```

Implementation:
- Guard: check `s.page != nil` (must be logged in), return `ErrSessionExpired` if nil
- Call `navigateTo(s.page, accountsURL)` — handles wait + DOM stable + modal dismiss
- Call `browser.FlattenShadowDOM(s.page)` — inlines shadow DOM for goquery
- Call `ParseAccountBalances(html)` — already tested, returns `[]bank.Balance`
- Wrap errors in `bank.ScraperError{BankCode: BankBBVA, Operation: "GetBalance", ...}`

**Note:** `FlattenShadowDOM` mutates the live DOM. This is fine — we read the HTML immediately after and don't reuse the flattened state. The next `navigateTo` call loads a fresh page.

### 3. Write integration test

**File:** `scraper_test.go`

Needs a new HAR recording: `testdata/recordings/get-balance.har.json`

Recording checklist:
- Start from authenticated session (login first)
- Navigate to accounts page
- Wait for accounts table to render
- Stop recording

Test: `TestBBVAScraper_GetBalance_Replay_Integration`
- Login with success HAR, then call GetBalance with accounts HAR
- Assert `len(balances) >= 2` (PEN + USD accounts)
- Assert each balance has non-empty AccountID, valid Currency, non-zero AvailableBalance

**Alternative (if no HAR yet):** Start with just the parser unit tests (already passing) and add the integration test once a recording is captured.

### 4. Capture HAR recording

```bash
# Record a session that includes login + accounts page navigation
go run ./scripts/record-session -bank=bbva -scenario=get-balance
```

The recording must include all HTTP traffic from login through accounts page load. The replayer will serve responses for both the login and accounts navigation.

### Verification

```bash
# Unit tests (already passing)
go test ./internal/scraper/bank/bbva/... -v -run TestParseAccountBalances

# Integration test (after HAR is captured)
SCRAPER_TEST_MODE=replay go test ./internal/scraper/bank/bbva/... -v -run TestBBVAScraper_GetBalance

# Full suite
SCRAPER_TEST_MODE=replay go test ./internal/scraper/bank/bbva/... -v
go build ./...
go vet ./...
```
