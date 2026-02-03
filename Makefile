# COLORS
ccgreen=$(shell printf "\033[32m")
ccred=$(shell printf "\033[0;31m")
ccyellow=$(shell printf "\033[0;33m")
ccend=$(shell printf "\033[0m")

.PHONY: capture-bbva

test:
	@printf "$(ccyellow)Testing files... $(ccend)\n"
	go test -v ./... -short
	@printf "$(ccgreen)Testing files done!$(ccend)\n"

# Capture fixtures
capture-bbva:
	go run ./scripts/capture-fixtures/main.go -bank=bbva

# Sanitize fixtures:
sanitize-fixtures:
	go run ./scripts/sanitize-patterns/main.go -bank=bbva

# Preview sanitization:
sanitize-preview:
	go run ./scripts/sanitize-patterns/main.go -bank=bbva --dry-run


