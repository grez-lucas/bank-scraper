# BBVA Webpage Behavior Reference

> Technical documentation of BBVA Net Cash portal behavior for scraper development and HAR replay testing.

## Overview

The BBVA Net Cash portal (`bbvanetcash.pe`) uses a hybrid authentication system with two parallel login flows. Post-login pages (2026 redesign) use Web Components with deeply nested shadow DOM, requiring special handling to extract data.

## Key Concepts

### Shadow DOM

Shadow DOM is a browser API that lets custom elements encapsulate their internal structure. An element's **shadow root** contains its private DOM tree that is invisible to `document.querySelector()` and `page.HTML()`.

```
<bbva-account-card>          ← host element (visible in page.HTML())
  #shadow-root (open)        ← shadow boundary (NOT in page.HTML())
    <div class="card">       ← shadow content (invisible to outer queries)
      <span>S/ 1,234.56</span>
    </div>
</bbva-account-card>
```

A plain `page.HTML()` call returns only:

```html
<bbva-account-card></bbva-account-card>
```

The actual balance text is hidden behind the shadow boundary.

### Web Components

BBVA uses the Polymer/Cells framework to build custom HTML elements (Web Components). Each component (e.g., `<bbva-btge-accounts-solution-page>`) has its own shadow root containing its template, styles, and child components. These nest deeply — 5+ levels is common.

### Micro-Frontends

BBVA loads different sections of the portal as independent micro-applications inside iframes. The outer page is a Polymer app shell; the actual content lives in an iframe loaded from a different path. These iframes are themselves Web Components with their own shadow DOM trees.

### Light DOM vs Shadow DOM

- **Light DOM**: Regular DOM visible via `page.HTML()`, `document.querySelector()`, etc.
- **Shadow DOM**: Private DOM inside a shadow root. Must be accessed via `element.shadowRoot` in JavaScript.
- **Slotted content**: Light DOM children projected into shadow DOM via `<slot>` elements. These remain in the light DOM but render inside the shadow tree.

The scraper's `FlattenShadowDOM()` function inlines all shadow content into the light DOM as `<div data-shadow-root="true">` containers, making it accessible to goquery parsers.

## Shadow DOM Architecture

### Post-Login Component Tree

All post-login pages share this structure. The actual data lives 5+ levels deep behind shadow roots and an iframe.

```
document (light DOM — empty custom element shells)
  → <bbva-btge-{page}-solution-page>
      .shadowRoot
        → <bbva-core-iframe>
            .shadowRoot
              → <iframe src=".../accounts/index.html#!/">  (same-origin)
                  → contentDocument
                    → <bbva-btge-{page}-solution-home-page>
                        .shadowRoot
                          → 30-40 nested shadow roots with actual data
```

### Dashboard

```
URL: /nextgenempresas/portal/index.html#!/bbva-btge-dashboard-solution/

document
  → bbva-btge-dashboard-solution-page.shadowRoot
    → bbva-core-iframe.shadowRoot
      → <iframe> (same-origin, ~70KB)
        → contentDocument
          → bbva-btge-dashboard-solution-home-page.shadowRoot
            → Account cards, balance summaries, sidebar navigation
```

After login, the dashboard shows:
- Balances for **all accounts** inline (no navigation needed)
- Sidebar with navigation: Accounts, Transfers, etc.
- Logout button in the sidebar

### Accounts Page

```
URL: /nextgenempresas/portal/index.html#!/bbva-btge-accounts-solution/

document
  → bbva-btge-accounts-solution-page.shadowRoot
    → bbva-core-iframe.shadowRoot
      → <iframe> (same-origin, ~70KB)
        → contentDocument
          → bbva-btge-accounts-solution-home-page.shadowRoot
            → Currency tabs: "Todas las divisas", "Cuentas S/", "Cuentas $"
            → Account cards with balances
            → Latest 10 movements per account
            → "Ver todos los movimientos" links
```

