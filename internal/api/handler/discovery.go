package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/grez-lucas/bank-scraper/internal/store"
)

// Discoverer discovers bank accounts and persists them.
type Discoverer interface {
	Discover(ctx context.Context, bankCode string, creds map[string]string, credentialID uuid.UUID) ([]store.Account, error)
}

// CredentialProvider retrieves decrypted bank credentials.
type CredentialProvider interface {
	GetCredentials(ctx context.Context, bankCode string) (map[string]string, error)
}

// DiscoveryHandler handles the admin account discovery endpoint.
type DiscoveryHandler struct {
	discovery Discoverer
	creds     CredentialProvider
	credRepo  store.CredentialRepository
}

// NewDiscoveryHandler creates a new DiscoveryHandler.
func NewDiscoveryHandler(discovery Discoverer, creds CredentialProvider, credRepo store.CredentialRepository) *DiscoveryHandler {
	return &DiscoveryHandler{
		discovery: discovery,
		creds:     creds,
		credRepo:  credRepo,
	}
}

// Trigger starts account discovery for a bank.
// POST /api/v1/admin/discover/:bank_code
func (h *DiscoveryHandler) Trigger(c *gin.Context) {
	bankCode := c.Param("bank_code")

	// Look up the credential to get its ID
	cred, err := h.credRepo.GetActiveByBankCode(c.Request.Context(), bankCode)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			ErrorJSON(c, http.StatusNotFound, "no credential configured for bank "+bankCode)
			return
		}
		ErrorJSON(c, http.StatusInternalServerError, "failed to lookup credential")
		return
	}

	// Get decrypted credentials
	fields, err := h.creds.GetCredentials(c.Request.Context(), bankCode)
	if err != nil {
		ErrorJSON(c, http.StatusInternalServerError, "failed to decrypt credentials")
		return
	}

	// Run discovery
	accounts, err := h.discovery.Discover(c.Request.Context(), bankCode, fields, cred.ID)
	if err != nil {
		ErrorJSON(c, http.StatusServiceUnavailable, "account discovery failed")
		return
	}

	resp := make([]AccountResponse, len(accounts))
	for i, a := range accounts {
		resp[i] = ToAccountResponse(a)
	}

	c.JSON(http.StatusOK, gin.H{"accounts": resp})
}
