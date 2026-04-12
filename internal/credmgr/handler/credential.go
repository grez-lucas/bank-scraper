package handler

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/aynifx/bank-scraper/internal/credmgr/middleware"
	"github.com/aynifx/bank-scraper/internal/credmgr/service"
	"github.com/aynifx/bank-scraper/internal/store"
)

// Discoverer triggers bank account discovery.
type Discoverer interface {
	Discover(ctx context.Context, bankCode string, creds map[string]string, credentialID uuid.UUID) ([]store.Account, error)
}

// CredentialHandler handles credential CRUD and account discovery.
type CredentialHandler struct {
	creds      *service.CredentialService
	accounts   store.AccountRepository // nil if not wired
	discoverer Discoverer              // nil if not wired
	log        *slog.Logger
}

// NewCredentialHandler creates a new CredentialHandler.
// discoverer and accounts may be nil (discovery features will be disabled).
func NewCredentialHandler(
	creds *service.CredentialService,
	logger *slog.Logger,
	accounts store.AccountRepository,
	discoverer Discoverer,
) *CredentialHandler {
	return &CredentialHandler{
		creds:      creds,
		accounts:   accounts,
		discoverer: discoverer,
		log:        logger,
	}
}

// findCredential looks up a credential summary by ID.
// Returns nil and logs an error if the lookup fails.
func (h *CredentialHandler) findCredential(c *gin.Context, id uuid.UUID) *service.CredentialSummary {
	user := middleware.GetUser(c)
	creds, err := h.creds.List(c.Request.Context(), user.ID, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		h.log.Error("list credentials failed", slog.Any("error", err))
		return nil
	}
	for i := range creds {
		if creds[i].ID == id {
			return &creds[i]
		}
	}
	return nil
}

// List shows all credentials.
func (h *CredentialHandler) List(c *gin.Context) {
	user := middleware.GetUser(c)
	creds, err := h.creds.List(c.Request.Context(), user.ID, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		h.log.Error("list credentials failed", slog.Any("error", err))
		setFlash(c, flashError, "Failed to load credentials")
	}
	renderPage(c, http.StatusOK, "credentials_list.html", gin.H{
		"Title":            "Credentials",
		"Credentials":      creds,
		"DiscoveryEnabled": h.discoverer != nil,
	})
}

// New shows the create credential form.
func (h *CredentialHandler) New(c *gin.Context) {
	renderPage(c, http.StatusOK, "credential_form.html", gin.H{
		"Title":      "New Credential",
		"EditMode":   false,
		"FormAction": "/credentials",
	})
}

// Create handles credential creation.
func (h *CredentialHandler) Create(c *gin.Context) {
	user := middleware.GetUser(c)
	cred := parsePlaintextCredential(c)

	_, err := h.creds.Create(c.Request.Context(), cred, user.ID, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		h.log.Error("create credential failed", slog.Any("error", err))
		renderPage(c, http.StatusOK, "credential_form.html", gin.H{
			"Title":      "New Credential",
			"EditMode":   false,
			"FormAction": "/credentials",
			"Error":      "Failed to create credential. Please check your input and try again.",
			"BankCode":   cred.BankCode,
			"Label":      cred.Label,
		})
		return
	}

	setFlash(c, flashSuccess, "Credential created successfully")
	c.Redirect(http.StatusFound, "/credentials")
}

// Edit shows the edit credential form.
func (h *CredentialHandler) Edit(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.Redirect(http.StatusFound, "/credentials")
		return
	}

	found := h.findCredential(c, id)
	if found == nil {
		setFlash(c, flashError, "Credential not found")
		c.Redirect(http.StatusFound, "/credentials")
		return
	}

	renderPage(c, http.StatusOK, "credential_form.html", gin.H{
		"Title":      "Edit Credential",
		"EditMode":   true,
		"FormAction": "/credentials/" + id.String() + "/update",
		"BankCode":   found.BankCode,
		"Label":      found.Label,
	})
}

// Update handles credential update.
func (h *CredentialHandler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.Redirect(http.StatusFound, "/credentials")
		return
	}

	user := middleware.GetUser(c)
	cred := parsePlaintextCredential(c)

	if err := h.creds.Update(c.Request.Context(), id, cred, user.ID, c.ClientIP(), c.Request.UserAgent()); err != nil {
		h.log.Error("update credential failed", slog.String("id", id.String()), slog.Any("error", err))
		renderPage(c, http.StatusOK, "credential_form.html", gin.H{
			"Title":      "Edit Credential",
			"EditMode":   true,
			"FormAction": "/credentials/" + id.String() + "/update",
			"Error":      "Failed to update credential. Please check your input and try again.",
			"BankCode":   cred.BankCode,
			"Label":      cred.Label,
		})
		return
	}

	setFlash(c, flashSuccess, "Credential updated successfully")
	c.Redirect(http.StatusFound, "/credentials")
}

