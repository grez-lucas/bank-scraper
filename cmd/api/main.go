// Package main is the API gateway entrypoint.
//
// Usage:
//
//	api serve           Start the HTTP server
//	api create-key      Create an API key
//	api discover        Trigger account discovery for a bank
//	api migrate         Run all pending database migrations
//	api migrate-down    Rollback the last migration
//	api version         Show the current migration version
package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/grez-lucas/bank-scraper/internal/api"
	"github.com/grez-lucas/bank-scraper/internal/api/handler"
	"github.com/grez-lucas/bank-scraper/internal/api/resilience"
	"github.com/grez-lucas/bank-scraper/internal/api/service"
	"github.com/grez-lucas/bank-scraper/internal/api/session"
	"github.com/grez-lucas/bank-scraper/internal/config"
	"github.com/grez-lucas/bank-scraper/internal/credmgr/crypto"
	credservice "github.com/grez-lucas/bank-scraper/internal/credmgr/service"
	"github.com/grez-lucas/bank-scraper/internal/scraper/bank"
	"github.com/grez-lucas/bank-scraper/internal/scraper/bank/bbva"
	"github.com/grez-lucas/bank-scraper/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		log.Fatalf("failed to load .env: %v", err)
	}

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	switch os.Args[1] {
	case "migrate":
		if err := store.RunMigrations(cfg.DatabaseURL); err != nil {
			log.Fatalf("migrate failed: %v", err)
		}
		fmt.Println("migrations applied successfully")

	case "migrate-down":
		if err := store.RollbackMigration(cfg.DatabaseURL); err != nil {
			log.Fatalf("migrate-down failed: %v", err)
		}
		fmt.Println("rolled back one migration")

	case "version":
		v, dirty, err := store.MigrationVersion(cfg.DatabaseURL)
		if err != nil {
			log.Fatalf("version check failed: %v", err)
		}
		fmt.Printf("version: %d, dirty: %v\n", v, dirty)

	case "create-key":
		if err := createKey(cfg); err != nil {
			log.Fatalf("create-key failed: %v", err)
		}

	case "discover":
		if err := discover(cfg); err != nil {
			log.Fatalf("discover failed: %v", err)
		}

	case "serve":
		if err := serve(cfg); err != nil {
			log.Fatalf("serve failed: %v", err)
		}

	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func serve(cfg *config.Config) error {
	logger := slog.Default()

	mk, err := crypto.ParseMasterKey(cfg.EncryptionKey)
	if err != nil {
		return fmt.Errorf("parse encryption key: %w", err)
	}

	db, err := connectDB(cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	pool := db.Pool()

	// Repositories
	accountRepo := store.NewAccountRepo(pool)
	credRepo := store.NewCredentialRepo(pool)
	apiKeyRepo := store.NewAPIKeyRepo(pool)

	// Credential service (read-only — used as CredentialProvider)
	credSvc := newCredService(pool, mk, logger)

	// Scraper factory (BBVA only for now)
	factory := newScraperFactory(cfg)

	// Session manager
	sessionMgr := session.NewManager(credSvc, factory, logger)

	// Discovery service (uses its own scraper instances, not the session manager's)
	discoverySvc := service.NewDiscoveryService(accountRepo, factory, logger)

	// Resilience layer wrapping the session manager
	retryCfg := resilience.Config{
		MaxAttempts:  cfg.RetryMaxAttempts,
		InitialDelay: cfg.RetryInitialDelay,
		MaxDelay:     cfg.RetryMaxDelay,
	}
	breakers := resilience.NewBreakerRegistry(resilience.BreakerConfig{
		MaxFailures:  cfg.CircuitBreakerMaxFailures,
		ResetTimeout: cfg.CircuitBreakerResetTimeout,
	})
	resilientProvider := resilience.NewResilientProvider(sessionMgr, retryCfg, breakers)

	// Router
	router := api.SetupRouter(api.RouterDeps{
		AccountRepo: accountRepo,
		APIKeyRepo:  apiKeyRepo,
		CredRepo:    credRepo,
		Scrapers:    resilientProvider,
		Discovery:   discoverySvc,
		Creds:       credSvc,
		PingDB:      func() error { return db.Ping(context.Background()) },
		Sessions:    sessionMgr,
	})

	// Start server with graceful shutdown
	addr := fmt.Sprintf(":%d", cfg.APIPort)
	srv := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Use an error channel to surface immediate startup failures (e.g., port in use)
	startErr := make(chan error, 1)
	go func() {
		logger.Info("API gateway starting", slog.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			startErr <- err
		}
	}()

	// Wait for interrupt signal or startup failure
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-startErr:
		return fmt.Errorf("server failed to start: %w", err)
	case sig := <-quit:
		logger.Info("shutting down", slog.String("signal", sig.String()))
	}

	// Graceful shutdown: drain HTTP requests first, then close scrapers
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown error", slog.Any("error", err))
	}
	sessionMgr.Shutdown(shutdownCtx)

	logger.Info("API gateway stopped")
	return nil
}

