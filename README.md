# Bank Scraper Platform

Internal platform for [AyniFX](https://aynifx.com) that programmatically accesses bank account data (balances, transactions) from Peruvian banks (BBVA, Interbank, BCP) via browser automation.

## Modules

| Module | Status | Description |
|--------|--------|-------------|
| **Scraper Engine** | BBVA complete | Browser automation + data extraction (`internal/scraper/`) |
| **Credential Manager** | In progress | Secure credential storage with 2FA web UI (`internal/credmgr/`) |
| **API Gateway** | Planned | REST API for AyniFX consumption (`internal/api/`) |

## Prerequisites

- **Go 1.22+**
- **Docker** (for PostgreSQL)
- **Chrome/Chromium** (for scraper browser automation — installed automatically by Rod)

## Quick Start

```bash
# 1. Clone and enter the repo
git clone <repo-url> && cd bank-scraper

# 2. Bootstrap the dev environment
make setup
```

This will:
- Copy `.env.example` to `.env` (if not already present)
- Start PostgreSQL via Docker Compose
- Run database migrations

Then fill in your secrets in `.env`:

```bash
# Generate an encryption key
make gen-encryption-key
# Copy the output into your .env file
```

## Environment Variables

See [`.env.example`](.env.example) for the full list with documentation.

| Variable | Required | Description |
|----------|----------|-------------|
| `DATABASE_URL` | Yes | PostgreSQL connection string |
| `ENCRYPTION_KEY` | For credmgr | 64-char hex (32 bytes) for envelope encryption |
| `BBVA_COMPANY_CODE` | For scraper | BBVA company code |
| `BBVA_USER_CODE` | For scraper | BBVA user code |
| `BBVA_PASSWORD` | For scraper | BBVA password |

## Common Commands

```bash
# Build all binaries
make build

# Run tests (unit + mock mode)
make test

# Run integration tests (replay mode)
make test-integration

# Start/stop PostgreSQL
make db-up
make db-down

# Run/rollback migrations
make migrate
make migrate-down

# Format + lint
make fmt
make lint
```

## Project Structure

```
bank-scraper/
├── cmd/
│   ├── main.go              # Main entrypoint (placeholder)
│   └── credmgr/main.go      # Credential Manager CLI
├── internal/
│   ├── config/               # Shared configuration
│   ├── store/                # Database layer (shared)
│   │   └── migrations/       # SQL migration files
│   ├── scraper/              # Scraper engine
│   │   ├── bank/bbva/        # BBVA implementation
│   │   ├── browser/          # DOM utilities
│   │   └── debug/            # Diagnostics
│   └── credmgr/              # Credential Manager (in progress)
├── adr/                      # Architecture Decision Records
├── docs/                     # Documentation
├── scripts/                  # Fixture capture + sanitization tools
├── docker-compose.yml        # PostgreSQL for dev
└── .env.example              # Environment variable template
```

## Architecture Decision Records

See [`adr/`](adr/) for all architectural decisions with context, alternatives, and trade-offs.