Key elements found inside nested shadow roots:
- `"Todas las divisas"` — currency filter tabs
- `"Cuentas S/"`, `"Cuentas $"` — PEN/USD account groups
- `"PEN"`, `"USD"` — currency labels on account cards
- Balance amounts per account
- Total balance by currency

### Transactions Page

```
URL: /nextgenempresas/portal/index.html#!/bbva-btge-accounts-solution/account/{id}/movements

document
  → bbva-btge-accounts-solution-page.shadowRoot
    → bbva-core-iframe.shadowRoot
      → <iframe> (same-origin)
        → contentDocument
          → Transaction list with date, description, amount, balance
          → "Ver mas" pagination button (loads next 50)
```

- Full history back to **January of the previous year**
- **50 transactions per page**
- **"Ver mas" button** for pagination

#### Transactions Table Structure (2026)

The table container is `bbva-btge-accounts-solution-table#moviments-table`. Key attributes on the container:

| Attribute | Has Data | Empty |
|-----------|----------|-------|
| `total-items` | `"56"` | `"0"` |
| `state` | `""` | `"noresults"` |

The `<tbody>` contains two kinds of `<tr>` elements:

1. **Date group separators** — plain `<tr>` (no `.row` class) with a `bbva-table-row-group` element showing the running balance at that date. These are NOT transactions and must be skipped.
2. **Transaction rows** — `<tr class="row" data-actionable="">` containing the actual transaction data.

Each transaction row has these cells:

| Cell | Component | Class | Key Attributes |
|------|-----------|-------|----------------|
| Operation date | `bbva-table-body-date` | `.operationDate` | `date="10 Feb"`, `year="2026"` |
| Value date | `bbva-table-body-date` | `.valueDate` | `date="10 Feb"`, `year="2026"` |
| Code | `bbva-table-body-text` | `.code` | `text="151"` |
| Movement number | `bbva-table-body-text` | `.numberMovement` | `text="1411"` |
| Concept | `bbva-table-body-text` | `.concept` | `text="PAGO FACTURA"`, `description="*Mp: 2060..."` |
| Financeable | `bbva-table-body-action` | `.financeable` | (action button, not data) |
| Amount | `bbva-table-body-amount` | `.transactionAmount` | `amount="-3.5"`, `secondary-amount="8577.97"` |

**Amount conventions:**
- Negative = debit (money out), positive = credit (money in)
- `amount-variant="income"` is present on credit rows (but the sign is sufficient)
- `secondary-amount` = running balance after the transaction

**Concept has two parts:**
- `text` attribute = main concept (e.g., `"PAGO FACTURA | SUNAT DETRACCIONES"`)
- `description` attribute = beneficiary detail (e.g., `"*Mp: 20607818054S Com Sunat Detraccione@"`)

**Date format:** Two attributes (`date` + `year`) combine to `"10 Feb 2026"`, parsed with Go layout `"02 Jan 2006"`. This replaces the old `DD-MM-YYYY` format.

**Removed columns:** The old "Oficina" (branch office) column no longer exists in the 2026 table.

#### Parsing Strategy

All data is in **element attributes**, not text content. The parser:

1. Finds `#moviments-table` — if missing, return `ErrParsingFailed`
2. Checks `state="noresults"` — if so, return empty slice
3. Iterates `tr.row[data-actionable]` (skips date-group separator rows)
4. Per row, reads attributes from the typed cell components (`.operationDate`, `.code`, `.concept`, `.transactionAmount`, etc.)

```
Table → check state → iterate tr.row[data-actionable] → read attrs → []Transaction
```

### Login Page (Unchanged)

The login page does NOT use shadow DOM. It is a traditional HTML page with all elements in the light DOM. No flattening is needed for login.

```
URL: /DFAUTH85/mult/KDPOSolicitarCredenciales_es.html

document
  → input#empresa           (light DOM, visible)
  → input#usuario           (light DOM, visible)
  → input#clave_acceso_ux   (light DOM, visible)
  → button#aceptar          (light DOM, legacy flow)
  → button#enviarSenda      (light DOM, micro-frontend flow)
  → iframe#microfrontend    (hidden, used by Senda auth)
```

