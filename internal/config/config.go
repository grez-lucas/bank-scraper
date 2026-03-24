// Package config provides shared configuration for all bank-scraper modules.
package config

import (
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"
)

// Config holds all configuration for the bank-scraper platform.
// Loaded from environment variables (optionally via .env file).
type Config struct {
	// Database
	DatabaseURL string `envconfig:"DATABASE_URL" required:"true"`

	// Encryption
	EncryptionKey string `envconfig:"ENCRYPTION_KEY"` // 64-char hex string (32 bytes)

	// Credential Manager
	CredMgrPort int           `envconfig:"CREDMGR_PORT" default:"8081"`
	SessionTTL  time.Duration `envconfig:"SESSION_TTL" default:"15m"`

	// API Gateway
	APIPort int `envconfig:"API_PORT" default:"8080"`

	// Cookie security (set to false for local dev without HTTPS)
	SecureCookies bool `envconfig:"CREDMGR_SECURE_COOKIES" default:"false"`

	// Scraper settings
	ScraperTimeout  time.Duration `envconfig:"SCRAPER_TIMEOUT" default:"30s"`
	ScraperHeadless bool          `envconfig:"SCRAPER_HEADLESS" default:"true"`

	// Resilience
	RetryMaxAttempts           uint64        `envconfig:"RETRY_MAX_ATTEMPTS" default:"3"`
	RetryInitialDelay          time.Duration `envconfig:"RETRY_INITIAL_DELAY" default:"1s"`
	RetryMaxDelay              time.Duration `envconfig:"RETRY_MAX_DELAY" default:"30s"`
	CircuitBreakerMaxFailures  uint32        `envconfig:"CB_MAX_FAILURES" default:"5"`
	CircuitBreakerResetTimeout time.Duration `envconfig:"CB_RESET_TIMEOUT" default:"5m"`

	// BBVA — backwards compatible with existing env vars.
	// Will move to DB-managed credentials in a future milestone.
	BBVA BBVAConfig `envconfig:"BBVA"`
}

// BBVAConfig holds BBVA-specific scraper credentials.
// Env vars: BBVA_COMPANY_CODE, BBVA_USER_CODE, BBVA_PASSWORD.
type BBVAConfig struct {
	CompanyCode string `envconfig:"COMPANY_CODE"`
	UserCode    string `envconfig:"USER_CODE"`
	Password    string `envconfig:"PASSWORD"`
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	return &cfg, nil
}
