# TODO: BBVA Scraper — Next Steps

## Roadmap

1. ~~**GetBalance** — scraper method to fetch account balances~~ **DONE**
2. ~~**GetTransactions** — scraper method to fetch transaction history~~ **DONE** (live-tested, pending transient bank error handling)
3. **API layer** — REST API exposing Login, GetBalance, GetTransactions to AyniFX

## What's Already Done

| Component | Status |
|-----------|--------|
| `Login` | Working — HAR replay tested (success, invalid creds, re-login). Uses Senda `#enviarSenda` flow. Anti-detection: stealth launcher flags, human-like typing, random delays. |
| `Login` replay (Senda probe) | Working — separate-tab `probeSendaAPI` bypasses broken iframe postMessage chain in replay mode. |
| `waitForLoginOutcome` | Working — dual-path: probe for replay mode, DOM polling (URL redirect + span#error-message) for live mode. |
| `classifySendaError` | Working — handles both UI text (live) and API probe format. Unit tested. |
| `classifySendaErrorCode` | Working — maps Senda error codes (160, 162) to typed errors. Unit tested. |
| HijackRouter context fix | Working — router created before `page.Timeout()` to prevent context kill. |
| `GetBalance` | **Working (live-tested)** — navigates to accounts page, dismisses modal, waits for Web Components, flattens shadow DOM + iframes, delegates to parser. |
| `GetTransactions` | **Working (live-tested)** — navigates to transactions page, waits for table render, flattens shadow DOM + iframes, pagination loop with "Ver más". Returns empty slice on bank error page. |
| `ParseAccountBalances` | Working — unit tested against `accounts_list` and `accounts_tile` fixtures |
| `ParseTransactions` | Working — unit tested against `transactions` fixture (50 rows) |
| `HasMoreTransactions` | Working — detects "Ver más" pagination footer |
| `DetectAnnouncementModal` | Working — unit tested against 6 fixtures + edge cases |
| `dismissAnnouncementModal` | Working — uses `deepQuery` to find buttons through nested shadow roots. Called after every `navigateTo`. |
| `navigateTo` helper | Working — navigates via `about:blank` (forces full reload after FlattenShadowDOM mutations), waits for load + DOM stable, dismisses modal |
| `waitForAccountsReady` | Working — polls with `DeepQueryExists`/`DeepQueryAttr` (crosses shadow DOM + iframes) |
| `waitForTransactionsReady` | Working — detects data rows, `noresults`, and error states |
| `deepQuery` (`browser/query.go`) | Working — JS helper that walks shadow DOM, light DOM, and iframe boundaries. Used by `DeepQueryExists`, `DeepQueryClick`, `DeepQueryAttr`. |
| `FlattenShadowDOM` | Working — in `browser/shadow.go`, tested |
| Page lifecycle | Working — page stored on `BBVAScraper`, cleaned up on failure/re-login/close |
| Hijacker lifetime | Working — router stored on `BBVAScraper`, kept alive between operations |
| Selectors | All defined in `selectors.go` for accounts (list + tile) and transactions |
| Debug diagnostics | Working — screenshots + HTML dumps + deepQuery diagnostics written to `os.TempDir()/bbva-debug/` on timeouts |
| Context handling | Idiomatic Go contexts throughout: `page.Context(ctx)` + `context.WithTimeout`, per-phase deadlines |

## Test Status

| Test | Mode | Status |
|------|------|--------|
| `TestBBVAScraper_Login_ReplaySuccess_Integration` | replay | PASS |
| `TestBBVAScraper_Login_ReplayErrorInvalidCredentials_Integration` | replay | PASS |
| `TestBBVAScraper_Login_ReplayRelogin_Integration` | replay | PASS |
| `TestBBVAScraper_Login_ReplayError403BotDetection_Integration` | replay | SKIP — needs re-recorded HAR with `#enviarSenda` |
| `TestBBVAScraper_GetBalance_Replay_Integration` | replay | SKIP — portal SPA can't initialize in CDP Fetch replay |
| `TestBBVAScraper_Live_Login` | live | PASS |
| `TestBBVAScraper_Live_GetBalance` | live | PASS |
| `TestBBVAScraper_Live_GetTransactions_FirstAccount` | live | PASS |
| `TestBBVAScraper_Live_GetTransactions_SecondAccount` | live | PASS (intermittent bank error page) |
| `TestClassifySendaError` | mock | PASS (9 cases) |
| `TestClassifySendaErrorCode` | mock | PASS (4 cases) |
| All parser unit tests | mock | PASS |

---

## ~~Pending: Wire `ctx context.Context` Through Scraper~~ DONE

Refactored all scraper methods to use idiomatic Go contexts:
- `page.Timeout()`/`CancelTimeout()` replaced with `page.Context(ctx)` + `context.WithTimeout(ctx, ...)`
- All poll loops use `select` on `ctx.Done()` via derived contexts
- `MustWaitDOMStable()` replaced with `WaitDOMStable(time.Second, 0)`
- Each method has per-phase contexts: navigation → wait → flatten/parse
- `dismissAnnouncementModal` uses a 3s derived context
- `probeSendaAPI` uses `context.WithTimeout`

---

## Blocked: GetBalance / GetTransactions Replay Tests

The portal SPA (Cells/Polymer framework) **cannot initialize in CDP Fetch replay mode**. CDP Fetch's `FetchFulfillRequest` bypasses cookie processing — `Set-Cookie` headers from replayed responses are NOT stored in the browser. The Cells framework's bootstrap chain requires auth cookies and `tsec` session tokens set during the real login redirect, so Custom Elements never register and the page renders empty shells with no shadow DOM content.

**Possible approaches:**
1. **Direct API probe** (like login) — identify the internal API endpoints the portal calls for balance/transaction data and probe them directly via the separate-tab technique. Most promising.
2. **Re-architect replay** to use a local HTTP proxy instead of CDP Fetch, so cookies are processed normally. High effort.
3. **Accept live-only integration tests** for GetBalance/GetTransactions, rely on unit-tested parsers for correctness. Pragmatic fallback.

---

## Pending: Re-record Bot Detection HAR

`login-bot-detection.har.json` was recorded with the legacy `#aceptar` button. Needs re-recording with `#enviarSenda` to match the current scraper flow. The test is skipped until then.

---

## Pending: Transactions Bank Error Handling

The bank intermittently returns "La información no está disponible" on the transactions page. Current behavior: `waitForTransactionsReady` detects the terminal state (no timeout), then `ParseTransactions` returns an empty slice.

**Improvements needed:**
1. Detect the error state explicitly (look for error message text or specific `state` attribute value)
2. Return a typed error like `bank.ErrBankUnavailable` instead of an empty slice
3. Consider retry logic — the error is transient ("inténtalo de nuevo más tarde")

---

## Pending: GetBalance Returns Tile View Only

Live testing shows `GetBalance` returns tile view balances (available only, `current=0`). The list view has both available and accounted balances. Consider:
1. Switching to list view before scraping (click the list toggle button)
2. Or accepting tile view data if available balance is sufficient

---

## Lessons Learned

### DOM Traversal: Three Boundaries

BBVA's portal has three DOM boundaries that standard `querySelector` cannot cross:
1. **Shadow DOM** — `element.shadowRoot`
2. **Light DOM (slotted)** — Polymer projects children via `<slot>`
3. **Iframes** — micro-frontends load in same-origin iframes nested inside shadow roots

The `deepQuery` helper (`browser/query.go`) crosses all three. This was the key breakthrough — initial implementations only crossed shadow DOM, causing 30s timeouts because account/transaction elements live inside iframes.

### FlattenShadowDOM Mutates the DOM

`FlattenShadowDOM()` inlines shadow content and replaces iframes with divs. This **destroys the SPA's DOM structure**. Hash-only URL changes (SPA routing) don't reload the page, so the SPA can't recover. Fix: `navigateTo` navigates to `about:blank` first, forcing a full page reload.

### Modal Buttons in Nested Shadow Roots

The announcement modal's buttons are inside nested shadow roots (modal → shadowRoot → bbva-button → shadowRoot → `<button>`). `Element.matches()` with descendant selectors cannot see ancestors across shadow boundaries. Fix: use `deepQuery(modal, 'button')` to walk the modal's entire shadow tree.
