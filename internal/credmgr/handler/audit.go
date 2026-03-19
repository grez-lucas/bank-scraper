package handler

import (
	"encoding/csv"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/grez-lucas/bank-scraper/internal/credmgr/middleware"
	"github.com/grez-lucas/bank-scraper/internal/credmgr/service"
	"github.com/grez-lucas/bank-scraper/internal/store"
)

const (
	auditPageSize  = 50
	AuditLogsViewed = "audit_logs_viewed"
)

// Known audit actions for the filter dropdown.
var auditActions = []string{
	service.AuditLoginSuccess, service.AuditLoginFailed, service.AuditLogout,
	service.AuditCredentialCreated, service.AuditCredentialUpdated,
	service.AuditCredentialDeleted, service.AuditCredentialsListed,
	AuditLogsViewed,
}

// AuditHandler handles audit log viewing and export.
type AuditHandler struct {
	audit store.AuditLogRepository
	aw    *service.AuditWriter
	log   *slog.Logger
}

// NewAuditHandler creates a new AuditHandler.
func NewAuditHandler(audit store.AuditLogRepository, aw *service.AuditWriter, log *slog.Logger) *AuditHandler {
	return &AuditHandler{audit: audit, aw: aw, log: log}
}

// List shows the audit log page with filtering and pagination.
func (h *AuditHandler) List(c *gin.Context) {
	filter, page := h.parseFilter(c)

	logs, total, err := h.audit.List(c.Request.Context(), filter)
	if err != nil {
		h.log.Error("list audit logs failed", slog.Any("error", err))
		renderPage(c, http.StatusOK, "audit_logs.html", gin.H{
			"Title": "Audit Log",
			"Error": "Failed to load audit logs",
		})
		return
	}

	// Audit the access itself (FR-1307) — only on first page to avoid self-flooding
	if page == 1 {
		user := middleware.GetUser(c)
		if user != nil {
			h.aw.Log(c.Request.Context(), &user.ID, AuditLogsViewed, "audit_log", "", c.ClientIP(), c.Request.UserAgent(), true, nil)
		}
	}

	totalPages := int((total + int64(auditPageSize) - 1) / int64(auditPageSize))
	if totalPages < 1 {
		totalPages = 1
	}

	renderPage(c, http.StatusOK, "audit_logs.html", gin.H{
		"Title":        "Audit Log",
		"Logs":         logs,
		"Page":         page,
		"TotalPages":   totalPages,
		"Actions":      auditActions,
		"FilterAction": c.Query("action"),
		"FilterFrom":   c.Query("from"),
		"FilterTo":     c.Query("to"),
		"FilterQuery":  h.filterQueryString(c),
	})
}

// Export streams audit logs as CSV or JSON.
func (h *AuditHandler) Export(c *gin.Context) {
	filter, _ := h.parseFilter(c)
	filter.Limit = 10000 // Export up to 10k rows
	filter.Offset = 0

	logs, _, err := h.audit.List(c.Request.Context(), filter)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to export: %v", err)
		return
	}

	format := c.DefaultQuery("format", "csv")
	switch format {
	case "json":
		h.exportJSON(c, logs)
	default:
		h.exportCSV(c, logs)
	}
}

func (h *AuditHandler) exportCSV(c *gin.Context, logs []store.AuditLog) {
	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", "attachment; filename=audit_logs.csv")

	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"Timestamp", "Action", "Target Type", "Target ID", "IP Address", "User Agent", "Success"})

	for _, l := range logs {
		_ = w.Write([]string{
			l.Timestamp.Format(time.RFC3339),
			l.Action,
			l.TargetType,
			l.TargetID,
			l.IPAddress,
			l.UserAgent,
			strconv.FormatBool(l.Success),
		})
	}
	w.Flush()
}

func (h *AuditHandler) exportJSON(c *gin.Context, logs []store.AuditLog) {
	c.Header("Content-Type", "application/json")
	c.Header("Content-Disposition", "attachment; filename=audit_logs.json")

	type exportEntry struct {
		Timestamp  string         `json:"timestamp"`
		Action     string         `json:"action"`
		TargetType string         `json:"target_type"`
		TargetID   string         `json:"target_id"`
		IPAddress  string         `json:"ip_address"`
		UserAgent  string         `json:"user_agent"`
		Success    bool           `json:"success"`
		Details    map[string]any `json:"details,omitempty"`
	}

	entries := make([]exportEntry, len(logs))
	for i, l := range logs {
		entries[i] = exportEntry{
			Timestamp:  l.Timestamp.Format(time.RFC3339),
			Action:     l.Action,
			TargetType: l.TargetType,
			TargetID:   l.TargetID,
			IPAddress:  l.IPAddress,
			UserAgent:  l.UserAgent,
			Success:    l.Success,
			Details:    l.Details,
		}
	}

	enc := json.NewEncoder(c.Writer)
	enc.SetIndent("", "  ")
	_ = enc.Encode(entries)
}

func (h *AuditHandler) parseFilter(c *gin.Context) (store.AuditFilter, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}

	filter := store.AuditFilter{
		Action: c.Query("action"),
		Limit:  auditPageSize,
		Offset: (page - 1) * auditPageSize,
	}

	if from := c.Query("from"); from != "" {
		if t, err := time.Parse("2006-01-02", from); err == nil {
			filter.FromDate = &t
		}
	}
	if to := c.Query("to"); to != "" {
		if t, err := time.Parse("2006-01-02", to); err == nil {
			end := t.Add(24*time.Hour - time.Second) // end of day
			filter.ToDate = &end
		}
	}

	return filter, page
}

func (h *AuditHandler) filterQueryString(c *gin.Context) string {
	params := url.Values{}
	if v := c.Query("action"); v != "" {
		params.Set("action", v)
	}
	if v := c.Query("from"); v != "" {
		params.Set("from", v)
	}
	if v := c.Query("to"); v != "" {
		params.Set("to", v)
	}
	if encoded := params.Encode(); encoded != "" {
		return "&" + encoded
	}
	return ""
}
