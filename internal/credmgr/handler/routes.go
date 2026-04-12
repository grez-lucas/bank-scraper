package handler

import (
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	apihandler "github.com/aynifx/bank-scraper/internal/api/handler"
	"github.com/aynifx/bank-scraper/internal/credmgr/middleware"
	"github.com/aynifx/bank-scraper/internal/credmgr/service"
	"github.com/aynifx/bank-scraper/internal/credmgr/static"
	"github.com/aynifx/bank-scraper/internal/credmgr/templates"
	"github.com/aynifx/bank-scraper/internal/store"
)

const (
	flashError   = "error"
	flashSuccess = "success"
)

// Template functions available in all templates.
var templateFuncs = template.FuncMap{
	"add":         func(a, b int) int { return a + b },
	"sub":         func(a, b int) int { return a - b },
	"maskAccount": apihandler.MaskAccountNumber,
}

// RouterDeps holds the dependencies needed to set up the credmgr router.
type RouterDeps struct {
	Auth        *service.AuthService
	Creds       *service.CredentialService
	AuditRepo   store.AuditLogRepository
	AuditWriter *service.AuditWriter
	Logger      *slog.Logger
	AccountRepo store.AccountRepository // nil = discovery disabled
	Discoverer  Discoverer              // nil = discovery disabled
}

// SetupRouter creates and configures the Gin router with all routes and middleware.
func SetupRouter(deps RouterDeps) *gin.Engine {
	r := gin.Default()

	tmpl := template.Must(
		template.New("").Funcs(templateFuncs).ParseFS(templates.FS, "*.html"),
	)
	r.SetHTMLTemplate(tmpl)

	staticFS, _ := fs.Sub(static.FS, ".")
	r.StaticFS("/static", http.FS(staticFS))

	authHandler := NewAuthHandler(deps.Auth, deps.Logger)
	credHandler := NewCredentialHandler(deps.Creds, deps.Logger, deps.AccountRepo, deps.Discoverer)
	auditHandler := NewAuditHandler(deps.AuditRepo, deps.AuditWriter, deps.Logger)

	r.Use(middleware.CSRF())

	r.GET("/login", authHandler.ShowLogin)
	r.POST("/login", authHandler.Login)
	r.GET("/login/totp", authHandler.ShowTOTP)
	r.POST("/login/totp", authHandler.VerifyTOTP)

	sessionMW := middleware.NewSessionMiddleware(deps.Auth)
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
		protected.GET("/credentials/:id/accounts", credHandler.Accounts)
		protected.POST("/credentials/:id/discover", credHandler.Discover)

		protected.GET("/audit", auditHandler.List)
		protected.GET("/audit/export", auditHandler.Export)
	}

	r.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/credentials")
	})

	return r
}

// renderPage renders a template, injecting CSRF token, user, and flash messages.
func renderPage(c *gin.Context, status int, name string, data gin.H) {
	if data["CSRFToken"] == nil {
		data["CSRFToken"] = middleware.GetCSRFToken(c)
	}
	if data["User"] == nil {
		data["User"] = middleware.GetUser(c)
	}

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
