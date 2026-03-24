# ADR-014: Resilience — Retry and Circuit Breaker

### **Context** 💭

Bank portals are inherently unreliable — they experience intermittent outages, slow responses, bot detection triggers, and session timeouts. The PRD requires automatic retry with exponential backoff (FR-801, FR-802: 3 attempts, 1s initial, 30s max delay) and per-bank circuit breakers (FR-804 through FR-807: open after 5 consecutive failures, half-open after 5 minutes, fail fast when open). The resilience layer must distinguish between transient errors (worth retrying) and permanent errors (fail immediately).

### **Alternatives** ⚖️

1. **Decorator pattern on `bank.Scraper`:** Wrap each scraper in a `ResilientScraper` that adds retry + circuit breaker to every method. Clean separation but ties circuit breaker state to the scraper instance lifecycle — if the session manager recreates the scraper (on session expiry), the circuit breaker state is lost.
2. **Handler-level resilience:** Apply retry and circuit breaker at the API handler level, wrapping the entire "get scraper + call method" operation. Circuit breaker state lives in the handler (or a shared registry), independent of scraper instance lifecycle. Allows the retry to include session invalidation + re-login as part of the retry flow.
3. **Middleware-level resilience:** A Gin middleware that wraps handler responses. Too coarse — can't distinguish between different types of errors or apply per-bank circuit breakers.

### **Decision** 💪

We will apply resilience at the **handler level** using `cenkalti/backoff/v4` for retry and `sony/gobreaker` for circuit breaking.

**Retry strategy:**
- 3 maximum attempts with exponential backoff (1s initial, 30s max delay)
- **Retryable errors:** `ErrBankUnavailable`, `ErrTimeout`, `ErrSessionExpired` (after calling `manager.Invalidate()` to trigger re-login on next attempt)
- **Permanent errors (no retry):** `ErrInvalidCredentials`, `ErrBotDetection`, `ErrAccountNotFound`, `ErrParsingFailed`
- Wrapped with `backoff.Permanent()` for non-retryable errors

**Circuit breaker:**
- One `gobreaker.CircuitBreaker` instance per bank code, stored in a shared registry
- Opens after 5 consecutive failures
- Half-open after 5 minutes (attempts one request to test recovery)
- When open, immediately returns `HTTP 503 Service Unavailable`
- Resets to closed on first successful request in half-open state

**Composition in handlers:**
```
circuitBreaker.Execute(func() {
    backoff.Retry(func() {
        scraper = manager.GetScraper(ctx, bankCode)
        result = scraper.GetBalance(ctx)
    })
})
```

### **Consequences**

✅ **Positivas:**

* Circuit breaker state survives scraper recreation — not tied to instance lifecycle
* Retry can include session recovery (invalidate + re-login) as part of the retry loop
* Clear error classification — retryable vs permanent is explicit
* Per-bank isolation — BBVA circuit breaking doesn't affect Interbank
* Handler has full context to map final errors to appropriate HTTP status codes

❌ **Negativas:**

* Retry + circuit breaker logic is in handler code rather than transparently applied — each handler must use the pattern
* Circuit breaker registry is an additional shared-state component to manage
* Exponential backoff adds latency — worst case: 1s + 2s + 4s = 7s extra before returning an error (still within 30s SLA)
