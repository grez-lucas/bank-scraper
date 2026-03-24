package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/grez-lucas/bank-scraper/internal/api/session"
	"github.com/grez-lucas/bank-scraper/internal/scraper/bank"
)

// DBPinger checks database connectivity.
type DBPinger func() error

// SessionStatusProvider returns per-bank session info without triggering scraping.
type SessionStatusProvider interface {
	SessionStatus() []session.SessionInfo
}

// KnownBanksProvider returns the list of bank codes that the system is configured to support.
// Used by the health endpoint to report "unavailable" for banks with no active session.
type KnownBanksProvider func() []bank.Code

// HealthHandler handles the health check endpoint.
type HealthHandler struct {
	pingDB     DBPinger
	sessions   SessionStatusProvider
	knownBanks KnownBanksProvider
}

// NewHealthHandler creates a new HealthHandler.
// knownBanks may be nil if you don't want to report unconfigured banks.
func NewHealthHandler(pingDB DBPinger, sessions SessionStatusProvider, knownBanks ...KnownBanksProvider) *HealthHandler {
	h := &HealthHandler{pingDB: pingDB, sessions: sessions}
	if len(knownBanks) > 0 {
		h.knownBanks = knownBanks[0]
	}
	return h
}

// Check returns system health status.
// GET /api/v1/health
func (h *HealthHandler) Check(c *gin.Context) {
	overall := StatusHealthy

	if err := h.pingDB(); err != nil {
		overall = StatusDegraded
	}

	banks := make(map[string]BankHealthInfo)

	// Populate known banks as unavailable first, then override with actual session status
	if h.knownBanks != nil {
		for _, code := range h.knownBanks() {
			banks[string(code)] = BankHealthInfo{Status: StatusUnavailable}
		}
	}

	for _, info := range h.sessions.SessionStatus() {
		bi := BankHealthInfo{}
		if info.Active {
			bi.Status = StatusHealthy
			exp := info.ExpiresAt.Format(time.RFC3339)
			bi.LastSuccessfulConnection = &exp
		} else {
			bi.Status = StatusDegraded
		}
		banks[string(info.BankCode)] = bi
	}

	c.JSON(http.StatusOK, HealthResponse{
		Status:    overall,
		Timestamp: time.Now().Format(time.RFC3339),
		Banks:     banks,
	})
}
