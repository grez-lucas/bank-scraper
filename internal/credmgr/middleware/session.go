package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/grez-lucas/bank-scraper/internal/credmgr/service"
	"github.com/grez-lucas/bank-scraper/internal/store"
)

const (
	sessionCookieName = "credmgr_session"
	contextKeyUser    = "user"
	contextKeyToken   = "session_token"
)

// SessionMiddleware validates the session cookie and injects the user into gin.Context.
type SessionMiddleware struct {
	auth *service.AuthService
}

// NewSessionMiddleware creates a new SessionMiddleware.
func NewSessionMiddleware(auth *service.AuthService) *SessionMiddleware {
	return &SessionMiddleware{auth: auth}
}

// Required returns a Gin handler that enforces authentication.
// Redirects to /login if the session is missing or expired.
func (m *SessionMiddleware) Required() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie(sessionCookieName)
		if err != nil || token == "" {
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}

		user, err := m.auth.ValidateSession(c.Request.Context(), token)
		if err != nil {
			// Clear invalid cookie
			c.SetCookie(sessionCookieName, "", -1, "/", "", false, true)
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}

		c.Set(contextKeyUser, user)
		c.Set(contextKeyToken, token)
		c.Next()
	}
}

// GetUser retrieves the authenticated user from the gin.Context.
func GetUser(c *gin.Context) *store.User {
	v, exists := c.Get(contextKeyUser)
	if !exists {
		return nil
	}
	u, _ := v.(*store.User)
	return u
}

// GetSessionToken retrieves the session token from the gin.Context.
func GetSessionToken(c *gin.Context) string {
	v, _ := c.Get(contextKeyToken)
	s, _ := v.(string)
	return s
}

// SessionCookieName returns the cookie name for use by handlers.
func SessionCookieName() string {
	return sessionCookieName
}
