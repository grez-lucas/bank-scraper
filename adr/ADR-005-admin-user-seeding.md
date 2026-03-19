# ADR-005: Admin User Seeding

### **Context** 💭
The Credential Manager supports a maximum of 3-4 C-level users (FR-1002). There is no self-registration — admin users must be created by an operator during system setup. Each user requires a username, bcrypt-hashed password, and an encrypted TOTP secret. We need a mechanism to create the first admin user.

### **Alternatives** ⚖️
1. **CLI seed command:** A `credmgr seed-admin` command that interactively prompts for username and password, generates a TOTP secret, encrypts it with envelope encryption, creates the user in the database, and prints the `otpauth://` URI (+ optional QR code) for Google Authenticator.
2. **SQL migration seed:** Insert an initial admin user via a SQL migration with a known temporary password. User must change password on first login.
3. **Config file seed:** Define initial users in a YAML/JSON config file. Parsed on first startup to create users if they don't exist.

### **Decision** 💪
We will use a **CLI seed command** (`credmgr seed-admin`). The operator runs it interactively during initial setup. It prompts for a password (no echo), generates a TOTP secret, encrypts it, stores the user in PostgreSQL, and outputs the `otpauth://` URI for scanning with an authenticator app.

### **Consequences**

✅ **Positivas:**
* Secure — password is entered interactively (not stored in files, migrations, or config)
* TOTP secret is generated and encrypted in one step — never touches disk in plaintext
* Reusable — can add more admin users later by running the command again
* Clear separation between schema migrations and data seeding

❌ **Negativas:**
* Requires interactive terminal access — cannot be fully automated in CI/CD without scripting stdin
* Operator must save the TOTP QR/URI at creation time — there's no "resend" without re-seeding
* Adding users in production requires shell access to the server
