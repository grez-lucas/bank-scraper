# COLORS
ccgreen=$(shell printf "\033[32m")
ccred=$(shell printf "\033[0;31m")
ccyellow=$(shell printf "\033[0;33m")
ccend=$(shell printf "\033[0m")

# ============================== #
# QUALITY CONTROL
# ============================== #

## fmt: format all .go files
.PHONY: fmt
fmt:
	@printf "$(ccyellow)Formatting files... $(ccend)\n"
	go fmt ./...
	@printf "$(ccgreen)Formatting done!$(ccend)\n"

## tidy: tidy modifies and formats .go files
.PHONY: tidy
tidy: fmt

## test: run all tests
.PHONY: test
test:
	@printf "$(ccyellow)Testing files... $(ccend)\n"
	go test -v ./... -short
	@printf "$(ccgreen)Testing files done!$(ccend)\n"

## test-integration: run with recorded sessions
.PHONY: test-integration
test-integration:
	@printf "$(ccyellow)Running integration tests... $(ccend)\n"
	SCRAPER_TEST_MODE=replay go test ./internal/scraper/bank/... -v
	@printf "$(ccgreen)Testing files done!$(ccend)\n"

## test-live: run against live banks (dangerous!!)
.PHONY: test-live
test-live:
	@printf "$(ccyellow)WARNING: This will hit live bank websites!$(ccend)\n" && \
	read -p "Are you sure? [y/N] " confirm && [ "$$confirm" = "y" ] && \
	set -a && . ./.env && set +a && \
	SCRAPER_TEST_MODE=live go test ./internal/scraper/bank/bbva/... -v -run TestBBVAScraper_Live -count=1

## test/cover: run all tests and display coverage
.PHONY: test/cover
test/cover:
	@printf "$(ccyellow)Testing files... $(ccend)\n"
	go test -v -buildvcs -coverprofile=/tmp/coverage.out ./...
	go tool cover -html=/tmp/coverage.out
	@printf "$(ccgreen)Testing files done!$(ccend)\n"
	@printf "$(ccgreen)Displaying coverage...$(ccend)\n"

## lint: run golangci-lint if installed, else print a fallback message
.PHONY: lint
lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run --config .golangci-lint.yml ./...; \
	else \
		printf "$(ccred)golangci-lint is not installed. Please install it from https://github.com/golangci-lint$(ccend)\n"; \
	fi

# ============================== #
# FIXTURES
# ============================== #

## capture-bbva: capture HTML fixtures from BBVA portal
.PHONY: capture-bbva
capture-bbva:
	go run ./scripts/capture-fixtures/main.go -bank=bbva

## sanitize-fixtures: redact PII from HTML fixtures
.PHONY: sanitize-fixtures
sanitize-fixtures:
	go run ./scripts/sanitize-patterns/main.go -bank=bbva

## sanitize-preview: preview fixture sanitization (dry run)
.PHONY: sanitize-preview
sanitize-preview:
	go run ./scripts/sanitize-patterns/main.go -bank=bbva --dry-run


# ============================== #
# HAR Recordings
# ============================== #

## sanitize-recordings: sanitize all HAR recordings
.PHONY: sanitize-recordings
sanitize-recordings: sanitize-recordings-login sanitize-recordings-post-login

## sanitize-recordings-login: sanitize login HAR recordings
.PHONY: sanitize-recordings-login
sanitize-recordings-login:
	go run ./scripts/sanitize-har/main.go -bank=bbva -scenario=login-success
	go run ./scripts/sanitize-har/main.go -bank=bbva -scenario=login-bot-detection
	go run ./scripts/sanitize-har/main.go -bank=bbva -scenario=login-invalid-credentials-legacy

## sanitize-recordings-post-login: sanitize post-login HAR recordings
.PHONY: sanitize-recordings-post-login
sanitize-recordings-post-login:
	go run ./scripts/sanitize-har/main.go -bank=bbva -scenario=dashboard-load
	go run ./scripts/sanitize-har/main.go -bank=bbva -scenario=accounts-page
	go run ./scripts/sanitize-har/main.go -bank=bbva -scenario=transactions-all
	go run ./scripts/sanitize-har/main.go -bank=bbva -scenario=transactions-load-more
	go run ./scripts/sanitize-har/main.go -bank=bbva -scenario=logout

# ============================== #
# HELP
# ============================== #

## help: show this help message
.PHONY: help
help:
	@printf "Usage: make [target]\n\n"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## //' | awk -F': ' '{printf "  $(ccgreen)%-30s$(ccend) %s\n", $$1, $$2}'
