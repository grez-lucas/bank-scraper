# ADR-011: Account Discovery and Persistence

### **Context** 💭

The PRD exposes per-account endpoints (`GET /accounts/{account_id}/balance`, `GET /accounts/{account_id}/transactions`), but the BBVA scraper returns all account balances at once from a single page navigation — there is no per-account fetch. The API needs a stable `account_id` (UUID) that maps to a bank's internal account number, and it needs to know which accounts exist before a consumer can query them.

### **Alternatives** ⚖️

1. **Derive from credentials:** No separate accounts table. Infer account list from `bank_credentials` + cached scraper results. Simple but provides no stable identifiers, no metadata persistence, and no way to list accounts without scraping.
2. **Per-request discovery:** On each API call, scrape the bank to discover accounts dynamically. Accurate but wasteful — adds 10-20 seconds of unnecessary latency and redundant bank portal hits.
3. **DB-backed `accounts` table with scraper discovery:** A dedicated table stores discovered accounts with stable UUIDs. Discovery is triggered on credential create/update and via a manual admin endpoint. Scraper fetches all accounts, the service upserts them into the DB.

### **Decision** 💪

We will use a **DB-backed `accounts` table** populated via scraper discovery. The discovery flow is:

1. **On credential create/update** (from the Credential Manager): the credmgr handler triggers discovery as a best-effort background operation. It creates a dedicated scraper instance (not the API's live singleton), logs in, calls `GetBalance()` to discover all accounts, maps `[]bank.Balance` → `[]store.Account`, upserts into the DB, then logs out and closes.
2. **Manual admin endpoint** (`POST /api/v1/admin/discover/:bank_code`): allows operators to force re-discovery when banks add new accounts.

The `accounts` table stores `id` (UUID), `bank_code`, `account_number` (full, masked only in API responses), `currency`, `account_type`, `status`, `credential_id` (FK), and `last_synced_at`. A unique constraint on `(bank_code, account_number)` prevents duplicates; upserts update metadata on re-discovery.

### **Consequences**

✅ **Positivas:**

* Stable UUID identifiers for API consumers — decoupled from bank-internal account numbers
* Account listing is a fast DB query — no scraping required
* Account numbers are masked in API responses but available internally for scraper matching
* Discovery is idempotent (upsert) — safe to run multiple times

❌ **Negativas:**

* Accounts can become stale if bank adds/removes accounts between discoveries
* Discovery requires a dedicated scraper instance (separate browser), adding ~15-20s of overhead
* Coupling between credmgr and discovery service via `DiscoveryTrigger` interface
