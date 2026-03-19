package handler

import (
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/grez-lucas/bank-scraper/internal/credmgr/middleware"
	"github.com/grez-lucas/bank-scraper/internal/credmgr/service"
	"github.com/grez-lucas/bank-scraper/internal/credmgr/static"
	"github.com/grez-lucas/bank-scraper/internal/credmgr/templates"
	"github.com/grez-lucas/bank-scraper/internal/store"
)

// Template functions available in all templates.
var templateFuncs = template.FuncMap{
	"add": func(a, b int) int { return a + b },
	"sub": func(a, b int) int { return a - b },
}

// SetupRouter creates and configures the Gin router with all routes and middleware.
func SetupRouter(
	auth *service.AuthService,
	creds *service.CredentialService,
	auditRepo store.AuditLogRepository,
	aw *service.AuditWriter,
	logger *slog.Logger,
) *gin.Engine {
	r := gin.Default()

	// Parse templates from embedded filesystem
	tmpl := template.Must(
		template.New("").Funcs(templateFuncs).ParseFS(templates.FS, "*.html"),
	)
	r.SetHTMLTemplate(tmpl)

	// Serve static assets (logo, etc.) from embedded filesystem
	staticFS, _ := fs.Sub(static.FS, ".")
	r.StaticFS("/static", http.FS(staticFS))

	// Create handlers
	authHandler := NewAuthHandler(auth, logger)
	credHandler := NewCredentialHandler(creds, logger)
	auditHandler := NewAuditHandler(auditRepo, aw, logger)

	// CSRF middleware on all routes
	r.Use(middleware.CSRF())

	// Public routes
	r.GET("/login", authHandler.ShowLogin)
	r.POST("/login", authHandler.Login)
	r.GET("/login/totp", authHandler.ShowTOTP)
	r.POST("/login/totp", authHandler.VerifyTOTP)

	// Protected routes (require session)
	sessionMW := middleware.NewSessionMiddleware(auth)
	protected := r.Group("/")
	protected.Use(sessionMW.Required())
	{
		protected.POST("/logout", authHandler.Logout)

		protected.GET("/credentials", credHandler.List)
		protected.GET("/credentials/new", credHandler.New)
		protected.POST("/credentials", credHandler.Create)
		protected.GET("/credentials/:id/edit", credHandler.Edit)
		protected.POST("/credentials/:id/update", credHandler.Update)
		protected.POST("/credentials/:id/delete", credHandler.Delete)
		protected.POST("/credentials/:id/test", credHandler.Test)

		protected.GET("/audit", auditHandler.List)
		protected.GET("/audit/export", auditHandler.Export)
	}

	// Root redirect
	r.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/credentials")
	})

	return r
}

// renderPage renders a template within the layout, injecting common data.
func renderPage(c *gin.Context, status int, name string, data gin.H) {
	// Inject CSRF token
	if data["CSRFToken"] == nil {
		data["CSRFToken"] = middleware.GetCSRFToken(c)
	}

	// Inject user (for nav display)
	if data["User"] == nil {
		data["User"] = middleware.GetUser(c)
	}

	// Read and clear flash message
	if flash, err := c.Cookie(flashCookieName); err == nil && flash != "" {
		data["Flash"] = flash
		if flashType, err := c.Cookie(flashCookieName + "_type"); err == nil {
			data["FlashType"] = flashType
		}
		c.SetCookie(flashCookieName, "", -1, "/", "", false, true)
		c.SetCookie(flashCookieName+"_type", "", -1, "/", "", false, true)
	}

	c.HTML(status, name, data)
}

// setFlash sets a flash message cookie to be displayed on the next page load.
func setFlash(c *gin.Context, flashType, message string) {
	c.SetCookie(flashCookieName, message, 30, "/", "", false, true)
	c.SetCookie(flashCookieName+"_type", flashType, 30, "/", "", false, true)
}
