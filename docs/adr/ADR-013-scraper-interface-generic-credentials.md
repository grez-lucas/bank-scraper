# ADR-013: Scraper Interface — Generic Credentials

### **Context** 💭

The `bank.Scraper` interface needs a `Login` method, but each bank has different credential fields: BBVA requires `company_code`, `user_code`, and `password`; Interbank and BCP will have their own field sets (TBD). The Credential Manager stores credentials as encrypted JSON maps (`map[string]string`), and `CredentialService.GetCredentials()` returns them in this format. The interface must be generic enough for all banks while remaining type-safe and easy to implement.

### **Alternatives** ⚖️

1. **Bank-specific credential structs in interface:** `Login(ctx, BankCredentials)` where `BankCredentials` is an interface with bank-specific implementations (e.g., `BBVACredentials`, `InterbankCredentials`). Type-safe but breaks the unified interface — the caller must know which struct to create, defeating the abstraction goal (PRD pillar: "consumers don't need to know which bank").
2. **Interface per bank:** Separate `BBVAScraper`, `InterbankScraper` interfaces. Maximum type safety but no polymorphism — the session manager and handlers cannot treat all banks uniformly.
3. **Generic field map:** `Login(ctx, map[string]string)` — the same format that `CredentialService.GetCredentials()` returns. Each bank scraper maps the generic fields internally to its specific struct. Zero conversion needed between credential storage and scraper consumption.

### **Decision** 💪

We will use `Login(ctx context.Context, creds map[string]string) (*Session, error)` with a **generic field map**. The full `bank.Scraper` interface:

```go
type Scraper interface {
    Login(ctx context.Context, creds map[string]string) (*Session, error)
    GetBalance(ctx context.Context) ([]Balance, error)
    GetTransactions(ctx context.Context, accountID string, count int) ([]Transaction, error)
    Logout(ctx context.Context) error
    Close() error
}
```

Each bank implementation maps the fields internally. For BBVA:
```go
creds := Credentials{
    CompanyCode: fields["company_code"],
    UserCode:    fields["user_code"],
    Password:    fields["password"],
}
```

Missing or invalid fields return a descriptive error from the bank's `Login` implementation.

### **Consequences**

✅ **Positivas:**

* Direct compatibility with `CredentialService.GetCredentials()` — no conversion layer needed
* Session manager and handlers work with any bank uniformly
* Adding a new bank only requires implementing the interface — no changes to callers
* Field validation happens inside each bank's `Login`, close to where the fields are used

❌ **Negativas:**

* No compile-time checking of credential field names — a typo in `"company_code"` vs `"companyCode"` is a runtime error
* Bank implementations must document their expected field names (mitigated by convention: snake_case matching the credential form fields)
* Less discoverable than a typed struct — callers must know which fields each bank needs (acceptable because callers never construct credentials manually; they come from the DB)