## Login Flows

### Senda Flow (Primary — Used by Scraper)

```
User fills form -> Clicks #enviarSenda -> preventDefault()
  -> postMessage(credentials) to iframe#microfrontend
  -> Iframe JS calls Senda API:
    1. POST grantingTicket/V02 (pre-auth, authenticationType=61)
    2. DELETE grantingTicket/V02 (clear pre-auth)
    3. POST grantingTicket/V02 (auth, authenticationType=16, full creds)
  -> Iframe postMessages result back to parent
  -> Parent JS _validateLoginError(event, detail):
    - DO_REDIRECT_SENDA -> location.href = portalURL (success!)
    - LOGIN_ERROR -> span#error-message shows error text
    - BLOQUED_USER -> span#error-message shows blocked text
    - DO_LOGIN_LEGACY / UNKNOWN_LOGIN_ERROR -> buttonLegacy.click() (DFServlet fallback)
```

| Step | Details |
|------|---------|
| Button | `button#enviarSenda` (visible "Ingresar" button) |
| Auth API | `asosenda.bbva.pe/TechArchitecture/pe/grantingTicket/V02` |
| Success | `DO_REDIRECT_SENDA` → JS redirects to `/nextgenempresas/portal/` |
| Invalid credentials | Senda API 403 (error-code 160/161) → `span#error-message` shows error text |
| User blocked | Senda error-code 162/164 → `span#error-message` shows blocked text |
| Bot detection | Akamai blocks at edge (403) before Senda API is reached |

**Senda Error Messages** (shown in `span#error-message`):

| Error Code | Message |
|------------|---------|
| EA160, EAI0000, USER_NOT_EXIST | "Es necesario que corrijas los datos que ingresaste para poder continuar." |
| EA161, INVALID_PASSWORD | "Es necesario que corrijas los datos que ingresaste para poder continuar." |
| EA162 | "Tu usuario está bloqueado..." |
| EA164 | "Los datos ingresados son incorrectos. Alcanzaste el límite de intentos." |
| UNKNOWN_LOGIN_ERROR | "Usuario no disponible." |

**Scraper detection:** The scraper polls for two conditions after clicking `#enviarSenda`:
1. **Success:** URL changes to contain `/nextgenempresas/portal/` (portal redirect)
2. **Error:** `span#error-message` becomes visible with non-empty text

### Legacy DFServlet Flow (Deprecated)

```
User fills form -> Clicks #aceptar -> POST to DFServlet -> Server response
```

| Step | Details |
|------|---------|
| Button | `button#aceptar` (hidden, 5x5px transparent) |
| Endpoint | `POST /DFAUTH85/slod_pe_web/DFServlet` |
| Success | HTTP 302 -> redirect to dashboard |
| Invalid credentials | HTTP 200 with error HTML page |
| Bot detection | HTTP 403 (Akamai WAF) |

**Note:** The legacy button is deprecated. BBVA hides it (5x5px transparent) and routes through `#enviarSenda` by default. The scraper previously used `#aceptar` but switched to `#enviarSenda` because HAR recordings captured with the Senda flow are replayable.

## Key Endpoints

### DFServlet (Form Submission)

```
URL:    https://www.bbvanetcash.pe/DFAUTH85/slod_pe_web/DFServlet
Method: POST
```

**Response Codes:**

| Status | Meaning | Action |
|--------|---------|--------|
| 302 | Success | Follow redirect to dashboard |
| 200 | Error page | Parse HTML for error code/message |
| 403 | Bot detection | Return `ErrBotDetection` |
| 503 | Service unavailable | Retry with backoff |

### Senda API (Micro-Frontend Auth)

```
URL:    https://asosenda.bbva.pe/TechArchitecture/pe/grantingTicket/V02
Method: POST
```

Used by micro-frontend flow. Returns 403 on invalid credentials.

## Error Detection

### Senda Flow Error Detection (Primary)

The Senda flow shows errors via `span#error-message` on the login page. The scraper detects errors by evaluating the element's visibility and text content:

```go
SelectorLoginErrorSpan = "span#error-message"
```

The scraper classifies error text into typed errors:

| Error text contains | Typed error |
|---------------------|-------------|
| "corrijas los datos" | `ErrInvalidCredentials` |
| "bloqueado" | `ErrInvalidCredentials` |
| "límite de intentos" | `ErrInvalidCredentials` |
| "no disponible" | `ErrBankUnavailable` |
| (anything else) | `ErrUnknown` |

### Legacy Error Page HTML Structure (DFServlet)

When DFServlet returns 200 with an error, the HTML contains:

```html
<!-- Comment identifies error page -->
<!-- errorBasicoPIBEE_CAS -->

<!-- Error code -->
<div class="error-code error-title">
    EAI0000
</div>

<!-- Error message -->
<h1 class="title">No pudimos iniciar tu sesion</h1>
```

### CSS Selectors for Legacy Error Detection

```go
SelectorLoginErrorCode    = "div.error-code.error-title"
SelectorLoginErrorMessage = "h1.title"
```

### Known Error Codes

| Code | Meaning |
|------|---------|
| `EAI0000` | Invalid credentials (user/company not found) |
| `EA160` | User not exist |
| `EA161` | Invalid password |
| `EA162` | User blocked |
| `EA164` | Token blocked (too many attempts) |

## Request Explosion Problem (Legacy Flow Only)

> **Note:** This section applies to the legacy DFServlet flow. The Senda flow does not have this problem because the scraper detects success/failure via URL change and `span#error-message` text, not HTTP status codes.

### The Issue

After DFServlet form submission, the browser fires many async requests whose 200 status codes can overwrite the captured DFServlet status.

### Legacy Solution

The legacy flow filtered status codes by DFServlet path. The Senda flow avoids this entirely by polling the DOM instead of capturing HTTP status codes.

## Bot Detection (Akamai WAF)

### Triggers

- Automated browser detection (missing human-like behavior)
- Rapid repeated requests
- Missing or suspicious headers
- Known bot signatures

### Response

```
HTTP/1.1 403 Forbidden
Server: AkamaiGHost
```

The response body may contain Akamai challenge JavaScript.

### Mitigation

1. Use stealth mode in Rod launcher
2. Add human-like delays between actions
3. Randomize typing speed
4. Avoid rapid repeated login attempts

## HAR Replay Testing

### Recording Requirements

1. **Use `#enviarSenda` button** during recording to capture Senda API traffic
2. **Wait for full page load** before stopping recording
3. **Capture all requests** including Senda API calls (grantingTicket/V02)

### Replayer Method-Aware Matching

The Senda flow makes 3 requests to the same grantingTicket URL with different methods (POST, DELETE, POST). The replayer uses method-aware indexing to disambiguate:

1. Method + exact URL (best match)
2. Method + path only
3. Exact URL (fallback, ignores method)
4. Path only (fallback)

### Known Limitations

1. **JavaScript state**: DOM changes from JS execution (e.g., `span#error-message` text) aren't in HAR — the browser must execute the page JS
2. **Session cookies**: May need to be sanitized or regenerated

### HAR Files

| File | Scenario | Key Response |
|------|----------|--------------|
| `login-success.har.json` | Successful Senda login | grantingTicket POST 200 → portal redirect |
| `login-bot-detection.har.json` | Akamai blocked | Needs re-recording with `#enviarSenda` |
| `login-invalid-credentials-legacy.har.json` | Wrong credentials (Senda) | grantingTicket POST 403 → span#error-message |
| `get-balance.har.json` | Login + accounts page | Full Senda login + accounts navigation |

## Announcement Modal

After login (and sometimes on page navigation), a news/announcement modal (`bbva-btge-microfrontend-modal`) may appear with `opened=""` attribute. This is **not** a cookie consent popup — it is a promotional/news overlay.

**Detection**: Check for `bbva-btge-microfrontend-modal[opened]` in the page HTML. The `opened` attribute is only present when the modal is visible.

