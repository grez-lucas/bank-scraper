// Package main is the credential manager entrypoint.
//
// Usage:
//
//	credmgr serve           Start the HTTP server
//	credmgr seed-admin      Create an admin user (interactive)
//	credmgr migrate         Run all pending database migrations
//	credmgr migrate-down    Rollback the last migration
//	credmgr version         Show the current migration version
package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strings"
	"time"

	apiservice "github.com/grez-lucas/bank-scraper/internal/api/service"
	"github.com/grez-lucas/bank-scraper/internal/config"
	"github.com/grez-lucas/bank-scraper/internal/credmgr/crypto"
	"github.com/grez-lucas/bank-scraper/internal/credmgr/handler"
	"github.com/grez-lucas/bank-scraper/internal/credmgr/service"
	scraperfactory "github.com/grez-lucas/bank-scraper/internal/scraper/factory"
	"github.com/grez-lucas/bank-scraper/internal/store"
	"github.com/joho/godotenv"
	"github.com/pquerna/otp/totp"
	"golang.org/x/term"
)

func main() {
	// Load .env if present (ignored if file doesn't exist)
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

	case "seed-admin":
		if err := seedAdmin(cfg); err != nil {
			log.Fatalf("seed-admin failed: %v", err)
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
	if cfg.EncryptionKey == "" {
		return fmt.Errorf("ENCRYPTION_KEY env var is required")
	}
	mk, err := crypto.ParseMasterKey(cfg.EncryptionKey)
	if err != nil {
		return fmt.Errorf("parse encryption key: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := store.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer db.Close()

	logger := slog.Default()

	pool := db.Pool()

	// Create repositories
	userRepo := store.NewUserRepo(pool)
	sessionRepo := store.NewSessionRepo(pool)
	auditRepo := store.NewAuditLogRepo(pool)
	credRepo := store.NewCredentialRepo(pool)
	accountRepo := store.NewAccountRepo(pool)

	// Create services
	aw := service.NewAuditWriter(auditRepo, logger)
	authSvc := service.NewAuthService(userRepo, sessionRepo, aw, mk, cfg.SessionTTL, logger)
	tester := service.NewScraperCredentialTester()
	credSvc := service.NewCredentialService(credRepo, aw, mk, tester, logger)

	// Account discovery service
	discoverySvc := apiservice.NewDiscoveryService(accountRepo, scraperfactory.New(cfg.ScraperTimeout, cfg.ScraperHeadless), logger)

	// Setup and start router
	router := handler.SetupRouter(handler.RouterDeps{
		Auth:        authSvc,
		Creds:       credSvc,
		AuditRepo:   auditRepo,
		AuditWriter: aw,
		Logger:      logger,
		AccountRepo: accountRepo,
		Discoverer:  discoverySvc,
	})

	addr := fmt.Sprintf(":%d", cfg.CredMgrPort)
	fmt.Printf("Credential Manager starting on http://localhost%s\n", addr)
	return router.Run(addr)
}

func seedAdmin(cfg *config.Config) error {
	// Parse username from args
	username := ""
	for i, arg := range os.Args {
		if arg == "--username" && i+1 < len(os.Args) {
			username = os.Args[i+1]
		} else if v, ok := strings.CutPrefix(arg, "--username="); ok {
			username = v
		}
	}
	if username == "" {
		return fmt.Errorf("--username is required\nUsage: credmgr seed-admin --username=<name>")
	}

	// Require encryption key
	if cfg.EncryptionKey == "" {
		return fmt.Errorf("ENCRYPTION_KEY env var is required for seed-admin")
	}
	mk, err := crypto.ParseMasterKey(cfg.EncryptionKey)
	if err != nil {
		return fmt.Errorf("parse encryption key: %w", err)
	}

	// Prompt for password
	fmt.Print("Enter password: ")
	passwordBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println() // newline after password input
	if err != nil {
		return fmt.Errorf("read password: %w", err)
	}
	password := string(passwordBytes)
	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}

	// Confirm password
	fmt.Print("Confirm password: ")
	confirmBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return fmt.Errorf("read password confirmation: %w", err)
	}
	if string(confirmBytes) != password {
		return fmt.Errorf("passwords do not match")
	}

	// Hash password
	passwordHash, err := service.HashPassword(password)
	if err != nil {
		return err
	}

	// Generate TOTP key
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "BankScraper",
		AccountName: username,
	})
	if err != nil {
		return fmt.Errorf("generate TOTP key: %w", err)
	}

	// Encrypt TOTP secret
	encSecret, encDEK, err := crypto.Seal(mk, []byte(key.Secret()))
	if err != nil {
		return fmt.Errorf("encrypt TOTP secret: %w", err)
	}

	// Connect to DB and create user
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := store.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer db.Close()

	userRepo := store.NewUserRepo(db.Pool())
	u := &store.User{
		Username:      username,
		PasswordHash:  passwordHash,
		TOTPSecretEnc: encSecret,
		TOTPSecretDEK: encDEK,
	}
	if err := userRepo.Create(ctx, u); err != nil {
		return fmt.Errorf("create user: %w", err)
	}

	fmt.Printf("\nAdmin user created: %s (ID: %s)\n\n", username, u.ID)
	fmt.Println("Scan this URI with your authenticator app (Google Authenticator, Authy, etc.):")
	fmt.Printf("\n  %s\n\n", key.URL())
	fmt.Printf("Or enter this secret manually: %s\n", key.Secret())

	return nil
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage: credmgr <command>\n\nCommands:\n")
	fmt.Fprintf(os.Stderr, "  serve         Start the HTTP server\n")
	fmt.Fprintf(os.Stderr, "  seed-admin    Create an admin user (interactive)\n")
	fmt.Fprintf(os.Stderr, "  migrate       Run all pending database migrations\n")
	fmt.Fprintf(os.Stderr, "  migrate-down  Rollback the last migration\n")
	fmt.Fprintf(os.Stderr, "  version       Show the current migration version\n")
}
