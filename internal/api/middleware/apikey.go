// Package middleware provides Gin middleware for the API gateway.
package middleware

import (
	"context"
	"crypto/sha256"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/grez-lucas/bank-scraper/internal/store"
)

const (
	headerAPIKey     = "X-API-Key"
	contextKeyClient = "client_id"
)

// errorJSON sends a standard error response and aborts the request.
func errorJSON(c *gin.Context, status int, message string) {
	c.AbortWithStatusJSON(status, gin.H{
		"status":  "error",
		"message": message,
	})
}

// APIKeyAuth returns Gin middleware that validates the X-API-Key header.
func APIKeyAuth(repo store.APIKeyRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		rawKey := c.GetHeader(headerAPIKey)
		if rawKey == "" {
			errorJSON(c, http.StatusUnauthorized, "missing API key")
			return
		}

		hash := sha256.Sum256([]byte(rawKey))

		apiKey, err := repo.GetByKeyHash(c.Request.Context(), hash[:])
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				errorJSON(c, http.StatusUnauthorized, "invalid API key")
				return
			}
			errorJSON(c, http.StatusInternalServerError, "internal error")
			return
		}

		if apiKey.RevokedAt != nil {
			errorJSON(c, http.StatusUnauthorized, "API key revoked")
			return
		}

		go func() { _ = repo.UpdateLastUsed(context.Background(), apiKey.ID) }()

		c.Set(contextKeyClient, apiKey.ClientID)
		c.Next()
	}
}

// GetClientID retrieves the authenticated client ID from the Gin context.
// Returns empty string if the API key middleware was not applied.
func GetClientID(c *gin.Context) string {
	v, exists := c.Get(contextKeyClient)
	if !exists {
		return ""
	}
	s, _ := v.(string)
	return s
}
