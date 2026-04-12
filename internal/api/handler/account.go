package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/aynifx/bank-scraper/internal/store"
)

// AccountHandler handles account listing endpoints.
type AccountHandler struct {
	accounts store.AccountRepository
}

// NewAccountHandler creates a new AccountHandler.
func NewAccountHandler(accounts store.AccountRepository) *AccountHandler {
	return &AccountHandler{accounts: accounts}
}

// List returns all configured accounts, optionally filtered by bank_code and/or currency.
// GET /api/v1/accounts?bank_code=BBVA&currency=PEN
func (h *AccountHandler) List(c *gin.Context) {
	var filter store.AccountFilter
	if bc := c.Query("bank_code"); bc != "" {
		filter.BankCode = &bc
	}
	if cur := c.Query("currency"); cur != "" {
		filter.Currency = &cur
	}

	accounts, err := h.accounts.List(c.Request.Context(), filter)
	if err != nil {
		ErrorJSON(c, http.StatusInternalServerError, "failed to list accounts")
		return
	}

	resp := make([]AccountResponse, len(accounts))
	for i, a := range accounts {
		resp[i] = ToAccountResponse(a)
	}

	c.JSON(http.StatusOK, gin.H{"accounts": resp})
}
