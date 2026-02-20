# COLORS
ccgreen=$(shell printf "\033[32m")
ccred=$(shell printf "\033[0;31m")
ccyellow=$(shell printf "\033[0;33m")
ccend=$(shell printf "\033[0m")

.PHONY: capture-bbva

# ============================== #
# QUALITY CONTROL
# ============================== #

# tidy: tidy modifies and formats .go files
.PHONY: tidy
tidy: fmt

# test: run all tests
.PHONY: test
test:
	@printf "$(ccyellow)Testing files... $(ccend)\n"
	go test -v ./... -short
	@printf "$(ccgreen)Testing files done!$(ccend)\n"

# test-ingegration: run with recorded sessions
test-integration:
	@printf "$(ccyellow)Running integration tests... $(ccend)\n"
	SCRAPER_TEST_MODE=replay go test ./internal/scraper/bank/... -v
	@printf "$(ccgreen)Testing files done!$(ccend)\n"

# test-live: run against live banks (dangerous!!)
test-live:
	@printf "$(ccyellow)WARNING: This will hit live bank websites!$(ccend)\n"
	@read -p "Are you sure? [y/N] " confirm && [ "$$confirm" = "y"]
	SCRAPER_TEST_MODE=live go test ./internal/scraper/bank/... -v -count=1

# test/cover: run all tests and display coverage
.PHONY: test/cover
test/cover:
	@printf "$(ccyellow)Testing files... $(ccend)\n"
	go test -v -buildvcs -coverprofile=/tmp/coverage.out ./...
	go tool cover -html=/tmp/coverage.out
	@printf "$(ccgreen)Testing files done!$(ccend)\n"
	@printf "$(ccgreen)Displaying coverage...$(ccend)\n"

# lint: run golangci-lint if installed, else print a fallback message
.PHONY: lint
lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run --config .golangci-lint.yml ./...; \
	else \
		@printf "$(ccred)golangci-lint is not installed. Please install it from https://github.com/golangci-lint$(ccend)\n"; \
	fi

# ============================== #
# FIXTURES
# ============================== #

# Capture fixtures
capture-bbva:
	go run ./scripts/capture-fixtures/main.go -bank=bbva

# Sanitize fixtures:
sanitize-fixtures:
	go run ./scripts/sanitize-patterns/main.go -bank=bbva

# Preview sanitization:
sanitize-preview:
	go run ./scripts/sanitize-patterns/main.go -bank=bbva --dry-run


# ============================== #
# HAR Recordings
# ============================== #

# Sanitize recordings:
sanitize-recordings: sanitize-recordings-login sanitize-recordings-post-login

# Sanitize login recordings:
sanitize-recordings-login:
	go run ./scripts/sanitize-har/main.go -bank=bbva -scenario=login-success
	go run ./scripts/sanitize-har/main.go -bank=bbva -scenario=login-bot-detection
	go run ./scripts/sanitize-har/main.go -bank=bbva -scenario=login-invalid-credentials-legacy

# Sanitize post-login recordings:
sanitize-recordings-post-login:
	go run ./scripts/sanitize-har/main.go -bank=bbva -scenario=dashboard-load
	go run ./scripts/sanitize-har/main.go -bank=bbva -scenario=accounts-page
	go run ./scripts/sanitize-har/main.go -bank=bbva -scenario=transactions-all
	go run ./scripts/sanitize-har/main.go -bank=bbva -scenario=transactions-load-more
	go run ./scripts/sanitize-har/main.go -bank=bbva -scenario=logout

