# ADR-006: Credential Testing Wired in v1

### **Context** 💭
The PRD requires the ability to test bank credentials before saving them (FR-1204). This means the Credential Manager must be able to attempt a login against the actual bank portal to verify the credentials are valid. The BBVA scraper's `Login()` method is already implemented and live-tested. The question is whether to wire this integration into v1 or defer it.

### **Alternatives** ⚖️
1. **Wire in v1:** The Credential Manager imports the scraper package and calls `bbva.NewScraper()` → `Login()` → `Logout()` → `Close()` to validate credentials before saving. Tight coupling between modules, but delivers the full PRD requirement immediately.
2. **Defer to v2:** Build the CRUD, crypto, and audit layers first. Add credential testing as a later milestone once the API layer connects the scraper and Credential Manager, potentially via an internal API call rather than direct import.

### **Decision** 💪
We will **wire credential testing into v1**. The `CredentialService.Test()` method will instantiate the appropriate bank scraper, call `Login()` to verify credentials, then `Logout()` and `Close()` to clean up. A `switch` on `bank.Code` dispatches to the correct scraper implementation (currently only BBVA).

### **Consequences**

✅ **Positivas:**
* Full PRD compliance from day one (FR-1204)
* Immediate feedback to admins — know if credentials work before committing them
* Catches common errors (wrong password, locked account) at configuration time rather than at runtime

❌ **Negativas:**
* Direct dependency between `credmgr/service` and `scraper/bank/bbva` packages — tighter coupling
* Credential testing launches a headless browser (resource-intensive, 30-60s timeout)
* The test endpoint needs special UX handling (loading indicator, generous timeout) since browser automation is slow
* If the bank portal is down, credential testing fails even with valid credentials — may confuse operators
