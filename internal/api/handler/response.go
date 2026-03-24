// Package handler contains HTTP handlers for the API gateway.
package handler

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/grez-lucas/bank-scraper/internal/store"
)

// Health status constants.
const (
	StatusHealthy     = "healthy"
	StatusDegraded    = "degraded"
	StatusUnavailable = "unavailable"
)

// ErrorJSON sends a standard error response and aborts the request.
// Format: {"status": "error", "message": "..."}
func ErrorJSON(c *gin.Context, status int, message string) {
	c.AbortWithStatusJSON(status, gin.H{
		"status":  "error",
		"message": message,
	})
}

// FormatAmount converts an int64 cents value to a decimal string.
// e.g., 123456 → "1234.56", -50 → "-0.50", 0 → "0.00"
func FormatAmount(cents int64) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	return fmt.Sprintf("%s%d.%02d", sign, cents/100, cents%100)
}

// MaskAccountNumber masks all but the last 4 characters.
// e.g., "PE001101190100064607" → "XXXXXXXXXXXXXXXX4607"
func MaskAccountNumber(num string) string {
	if len(num) <= 4 {
		return num
	}
	masked := make([]byte, len(num))
	for i := range masked {
		masked[i] = 'X'
	}
	copy(masked[len(masked)-4:], num[len(num)-4:])
	return string(masked)
}

// AccountResponse is the API representation of a bank account.
type AccountResponse struct {
	AccountID     string     `json:"account_id"`
	BankCode      string     `json:"bank_code"`
	Currency      string     `json:"currency"`
	AccountType   string     `json:"account_type"`
	Status        string     `json:"status"`
	AccountNumber string     `json:"account_number"` // masked
	LastSync      *time.Time `json:"last_sync"`
}

// BalanceResponse is the API representation of an account balance.
type BalanceResponse struct {
	AccountID        string `json:"account_id"`
	BankCode         string `json:"bank_code"`
	Currency         string `json:"currency"`
	AvailableBalance string `json:"available_balance"`
	CurrentBalance   string `json:"current_balance"`
	FetchedAt        string `json:"fetched_at"` // ISO 8601
}

// TransactionResponse is the API representation of a single transaction.
type TransactionResponse struct {
	ID           string            `json:"id"`
	Reference    string            `json:"reference,omitempty"`
	Date         string            `json:"date"` // ISO 8601
	Description  string            `json:"description"`
	Amount       string            `json:"amount"`
	Type         string            `json:"type"` // CREDIT or DEBIT
	BalanceAfter *string           `json:"balance_after,omitempty"`
	Extra        map[string]string `json:"extra,omitempty"`
}

// TransactionsListResponse wraps a list of transactions with metadata.
type TransactionsListResponse struct {
	AccountID    string                `json:"account_id"`
	BankCode     string                `json:"bank_code"`
	Currency     string                `json:"currency"`
	FromDate     string                `json:"from_date"`
	ToDate       string                `json:"to_date"`
	Transactions []TransactionResponse `json:"transactions"`
	Pagination   PaginationResponse    `json:"pagination"`
	FetchedAt    string                `json:"fetched_at"` // ISO 8601
}

// PaginationResponse contains pagination metadata.
type PaginationResponse struct {
	TotalCount int  `json:"total_count"`
	Page       int  `json:"page"`
	PageSize   int  `json:"page_size"`
	HasMore    bool `json:"has_more"`
}

// HealthResponse is the API representation of system health.
type HealthResponse struct {
	Status    string                    `json:"status"` // healthy, degraded, unavailable
	Timestamp string                    `json:"timestamp"`
	Banks     map[string]BankHealthInfo `json:"banks"`
}

// BankHealthInfo describes the health of a single bank integration.
type BankHealthInfo struct {
	Status                   string  `json:"status"`
	LastSuccessfulConnection *string `json:"last_successful_connection"`
	ErrorMessage             *string `json:"error_message"`
}

// ToAccountResponse converts a store.Account to an API response with masked account number.
func ToAccountResponse(a store.Account) AccountResponse {
	return AccountResponse{
		AccountID:     a.ID.String(),
		BankCode:      a.BankCode,
		Currency:      a.Currency,
		AccountType:   a.AccountType,
		Status:        a.Status,
		AccountNumber: MaskAccountNumber(a.AccountNumber),
		LastSync:      a.LastSyncedAt,
	}
}