func createKey(cfg *config.Config) error {
	clientID := parseFlag("--client-id")
	description := parseFlag("--description")
	if clientID == "" {
		return fmt.Errorf("--client-id is required\nUsage: api create-key --client-id=<id> [--description=<desc>]")
	}

	// Generate random 32-byte key, display as hex
	rawKey := make([]byte, 32)
	if _, err := rand.Read(rawKey); err != nil {
		return fmt.Errorf("generate key: %w", err)
	}
	rawKeyHex := hex.EncodeToString(rawKey)

	// Hash the hex string (same as middleware which hashes the raw X-API-Key header value)
	hash := sha256.Sum256([]byte(rawKeyHex))

	db, err := connectDB(cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	repo := store.NewAPIKeyRepo(db.Pool())
	var desc *string
	if description != "" {
		desc = &description
	}
	k := &store.APIKey{
		KeyHash:     hash[:],
		ClientID:    clientID,
		Description: desc,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := repo.Create(ctx, k); err != nil {
		return fmt.Errorf("create api key: %w", err)
	}

	fmt.Printf("\nAPI key created successfully!\n\n")
	fmt.Printf("  Client ID:   %s\n", clientID)
	fmt.Printf("  Key ID:      %s\n", k.ID)
	fmt.Printf("  API Key:     %s\n\n", rawKeyHex)
	fmt.Println("Save this key now — it will not be shown again.")
	fmt.Println("Use it in requests: X-API-Key: " + rawKeyHex)

	return nil
}

func discover(cfg *config.Config) error {
	bankCode := parseFlag("--bank")
	if bankCode == "" {
		return fmt.Errorf("--bank is required\nUsage: api discover --bank=BBVA")
	}

	mk, err := crypto.ParseMasterKey(cfg.EncryptionKey)
	if err != nil {
		return fmt.Errorf("parse encryption key: %w", err)
	}

	db, err := connectDB(cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	pool := db.Pool()
	credRepo := store.NewCredentialRepo(pool)
	accountRepo := store.NewAccountRepo(pool)
	logger := slog.Default()

	credSvc := newCredService(pool, mk, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	creds, err := credSvc.GetCredentials(ctx, bankCode)
	if err != nil {
		return fmt.Errorf("get credentials: %w", err)
	}

	cred, err := credRepo.GetActiveByBankCode(ctx, bankCode)
	if err != nil {
		return fmt.Errorf("get credential record: %w", err)
	}

	factory := newScraperFactory(cfg)
	discoverySvc := service.NewDiscoveryService(accountRepo, factory, logger)

	fmt.Printf("Discovering accounts for %s...\n", bankCode)
	accounts, err := discoverySvc.Discover(ctx, bankCode, creds, cred.ID)
	if err != nil {
		return fmt.Errorf("discover accounts: %w", err)
	}

	fmt.Printf("\nDiscovered %d account(s):\n", len(accounts))
	for i, a := range accounts {
		fmt.Printf("  [%d] %s %s (%s)\n", i+1, a.BankCode, handler.MaskAccountNumber(a.AccountNumber), a.Currency)
	}

	return nil
}

// --- Helpers ---

// connectDB connects to the database with a 10-second timeout for the initial connection.
func connectDB(databaseURL string) (*store.DB, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := store.Connect(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	return db, nil
}

// newCredService creates a read-only credential service (no tester, for decryption only).
func newCredService(pool *pgxpool.Pool, mk crypto.MasterKey, logger *slog.Logger) *credservice.CredentialService {
	credRepo := store.NewCredentialRepo(pool)
	auditRepo := store.NewAuditLogRepo(pool)
	aw := credservice.NewAuditWriter(auditRepo, logger)
	return credservice.NewCredentialService(credRepo, aw, mk, nil, logger)
}

func newScraperFactory(cfg *config.Config) bank.ScraperFactory {
	return func(bankCode bank.Code) (bank.Scraper, error) {
		switch bankCode {
		case bank.BankBBVA:
			return bbva.NewScraper(
				bbva.WithTimeout(cfg.ScraperTimeout),
				bbva.WithHeadless(cfg.ScraperHeadless),
			)
		default:
			return nil, fmt.Errorf("unsupported bank: %s", bankCode)
		}
	}
}

// parseFlag extracts a CLI flag value from os.Args.
// Supports both --flag=value and --flag value forms.
func parseFlag(name string) string {
	for i, arg := range os.Args {
		if v, ok := strings.CutPrefix(arg, name+"="); ok {
			return v
		}
		if arg == name && i+1 < len(os.Args) {
			return os.Args[i+1]
		}
	}
	return ""
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage: api <command>\n\nCommands:\n")
	fmt.Fprintf(os.Stderr, "  serve         Start the API gateway\n")
	fmt.Fprintf(os.Stderr, "  create-key    Create an API key\n")
	fmt.Fprintf(os.Stderr, "  discover      Trigger account discovery for a bank\n")
	fmt.Fprintf(os.Stderr, "  migrate       Run all pending database migrations\n")
	fmt.Fprintf(os.Stderr, "  migrate-down  Rollback the last migration\n")
	fmt.Fprintf(os.Stderr, "  version       Show the current migration version\n")
}
