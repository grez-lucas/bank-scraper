// Package middleware provides Gin middleware for the credential manager.
package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	csrfCookieName = "csrf_token"
	csrfFormField  = "csrf_token"
	csrfTokenBytes = 32
)

// CSRF implements double-submit cookie CSRF protection.
// On GET/HEAD: generates a token, sets it as a cookie, and injects into gin.Context.
// On POST/PUT/DELETE: validates that the form field matches the cookie.
func CSRF() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead:
			token, err := generateCSRFToken()
			if err != nil {
				c.AbortWithStatus(http.StatusInternalServerError)
				return
			}
			c.SetSameSite(http.SameSiteStrictMode)
			c.SetCookie(csrfCookieName, token, 3600, "/", "", false, true)
			c.Set(csrfCookieName, token)
			c.Next()

		case http.MethodPost, http.MethodPut, http.MethodDelete:
			cookieToken, err := c.Cookie(csrfCookieName)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "missing CSRF token"})
				return
			}
			formToken := c.PostForm(csrfFormField)
			if formToken == "" || formToken != cookieToken {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "invalid CSRF token"})
				return
			}
			// Generate a fresh token for any re-rendered forms (e.g., validation errors).
			// Without this, error pages would render with an empty CSRF token.
			newToken, err := generateCSRFToken()
			if err != nil {
				c.AbortWithStatus(http.StatusInternalServerError)
				return
			}
			c.SetSameSite(http.SameSiteStrictMode)
			c.SetCookie(csrfCookieName, newToken, 3600, "/", "", false, true)
			c.Set(csrfCookieName, newToken)
			c.Next()

		default:
			c.Next()
		}
	}
}

// GetCSRFToken retrieves the CSRF token from the gin.Context.
func GetCSRFToken(c *gin.Context) string {
	v, _ := c.Get(csrfCookieName)
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func generateCSRFToken() (string, error) {
	b := make([]byte, csrfTokenBytes)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
