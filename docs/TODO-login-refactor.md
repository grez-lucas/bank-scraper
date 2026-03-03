# TODO: Login Method Refactor

> Reference document for a future coding session. Not implemented yet.
>
> **Goal:** Move `Credentials` to the shared `bank` package, accept a `*rod.Page` in `Login`, and add cookie popup handling.

---

## Task List

### 1. Add `bank.Credentials` type

**File:** `internal/scraper/bank/types.go`

Add a flat credentials struct to the shared bank package:

```go
type Credentials struct {
    CompanyCode string
    Username    string
    Password    string
}
```

This replaces the bank-specific `bbva.Credentials` struct with a shared type usable by all bank scrapers.

---

### 2. ~~Add cookie popup selector~~ **DONE — Announcement modal selectors**

**File:** `internal/scraper/bank/bbva/selectors.go`

Added selectors for the announcement/news modal that appears after login (not a cookie popup):

```go
// Announcement modal (post-login news popup)
SelectorAnnouncementModal    = `bbva-btge-microfrontend-modal[opened]`
SelectorAnnouncementCloseBtn = `bbva-btge-microfrontend-modal[opened] button.close-btn`
```

---

### 3. Refactor `Login` method signature

**File:** `internal/scraper/bank/bbva/scraper.go`

Change from:

```go
func (s *BBVAScraper) Login(ctx context.Context, creds Credentials) (*bank.Session, error)
```

To:

```go
func (s *BBVAScraper) Login(ctx context.Context, page *rod.Page, credentials bank.Credentials) error
```

Key changes:
- Accept `*rod.Page` from caller (scraper no longer creates the page)
- Use shared `bank.Credentials` instead of `bbva.Credentials`
- Return `error` only (session is stored internally on `s.session`)
- Remove `defer page.Close()` - caller owns the page lifecycle

---

### 4. ~~Add `dismissCookiePopup` helper~~ **DONE — `dismissAnnouncementModal`**

**File:** `internal/scraper/bank/bbva/scraper.go`

```go
func dismissAnnouncementModal(page *rod.Page) {
    el, err := page.Timeout(3 * time.Second).Element(SelectorAnnouncementCloseBtn)
    if err != nil {
        return // Modal not present
    }
    visible, err := el.Visible()
    if err != nil || !visible {
        return
    }
    _ = el.Click(proto.InputMouseButtonLeft, 1)
    time.Sleep(500 * time.Millisecond)
}
```

Called after `isLoginSuccessful` in the Login method (modal appears on dashboard after login).

---

### 5. Update `fillLoginForm` to use `bank.Credentials`

**File:** `internal/scraper/bank/bbva/scraper.go`

Change from:

```go
func fillLoginForm(page *rod.Page, creds Credentials) error
```

To:

```go
func fillLoginForm(page *rod.Page, creds bank.Credentials) error
```

Update field references:
- `creds.CompanyCode` stays the same
- `creds.UserCode` -> `creds.Username`
- `creds.Password` stays the same

---

### 6. Remove `bbva.Credentials` type

**File:** `internal/scraper/bank/bbva/scraper.go`

Delete the local `Credentials` struct:

```go
// DELETE THIS:
type Credentials struct {
    CompanyCode string
    UserCode    string
    Password    string
}
```

---

### 7. Add `NewPage()` method to `BBVAScraper`

**File:** `internal/scraper/bank/bbva/scraper.go`

```go
func (s *BBVAScraper) NewPage() (*rod.Page, error) {
    page, err := s.browser.Page(proto.TargetCreateTarget{URL: ""})
    if err != nil {
        return nil, fmt.Errorf("create page: %w", err)
    }
    return page, nil
}
```

This gives callers a way to create pages from the scraper's browser instance.

---

### 8. Session management inside Login

**File:** `internal/scraper/bank/bbva/scraper.go`

Keep session creation inside Login. On success, store page ref + 10min expiry:

```go
s.session = &bank.Session{
    ID:        generateSessionID(),
    BankCode:  bank.BankBBVA,
    ExpiresAt: time.Now().Add(bbvaSessionTimeout), // 10 minutes
    // Store page reference for subsequent operations
}
```

The page reference should be accessible for `GetBalance`, `GetTransactions`, etc.

---

### 9. Update `BankScraper` interface

**File:** `internal/scraper/bank/interface.go`

Update the `Login` signature in the interface to match:

```go
type BankScraper interface {
    Login(ctx context.Context, page *rod.Page, credentials Credentials) error
    // ... other methods
}
```

**Note:** This introduces a Rod dependency in the interface package. Consider whether to use an abstraction (e.g., `PageProvider` interface) or accept the coupling since all scrapers use Rod.

---

### 10. Update tests

**File:** `internal/scraper/bank/bbva/scraper_test.go`

Update all test functions to use the new signature:

```go
// Before:
session, err := scraper.Login(ctx, Credentials{
    CompanyCode: "test-company",
    UserCode:    "test-user",
    Password:    "test-password",
})

// After:
page, err := scraper.NewPage()
require.NoError(t, err)
err = scraper.Login(ctx, page, bank.Credentials{
    CompanyCode: "test-company",
    Username:    "test-user",
    Password:    "test-password",
})
```

Tests affected:
- `TestBBVAScraper_Login_ReplaySuccess_Integration`
- `TestBBVAScraper_Login_ReplayError403BotDetection_Integration`
- `TestBBVAScraper_Login_ReplayErrorInvalidCredentialsLegacy_Integration`

---

## Implementation Order

1. Add `bank.Credentials` to `types.go` (no breakage)
2. Add cookie popup selector to `selectors.go` (no breakage)
3. Add `NewPage()` method (no breakage)
4. Add `dismissCookiePopup` helper (no breakage)
5. Refactor `Login` signature + `fillLoginForm` + remove `bbva.Credentials` (breaking)
6. Update `BankScraper` interface (breaking)
7. Update tests (fix breakage)
8. Run `go build ./...` and `go test ./... -short` to verify

## Verification

```bash
go build ./...
go test ./internal/scraper/bank/bbva/... -short
SCRAPER_TEST_MODE=replay go test ./internal/scraper/bank/bbva/... -v
go vet ./...
```
