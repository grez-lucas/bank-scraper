package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/grez-lucas/bank-scraper/internal/scraper/bank"
	"github.com/grez-lucas/bank-scraper/internal/store"
)

const (
	defaultDateRangeDays   = 7
	maxDateRangeDays       = 90
	defaultPageSize        = 50
	scraperFetchCount      = 250 // Max transactions to fetch from the bank scraper
	dateLayout             = "2006-01-02"
)

// TransactionHandler handles transaction listing endpoints.
type TransactionHandler struct {
	accounts store.AccountRepository
	scrapers ScraperProvider
}

// NewTransactionHandler creates a new TransactionHandler.
func NewTransactionHandler(accounts store.AccountRepository, scrapers ScraperProvider) *TransactionHandler {
	return &TransactionHandler{accounts: accounts, scrapers: scrapers}
}

// List returns transactions for a specific account.
// GET /api/v1/accounts/:account_id/transactions?from_date=2026-03-01&to_date=2026-03-15&page=1&page_size=50
func (h *TransactionHandler) List(c *gin.Context) {
	accountID, err := uuid.Parse(c.Param("account_id"))
	if err != nil {
		ErrorJSON(c, http.StatusBadRequest, "invalid account_id")
		return
	}

	// Parse date range
	now := time.Now()
	toDate := now
	fromDate := now.AddDate(0, 0, -defaultDateRangeDays)

	if fd := c.Query("from_date"); fd != "" {
		parsed, err := time.Parse(dateLayout, fd)
		if err != nil {
			ErrorJSON(c, http.StatusBadRequest, "invalid from_date format, use YYYY-MM-DD")
			return
		}
		fromDate = parsed
	}
	if td := c.Query("to_date"); td != "" {
		parsed, err := time.Parse(dateLayout, td)
		if err != nil {
			ErrorJSON(c, http.StatusBadRequest, "invalid to_date format, use YYYY-MM-DD")
			return
		}
		toDate = parsed
	}

	if fromDate.After(toDate) {
		ErrorJSON(c, http.StatusBadRequest, "from_date must be before to_date")
		return
	}
	if toDate.Sub(fromDate).Hours()/24 > maxDateRangeDays {
		ErrorJSON(c, http.StatusBadRequest, "date range exceeds maximum of 90 days")
		return
	}

	// Parse pagination
	page := 1
	pageSize := defaultPageSize
	if p := c.Query("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}
	if ps := c.Query("page_size"); ps != "" {
		if v, err := strconv.Atoi(ps); err == nil && v > 0 && v <= 250 {
			pageSize = v
		}
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

	txns, err := scraper.GetTransactions(c.Request.Context(), acct.AccountNumber, scraperFetchCount)
	if err != nil {
		h.scrapers.Invalidate(bank.Code(acct.BankCode))
		ErrorJSON(c, http.StatusServiceUnavailable, "failed to fetch transactions")
		return
	}

	// Filter by date range and build response
	var items []TransactionResponse
	for _, tx := range txns {
		if tx.Date.Before(fromDate) || tx.Date.After(toDate.AddDate(0, 0, 1)) {
			continue
		}
		item := TransactionResponse{
			ID:          tx.ID,
			Reference:   tx.Reference,
			Date:        tx.Date.Format(time.RFC3339),
			Description: tx.Description,
			Amount:      FormatAmount(tx.Amount),
			Type:        string(tx.Type),
			Extra:       tx.Extra,
		}
		if tx.BalanceAfter != nil {
			s := FormatAmount(*tx.BalanceAfter)
			item.BalanceAfter = &s
		}
		items = append(items, item)
	}

	// Apply pagination
	total := len(items)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	pageItems := items[start:end]

	c.JSON(http.StatusOK, TransactionsListResponse{
		AccountID:    acct.ID.String(),
		BankCode:     acct.BankCode,
		Currency:     acct.Currency,
		FromDate:     fromDate.Format(dateLayout),
		ToDate:       toDate.Format(dateLayout),
		Transactions: pageItems,
		Pagination: PaginationResponse{
			TotalCount: total,
			Page:       page,
			PageSize:   pageSize,
			HasMore:    end < total,
		},
		FetchedAt: time.Now().Format(time.RFC3339),
	})
}
