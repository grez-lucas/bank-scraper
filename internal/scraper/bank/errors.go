package bank

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrSessionExpired     = errors.New("session expired")

	ErrParsingFailed = errors.New("failed to parse bank response")
	ErrTimeout       = errors.New("operation timed out")
)

// ScraperError provides detailed error context
type ScraperError struct {
	BankCode  BankCode
	Operation string
	Cause     error
	Details   string
}

func (e *ScraperError) Error() string {
	return fmt.Sprintf("[%s] %s failed: %v - %s", e.BankCode, e.Operation, e.Cause, e.Details)
}

func (e *ScraperError) Unwrap() error {
	return e.Cause
}