// Delete handles soft-delete.
func (h *CredentialHandler) Delete(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.Redirect(http.StatusFound, "/credentials")
		return
	}

	user := middleware.GetUser(c)
	if err := h.creds.SoftDelete(c.Request.Context(), id, user.ID, c.ClientIP(), c.Request.UserAgent()); err != nil {
		setFlash(c, flashError, "Failed to delete credential")
	} else {
		setFlash(c, flashSuccess, "Credential deleted")
	}
	c.Redirect(http.StatusFound, "/credentials")
}

// Test validates stored credentials against the bank.
func (h *CredentialHandler) Test(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.Redirect(http.StatusFound, "/credentials")
		return
	}

	user := middleware.GetUser(c)
	if err := h.creds.TestByID(c.Request.Context(), id, user.ID, c.ClientIP(), c.Request.UserAgent()); err != nil {
		h.log.Error("credential test failed", slog.String("id", id.String()), slog.Any("error", err))
		setFlash(c, flashError, "Credential test failed. The bank login did not succeed.")
	} else {
		setFlash(c, flashSuccess, "Credential test passed!")
	}
	c.Redirect(http.StatusFound, "/credentials")
}

// Accounts shows the discovered accounts for a credential.
func (h *CredentialHandler) Accounts(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.Redirect(http.StatusFound, "/credentials")
		return
	}

	found := h.findCredential(c, id)
	if found == nil {
		setFlash(c, flashError, "Credential not found")
		c.Redirect(http.StatusFound, "/credentials")
		return
	}

	var accounts []store.Account
	if h.accounts != nil {
		bankCode := found.BankCode
		accounts, err = h.accounts.List(c.Request.Context(), store.AccountFilter{BankCode: &bankCode})
		if err != nil {
			h.log.Error("list accounts failed", slog.Any("error", err))
			setFlash(c, flashError, "Failed to load accounts")
		}
	}

	renderPage(c, http.StatusOK, "credential_accounts.html", gin.H{
		"Title":            fmt.Sprintf("Accounts — %s", found.BankCode),
		"Credential":       found,
		"Accounts":         accounts,
		"DiscoveryEnabled": h.discoverer != nil,
	})
}

// Discover triggers account discovery for a credential's bank.
func (h *CredentialHandler) Discover(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.Redirect(http.StatusFound, "/credentials")
		return
	}

	redirectURL := "/credentials/" + id.String() + "/accounts"

	if h.discoverer == nil {
		setFlash(c, flashError, "Account discovery is not configured")
		c.Redirect(http.StatusFound, redirectURL)
		return
	}

	found := h.findCredential(c, id)
	if found == nil {
		setFlash(c, flashError, "Credential not found")
		c.Redirect(http.StatusFound, "/credentials")
		return
	}

	fields, err := h.creds.GetCredentials(c.Request.Context(), found.BankCode)
	if err != nil {
		h.log.Error("decrypt credentials failed", slog.String("id", id.String()), slog.Any("error", err))
		setFlash(c, flashError, "Failed to decrypt credentials")
		c.Redirect(http.StatusFound, redirectURL)
		return
	}

	accounts, err := h.discoverer.Discover(c.Request.Context(), found.BankCode, fields, id)
	if err != nil {
		h.log.Error("account discovery failed",
			slog.String("bank", found.BankCode),
			slog.String("credential_id", id.String()),
			slog.Any("error", err))
		setFlash(c, flashError, "Account discovery failed. Please check the credential and try again.")
		c.Redirect(http.StatusFound, redirectURL)
		return
	}

	setFlash(c, flashSuccess, fmt.Sprintf("Discovered %d account(s) for %s", len(accounts), found.BankCode))
	c.Redirect(http.StatusFound, redirectURL)
}

func parsePlaintextCredential(c *gin.Context) service.PlaintextCredential {
	fields := make(map[string]string)
	for _, key := range []string{"company_code", "user_code", "password"} {
		if v := c.PostForm(key); v != "" {
			fields[key] = v
		}
	}
	return service.PlaintextCredential{
		BankCode: c.PostForm("bank_code"),
		Label:    c.PostForm("label"),
		Fields:   fields,
	}
}
