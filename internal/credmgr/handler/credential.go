package handler

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/grez-lucas/bank-scraper/internal/credmgr/middleware"
	"github.com/grez-lucas/bank-scraper/internal/credmgr/service"
)

// CredentialHandler handles credential CRUD operations.
type CredentialHandler struct {
	creds *service.CredentialService
	log   *slog.Logger
}

// NewCredentialHandler creates a new CredentialHandler.
func NewCredentialHandler(creds *service.CredentialService, log *slog.Logger) *CredentialHandler {
	return &CredentialHandler{creds: creds, log: log}
}

// List shows all credentials.
func (h *CredentialHandler) List(c *gin.Context) {
	user := middleware.GetUser(c)
	creds, err := h.creds.List(c.Request.Context(), user.ID, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		h.log.Error("list credentials failed", slog.Any("error", err))
		setFlash(c, "error", "Failed to load credentials")
	}
	renderPage(c, http.StatusOK, "credentials_list.html", gin.H{
		"Title":       "Credentials",
		"Credentials": creds,
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

	setFlash(c, "success", "Credential created successfully")
	c.Redirect(http.StatusFound, "/credentials")
}

// Edit shows the edit credential form.
func (h *CredentialHandler) Edit(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.Redirect(http.StatusFound, "/credentials")
		return
	}

	// Get credential summary for pre-filling bank code and label
	user := middleware.GetUser(c)
	creds, _ := h.creds.List(c.Request.Context(), user.ID, c.ClientIP(), c.Request.UserAgent())
	var found *service.CredentialSummary
	for i := range creds {
		if creds[i].ID == id {
			found = &creds[i]
			break
		}
	}
	if found == nil {
		setFlash(c, "error", "Credential not found")
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

	setFlash(c, "success", "Credential updated successfully")
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
		setFlash(c, "error", "Failed to delete credential")
	} else {
		setFlash(c, "success", "Credential deleted")
	}
	c.Redirect(http.StatusFound, "/credentials")
}

// Test validates credentials against the bank.
func (h *CredentialHandler) Test(c *gin.Context) {
	cred := parsePlaintextCredential(c)
	// If form fields are empty (e.g., from list page test button), redirect back
	if cred.BankCode == "" {
		setFlash(c, "error", "Cannot test: credential fields not provided")
		c.Redirect(http.StatusFound, "/credentials")
		return
	}

	if err := h.creds.Test(c.Request.Context(), cred); err != nil {
		h.log.Error("credential test failed", slog.Any("error", err))
		setFlash(c, "error", "Credential test failed. The bank login did not succeed.")
	} else {
		setFlash(c, "success", "Credential test passed!")
	}
	c.Redirect(http.StatusFound, "/credentials")
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