**Dismissal**: Click `button.close-btn` inside the opened modal. The close button exists in the DOM even when the modal is closed, so always check for the `[opened]` attribute first.

**Fixtures with modal open**: `login_popup.html`, `dashboard_news_popup.html`, `accounts_news_popup.html`
**Fixtures without modal**: `dashboard.html`, `accounts_list.html`, `transactions.html`

**Shadow DOM note**: In the live browser, `button.close-btn` may live inside shadow DOM. Rod can traverse shadow boundaries via CDP. If the CSS selector doesn't cross shadow boundaries at runtime, fall back to `modal.ShadowRoot().Element("button.close-btn")`.

## Logout Flow

Logout is no longer a direct link/redirect:

1. Click logout button in the sidebar
2. Confirmation modal appears ("Are you sure?")
3. Confirm to complete logout

## Scraper Impact

| Component | Action Required |
|-----------|----------------|
| `browser/shadow.go` | **Done** — `FlattenShadowDOM()` inlines shadow DOM + iframes into single HTML |
| `capture-fixtures/main.go` | **Done** — uses `FlattenShadowDOM()` instead of iframe-only inlining |
| `selectors.go` | New selectors needed after capturing flattened fixtures |
| `parser.go` | New parsers for accounts page, dashboard balances, full history pagination |
| `scraper.go` | Call `FlattenShadowDOM()` before passing HTML to parsers, dismiss announcement modal after login |
| Fixtures | Re-capture all post-login fixtures with shadow DOM flattener |
| HAR recordings | Record new scenarios for dashboard, accounts, transactions, logout |

## Summary: Differentiating Error Types

### Senda Flow (Primary)

| Signal | Error Type |
|--------|------------|
| URL → PortalPath + no dashboard | `ErrUnknown` |
| `span#error-message` contains "corrijas los datos" | `ErrInvalidCredentials` |
| `span#error-message` contains "bloqueado" | `ErrInvalidCredentials` |
| `span#error-message` contains "límite de intentos" | `ErrInvalidCredentials` |
| `span#error-message` contains "no disponible" | `ErrBankUnavailable` |
| Timeout (no redirect, no error span) | `ErrUnknown` |
| Connection/timeout errors | `ErrBankUnavailable` |

### Legacy DFServlet Flow (Deprecated)

| Signal | Error Type |
|--------|------------|
| DFServlet 403 | `ErrBotDetection` |
| DFServlet 200 + error HTML | `ErrInvalidCredentials` |
| DFServlet 302 + no dashboard | `ErrUnknown` |

---

## Iframe Discovery Output

> Output from `scripts/discover-iframes` run against the live portal (Feb 2026).
> Note: The discover tool only sees light DOM elements. All post-login pages show "(no known selectors found)" because content is inside shadow roots, not queryable from the outer document.

### Login Page

```
URL: /DFAUTH85/mult/KDPOSolicitarCredenciales_es.html

FOUND  Company input                   input#empresa  (visible=true)
FOUND  User input                      input#usuario  (visible=true)
FOUND  Password input                  input#clave_acceso_ux  (visible=true)
FOUND  Login button (legacy)           button#aceptar  (visible=true)
FOUND  Login button (senda)            button#enviarSenda  (visible=true)
FOUND  Login error span                span#error-message  (visible=true)

IFRAME main > iframe#microfrontend  visible=false
  (Senda auth iframe — not used by scraper)
```

### Post-Login Pages (Dashboard, Accounts, Transactions, Logout)

All post-login pages show the same pattern: no selectors found in light DOM, only a Google Tag Manager iframe visible. This is because all content is inside shadow roots.

```
URL: /nextgenempresas/portal/index.html#!/bbva-btge-{page}-solution/

(no known selectors found in light DOM)

IFRAME main > iframe[0]  visible=false  src=
  (Google Tag Manager service worker iframe)
```

To discover selectors inside shadow DOM, use the capture-fixtures tool with `FlattenShadowDOM()` and inspect the resulting HTML fixtures.
