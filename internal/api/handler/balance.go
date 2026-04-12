package handler

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/aynifx/bank-scraper/internal/scraper/bank"
	"github.com/aynifx/bank-scraper/internal/store"
)

// ScraperProvider returns a logged-in scraper for a bank.
type ScraperProvider interface {
	GetScraper(ctx context.Context, bankCode bank.Code) (bank.Scraper, error)
	Invalidate(bankCode bank.Code)
}

// BalanceHandler handles balance retrieval endpoints.
type BalanceHandler struct {
	accounts store.AccountRepository
	scrapers ScraperProvider
}

// NewBalanceHandler creates a new BalanceHandler.
func NewBalanceHandler(accounts store.AccountRepository, scrapers ScraperProvider) *BalanceHandler {
	return &BalanceHandler{accounts: accounts, scrapers: scrapers}
}

// Get returns the balance for a specific account.
// GET /api/v1/accounts/:account_id/balance
func (h *BalanceHandler) Get(c *gin.Context) {
	accountID, err := uuid.Parse(c.Param("account_id"))
	if err != nil {
		ErrorJSON(c, http.StatusBadRequest, "invalid account_id")
		return
	}

	acct, err := h.accounts.GetByID(c.Request.Context(), accountID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			ErrorJSON(c, http.StatusNotFound, "account not found")
			return
		}
		ErrorJSON(c, http.StatusInternalServerError, "failed to lookup account")
		return
	}

	scraper, err := h.scrapers.GetScraper(c.Request.Context(), bank.Code(acct.BankCode))
	if err != nil {
		ErrorJSON(c, http.StatusServiceUnavailable, "bank connection unavailable")
		return
	}

	balances, err := scraper.GetBalance(c.Request.Context())
	if err != nil {
		h.scrapers.Invalidate(bank.Code(acct.BankCode))
		ErrorJSON(c, http.StatusServiceUnavailable, "failed to fetch balance")
		return
	}

	for _, b := range balances {
		if b.AccountID == acct.AccountNumber {
			c.JSON(http.StatusOK, BalanceResponse{
				AccountID:        acct.ID.String(),
				BankCode:         acct.BankCode,
				Currency:         string(b.Currency),
				AvailableBalance: FormatAmount(b.AvailableBalance),
				CurrentBalance:   FormatAmount(b.CurrentBalance),
				FetchedAt:        time.Now().Format(time.RFC3339),
			})
			return
		}
	}

	ErrorJSON(c, http.StatusNotFound, "balance not found for account")
}
