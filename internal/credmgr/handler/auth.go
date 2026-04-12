// Package handler provides HTTP handlers for the credential manager web UI.
package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/aynifx/bank-scraper/internal/credmgr/middleware"
	"github.com/aynifx/bank-scraper/internal/credmgr/service"
)

const (
	pendingCookieName = "credmgr_pending"
	flashCookieName   = "credmgr_flash"
)

// AuthHandler handles login, TOTP verification, and logout.
type AuthHandler struct {
	auth *service.AuthService
	log  *slog.Logger
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(auth *service.AuthService, log *slog.Logger) *AuthHandler {
	return &AuthHandler{auth: auth, log: log}
}

// ShowLogin renders the login form.
func (h *AuthHandler) ShowLogin(c *gin.Context) {
	renderPage(c, http.StatusOK, "login.html", gin.H{
		"Title": "Login",
	})
}

// Login handles the login form submission.
func (h *AuthHandler) Login(c *gin.Context) {
	username := c.PostForm("username")
	password := c.PostForm("password")

	_, pendingToken, err := h.auth.Login(c.Request.Context(), username, password, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		msg := "Invalid credentials"
		if errors.Is(err, service.ErrAccountLocked) {
			msg = "Account is locked. Try again later."
		}
		renderPage(c, http.StatusOK, "login.html", gin.H{
			"Title":    "Login",
			"Error":    msg,
			"Username": username,
		})
		return
	}

	// Store pending token in cookie for TOTP step
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(pendingCookieName, pendingToken, 300, "/", "", false, true)
	c.Redirect(http.StatusFound, "/login/totp")
}

// ShowTOTP renders the TOTP verification form.
func (h *AuthHandler) ShowTOTP(c *gin.Context) {
	pendingToken, err := c.Cookie(pendingCookieName)
	if err != nil || pendingToken == "" {
		c.Redirect(http.StatusFound, "/login")
		return
	}

	renderPage(c, http.StatusOK, "totp.html", gin.H{
		"Title":        "Two-Factor Authentication",
		"PendingToken": pendingToken,
	})
}

// VerifyTOTP handles the TOTP code submission.
func (h *AuthHandler) VerifyTOTP(c *gin.Context) {
	pendingToken := c.PostForm("pending_token")
	code := c.PostForm("code")

	sessionToken, err := h.auth.VerifyTOTP(c.Request.Context(), pendingToken, code, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		msg := "Invalid code. Please try again."
		if errors.Is(err, service.ErrInvalidCredentials) {
			msg = "Session expired. Please login again."
		}
		renderPage(c, http.StatusOK, "totp.html", gin.H{
			"Title":        "Two-Factor Authentication",
			"Error":        msg,
			"PendingToken": pendingToken,
		})
		return
	}

	// Clear pending cookie, set session cookie
	c.SetCookie(pendingCookieName, "", -1, "/", "", false, true)
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(middleware.SessionCookieName(), sessionToken, 0, "/", "", false, true)
	c.Redirect(http.StatusFound, "/credentials")
}

// Logout destroys the session and redirects to login.
func (h *AuthHandler) Logout(c *gin.Context) {
	token := middleware.GetSessionToken(c)
	if token != "" {
		if err := h.auth.Logout(c.Request.Context(), token); err != nil {
			h.log.Warn("logout error", slog.Any("error", err))
		}
	}
	c.SetCookie(middleware.SessionCookieName(), "", -1, "/", "", false, true)
	c.Redirect(http.StatusFound, "/login")
}
