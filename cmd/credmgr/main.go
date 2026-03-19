// Package main is the credential manager entrypoint.
//
// Usage:
//
//	credmgr serve         Start the HTTP server
//	credmgr migrate       Run all pending database migrations
//	credmgr migrate-down  Rollback the last migration
//	credmgr version       Show the current migration version
package main

import (
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/grez-lucas/bank-scraper/internal/config"
	"github.com/grez-lucas/bank-scraper/internal/store"
	"github.com/joho/godotenv"
)

func main() {
	// Load .env if present (ignored if file doesn't exist)
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		// godotenv returns a *PathError when the file doesn't exist
		if !os.IsNotExist(err) {
			log.Fatalf("failed to load .env: %v", err)
		}
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

	case "serve":
		fmt.Printf("credential manager server would start on port %d\n", cfg.CredMgrPort)
		fmt.Println("(not yet implemented — see M6)")

	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage: credmgr <command>\n\nCommands:\n")
	fmt.Fprintf(os.Stderr, "  serve         Start the HTTP server\n")
	fmt.Fprintf(os.Stderr, "  migrate       Run all pending database migrations\n")
	fmt.Fprintf(os.Stderr, "  migrate-down  Rollback the last migration\n")
	fmt.Fprintf(os.Stderr, "  version       Show the current migration version\n")
}
