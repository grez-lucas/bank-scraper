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

// APIKeyAuth returns Gin middleware that validates the X-API-Key header.
// It hashes the raw key with SHA-256, looks it up in the database,
// checks revocation, and injects the client_id into the request context.
func APIKeyAuth(repo store.APIKeyRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		rawKey := c.GetHeader(headerAPIKey)
		if rawKey == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"status":  "error",
				"message": "missing API key",
			})
			return
		}

		hash := sha256.Sum256([]byte(rawKey))

		apiKey, err := repo.GetByKeyHash(c.Request.Context(), hash[:])
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"status":  "error",
					"message": "invalid API key",
				})
				return
			}
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"status":  "error",
				"message": "internal error",
			})
			return
		}

		if apiKey.RevokedAt != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"status":  "error",
				"message": "API key revoked",
			})
			return
		}

		// Best-effort async update of last_used_at (don't block the request)
		go repo.UpdateLastUsed(context.Background(), apiKey.ID)

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
