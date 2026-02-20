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

### Legacy Flow (Used by Scraper)

```
User fills form -> Clicks #aceptar -> POST to DFServlet -> Server response
```

| Step | Details |
|------|---------|
| Button | `button#aceptar` (hidden, 5x5px) |
| Endpoint | `POST /DFAUTH85/slod_pe_web/DFServlet` |
| Success | HTTP 302 -> redirect to dashboard |
| Invalid credentials | HTTP 200 with error HTML page |
| Bot detection | HTTP 403 (Akamai WAF) |

### Micro-Frontend Flow (Modern UI)

```
User fills form -> Clicks #enviarSenda -> postMessage to iframe -> Senda API -> JS updates DOM
```

| Step | Details |
|------|---------|
| Button | `button#enviarSenda` (visible "Ingresar" button) |
| Auth API | `asosenda.bbva.pe/TechArchitecture/pe/grantingTicket/V02` |
| Success | postMessage response -> JS redirects |
| Invalid credentials | Senda API 403 -> JS shows error in `#error-message` span |
| Bot detection | Same as legacy (Akamai blocks at edge) |

**Important:** The scraper uses the legacy flow because:

1. Direct server responses are easier to capture and parse
2. postMessage between windows is not captured in HAR recordings
3. iframe authentication state is complex to replay

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

### Error Page HTML Structure

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

### CSS Selectors for Error Detection

```go
SelectorLoginErrorCode    = "div.error-code.error-title"
SelectorLoginErrorMessage = "h1.title"
SelectorLoginErrorSpan    = "span#error-message.coronita-small-icon-warning.icon-info-svg-warning.span-error"
```

### Known Error Codes

| Code | Meaning |
|------|---------|
| `EAI0000` | Invalid credentials (user/company not found) |
| `EA160` | User not exist |
| `EA161` | Invalid password |
| `EA162` | User blocked |
| `EA164` | Token blocked (too many attempts) |

## Request Explosion Problem

### The Issue

After form submission, the browser fires many async requests:

```
DFServlet POST (403) -+-> wup-stats (200)
                      +-> analytics (200)
                      +-> Adobe DTM (200)
                      +-> tracking pixels (200)
                      +-> ... many more
```

If capturing status codes naively, the 403 from DFServlet gets overwritten by subsequent 200s.

### Solution

Only capture status codes from the DFServlet path:

```go
const dfServletPath = "/DFAUTH85/slod_pe_web/DFServlet"

router.MustAdd("*", func(ctx *rod.Hijack) {
    // Process request...

    // Only capture status for form submission, ignore analytics
    if ctx.Response != nil && strings.Contains(ctx.Request.URL().Path, dfServletPath) {
        statusCode = ctx.Response.Payload().ResponseCode
    }
})
```

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

1. **Use legacy button** (`#aceptar`) during recording to capture DFServlet responses
2. **Wait for full page load** before stopping recording
3. **Capture all requests** including redirects

### Known Limitations

1. **postMessage not captured**: Micro-frontend iframe communication isn't in HAR
2. **JavaScript state**: DOM changes from JS execution aren't recorded
3. **Session cookies**: May need to be sanitized or regenerated

### HAR Files

| File | Scenario | Key Response |
|------|----------|--------------|
| `login-success.har.json` | Successful login | DFServlet 302 -> dashboard |
| `login-bot-detection.har.json` | Akamai blocked | DFServlet 403 |
| `login-invalid-credentials-legacy.har.json` | Wrong credentials | DFServlet 200 + error HTML |

## Cookie Consent Popup

The login page shows a cookie consent popup on first visit. The scraper must dismiss it before interacting with the login form. The popup is non-blocking (login form is still in the DOM) but may overlap input fields.

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
| `scraper.go` | Call `FlattenShadowDOM()` before passing HTML to parsers, add cookie popup dismissal |
| Fixtures | Re-capture all post-login fixtures with shadow DOM flattener |
| HAR recordings | Record new scenarios for dashboard, accounts, transactions, logout |

## Summary: Differentiating Error Types

| Signal | Error Type |
|--------|------------|
| DFServlet 403 | `ErrBotDetection` |
| DFServlet 200 + error HTML | `ErrInvalidCredentials` |
| DFServlet 302 + no dashboard | `ErrUnknown` |
| Senda API 403 (micro-frontend) | `ErrInvalidCredentials` |
| Connection/timeout errors | `ErrBankUnavailable` |

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
