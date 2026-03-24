// Package api wires together the API gateway components.
package api

import (
	"github.com/gin-gonic/gin"
	"github.com/grez-lucas/bank-scraper/internal/api/handler"
	"github.com/grez-lucas/bank-scraper/internal/api/middleware"
	"github.com/grez-lucas/bank-scraper/internal/store"
)

// RouterDeps holds the dependencies needed to set up the API router.
type RouterDeps struct {
	AccountRepo store.AccountRepository
	APIKeyRepo  store.APIKeyRepository
	CredRepo    store.CredentialRepository
	Scrapers    handler.ScraperProvider
	Discovery   handler.Discoverer
	Creds       handler.CredentialProvider
	PingDB      handler.DBPinger
	Sessions    handler.SessionStatusProvider
}

// SetupRouter creates and configures the Gin router with all API routes.
func SetupRouter(deps RouterDeps) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())

	accountH := handler.NewAccountHandler(deps.AccountRepo)
	balanceH := handler.NewBalanceHandler(deps.AccountRepo, deps.Scrapers)
	txH := handler.NewTransactionHandler(deps.AccountRepo, deps.Scrapers)
	healthH := handler.NewHealthHandler(deps.PingDB, deps.Sessions)
	discoveryH := handler.NewDiscoveryHandler(deps.Discovery, deps.Creds, deps.CredRepo)

	v1 := r.Group("/api/v1")
	v1.Use(middleware.APIKeyAuth(deps.APIKeyRepo))
	{
		v1.GET("/accounts", accountH.List)
		v1.GET("/accounts/:account_id/balance", balanceH.Get)
		v1.GET("/accounts/:account_id/transactions", txH.List)
		v1.GET("/health", healthH.Check)
		v1.POST("/admin/discover/:bank_code", discoveryH.Trigger)
	}

	return r
}
