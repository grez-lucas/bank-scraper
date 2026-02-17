# BBVA Webpage Behavior Reference

> Technical documentation of BBVA Net Cash portal behavior for scraper development and HAR replay testing.

## Overview

The BBVA Net Cash portal (`bbvanetcash.pe`) uses a hybrid authentication system with two parallel login flows. Understanding these flows is critical for reliable scraping and test replay.

## Login Flows

### Legacy Flow (Used by Scraper)

```
User fills form → Clicks #aceptar → POST to DFServlet → Server response
```

| Step | Details |
|------|---------|
| Button | `button#aceptar` (hidden, 5x5px) |
| Endpoint | `POST /DFAUTH85/slod_pe_web/DFServlet` |
| Success | HTTP 302 → redirect to dashboard |
| Invalid credentials | HTTP 200 with error HTML page |
| Bot detection | HTTP 403 (Akamai WAF) |

### Micro-Frontend Flow (Modern UI)

```
User fills form → Clicks #enviarSenda → postMessage to iframe → Senda API → JS updates DOM
```

| Step | Details |
|------|---------|
| Button | `button#enviarSenda` (visible "Ingresar" button) |
| Auth API | `asosenda.bbva.pe/TechArchitecture/pe/grantingTicket/V02` |
| Success | postMessage response → JS redirects |
| Invalid credentials | Senda API 403 → JS shows error in `#error-message` span |
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
<h1 class="title">No pudimos iniciar tu sesión</h1>
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
DFServlet POST (403) ─┬─→ wup-stats (200)
                      ├─→ analytics (200)
                      ├─→ Adobe DTM (200)
                      ├─→ tracking pixels (200)
                      └─→ ... many more
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
| `login-success.har.json` | Successful login | DFServlet 302 → dashboard |
| `login-bot-detection.har.json` | Akamai blocked | DFServlet 403 |
| `login-invalid-credentials-legacy.har.json` | Wrong credentials | DFServlet 200 + error HTML |

## Dashboard Detection

After successful login, verify by checking for dashboard element:

```go
SelectorDashboard = "table#kyop-boby-table.kyop-boby-table"
```

**Important:** Call `page.MustWaitDOMStable()` before checking, as post-login JavaScript needs time to render.

## Summary: Differentiating Error Types

| Signal | Error Type |
|--------|------------|
| DFServlet 403 | `ErrBotDetection` |
| DFServlet 200 + error HTML | `ErrInvalidCredentials` |
| DFServlet 302 + no dashboard | `ErrUnknown` |
| Senda API 403 (micro-frontend) | `ErrInvalidCredentials` |
| Connection/timeout errors | `ErrBankUnavailable` |
