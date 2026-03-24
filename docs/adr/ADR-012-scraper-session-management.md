# ADR-012: Scraper Session Management — Lazy Singleton

### **Context** 💭

The bank scraper is stateful — it holds a headless browser instance and an authenticated session. BBVA sessions last approximately 10 minutes before timing out. The login process takes 15-20 seconds (browser launch + page navigation + form submission + dashboard wait). The PRD requires API response times under 30 seconds (G6). With a single consumer (AyniFX), concurrent scraper access is not expected.

### **Alternatives** ⚖️

1. **Per-request browser:** Launch a new browser, login, scrape, logout, close for each API call. Simplest code but adds 15-20 seconds of login overhead per request. Combined with scraping time, most requests would exceed the 30-second SLA. Also increases load on bank portals.
2. **Session pool:** Maintain a pool of N authenticated scraper sessions per bank. Supports concurrent requests and provides redundancy. However, with a single consumer, concurrency is unlikely. The pool adds significant lifecycle complexity (health checking, eviction, pool sizing) for no practical benefit.
3. **Lazy singleton:** One scraper instance per bank, created on first request, reused until session expires. On expiry, automatically re-login. Mutex-protected to prevent concurrent login attempts.

### **Decision** 💪

We will use a **lazy singleton** pattern — one scraper instance per bank managed by a `session.Manager` component.

**Lifecycle:**
- `GetScraper(ctx, bankCode)` checks the internal map. If no scraper exists or the session has expired, it creates one via a `ScraperFactory`, fetches credentials via a `CredentialProvider` interface (satisfied by `CredentialService.GetCredentials()`), and calls `Login()`.
- The scraper is stored in a `map[bank.Code]*managedScraper` protected by a `sync.Mutex`.
- On `ErrSessionExpired` during an operation, the handler calls `Invalidate(bankCode)` which removes the scraper from the map. The next `GetScraper` call triggers a fresh login.
- On graceful shutdown, `Shutdown()` calls `Logout()` + `Close()` on all active scrapers.

**Thread safety:** The mutex serializes login attempts. Since Rod is not safe for concurrent page operations anyway, this is the correct granularity.

### **Consequences**

✅ **Positivas:**

* First request per bank pays the login cost (~15-20s); subsequent requests are fast (~5-10s scraping only)
* Simple lifecycle — no pool management, health checking, or eviction policies
* Natural fit for single-consumer model
* Automatic session recovery on expiry

❌ **Negativas:**

* First request after session expiry is slow (login overhead)
* Mutex means concurrent requests to the same bank are serialized (acceptable for single consumer)
* If the scraper enters a bad state (e.g., browser crash), `Invalidate` must be called to recover — no automatic health checking
