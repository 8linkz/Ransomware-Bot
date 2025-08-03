package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

// LoadConfig loads and validates all configuration files from the specified directory
func LoadConfig(configDir string) (*Config, error) {
	// Start with default configuration
	cfg := DefaultConfig()

	// Load general configuration
	if err := loadGeneralConfig(cfg, configDir); err != nil {
		return nil, fmt.Errorf("failed to load general config: %w", err)
	}

	// Load feeds configuration
	if err := loadFeedsConfig(cfg, configDir); err != nil {
		return nil, fmt.Errorf("failed to load feeds config: %w", err)
	}

	// Load format configuration
	if err := loadFormatConfig(cfg, configDir); err != nil {
		return nil, fmt.Errorf("failed to load format config: %w", err)
	}

	// Validate configuration
	if err := validateConfig(cfg); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	log.Info("Configuration loaded successfully")
	return cfg, nil
}

// loadGeneralConfig loads the main configuration file
func loadGeneralConfig(cfg *Config, configDir string) error {
	configPath := filepath.Join(configDir, "config_general.json")

	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file %s: %w", configPath, err)
	}

	var generalCfg GeneralConfig
	if err := json.Unmarshal(data, &generalCfg); err != nil {
		return fmt.Errorf("failed to parse config file %s: %w", configPath, err)
	}

	// Apply general configuration to main config
	cfg.LogLevel = generalCfg.LogLevel
	cfg.MaxRSSWorkers = generalCfg.MaxRSSWorkers
	cfg.APIKey = generalCfg.APIKey
	cfg.APIStartTime = generalCfg.APIStartTime
	cfg.RSSRetryCount = generalCfg.RSSRetryCount
	cfg.Webhooks = generalCfg.Webhooks

	// Apply log rotation settings (use defaults if not specified)
	if generalCfg.LogRotation.MaxSizeMB > 0 {
		cfg.LogRotation.MaxSizeMB = generalCfg.LogRotation.MaxSizeMB
	}
	if generalCfg.LogRotation.MaxBackups > 0 {
		cfg.LogRotation.MaxBackups = generalCfg.LogRotation.MaxBackups
	}
	if generalCfg.LogRotation.MaxAgeDays > 0 {
		cfg.LogRotation.MaxAgeDays = generalCfg.LogRotation.MaxAgeDays
	}
	// Compress can be explicitly set to false, so check if it was provided
	if generalCfg.LogRotation != (LogRotation{}) {
		cfg.LogRotation.Compress = generalCfg.LogRotation.Compress
	}

	// Parse duration strings
	if generalCfg.APIPollInterval != "" {
		duration, err := time.ParseDuration(generalCfg.APIPollInterval)
		if err != nil {
			return fmt.Errorf("invalid api_poll_interval format: %w", err)
		}
		cfg.APIPollInterval = duration
	}

	if generalCfg.RSSPollInterval != "" {
		duration, err := time.ParseDuration(generalCfg.RSSPollInterval)
		if err != nil {
			return fmt.Errorf("invalid rss_poll_interval format: %w", err)
		}
		cfg.RSSPollInterval = duration
	}

	if generalCfg.RSSRetryDelay != "" {
		duration, err := time.ParseDuration(generalCfg.RSSRetryDelay)
		if err != nil {
			return fmt.Errorf("invalid rss_retry_delay format: %w", err)
		}
		cfg.RSSRetryDelay = duration
	}

	// Parse Discord delay duration
	if generalCfg.DiscordDelay != "" {
		duration, err := time.ParseDuration(generalCfg.DiscordDelay)
		if err != nil {
			return fmt.Errorf("invalid discord_delay format: %w", err)
		}
		cfg.DiscordDelay = duration
	}

	return nil
}

// loadFeedsConfig loads the RSS feeds configuration
func loadFeedsConfig(cfg *Config, configDir string) error {
	configPath := filepath.Join(configDir, "config_feeds.json")

	data, err := os.ReadFile(configPath)
	if err != nil {
		// Feeds config is optional, log warning and continue
		log.WithField("path", configPath).Warn("Feeds config file not found, using defaults")
		return nil
	}

	if err := json.Unmarshal(data, &cfg.Feeds); err != nil {
		return fmt.Errorf("failed to parse feeds config file %s: %w", configPath, err)
	}

	return nil
}

// loadFormatConfig loads the message formatting configuration
func loadFormatConfig(cfg *Config, configDir string) error {
	configPath := filepath.Join(configDir, "config_format.json")

	data, err := os.ReadFile(configPath)
	if err != nil {
		// Format config is optional, log warning and continue
		log.WithField("path", configPath).Warn("Format config file not found, using defaults")
		return nil
	}

	if err := json.Unmarshal(data, &cfg.Format); err != nil {
		return fmt.Errorf("failed to parse format config file %s: %w", configPath, err)
	}

	return nil
}

// validateConfig validates the loaded configuration
func validateConfig(cfg *Config) error {
	// Validate log level
	switch strings.ToUpper(cfg.LogLevel) {
	case "DEBUG", "INFO", "WARNING", "ERROR":
		// Valid log levels
	default:
		return fmt.Errorf("invalid log level: %s (must be DEBUG, INFO, WARNING, or ERROR)", cfg.LogLevel)
	}

	// Validate log rotation settings
	if cfg.LogRotation.MaxSizeMB < 1 || cfg.LogRotation.MaxSizeMB > 1000 {
		return fmt.Errorf("invalid log rotation max_size_mb: %d (must be 1-1000)", cfg.LogRotation.MaxSizeMB)
	}
	if cfg.LogRotation.MaxBackups < 0 || cfg.LogRotation.MaxBackups > 50 {
		return fmt.Errorf("invalid log rotation max_backups: %d (must be 0-50)", cfg.LogRotation.MaxBackups)
	}
	if cfg.LogRotation.MaxAgeDays < 0 || cfg.LogRotation.MaxAgeDays > 365 {
		return fmt.Errorf("invalid log rotation max_age_days: %d (must be 0-365)", cfg.LogRotation.MaxAgeDays)
	}

	// Validate API key if any webhook is enabled
	hasEnabledWebhook := cfg.Webhooks.Ransomware.Enabled || cfg.Webhooks.RSS.Enabled || cfg.Webhooks.Government.Enabled
	if hasEnabledWebhook && cfg.APIKey == "" {
		log.Warn("API key is empty but webhooks are enabled")
	}

	// Validate webhook URLs
	if err := validateWebhook("ransomware", cfg.Webhooks.Ransomware); err != nil {
		return err
	}
	if err := validateWebhook("rss", cfg.Webhooks.RSS); err != nil {
		return err
	}
	if err := validateWebhook("government", cfg.Webhooks.Government); err != nil {
		return err
	}

	// Validate intervals
	if cfg.APIPollInterval < time.Minute {
		return fmt.Errorf("api_poll_interval too short: %v (minimum 1 minute)", cfg.APIPollInterval)
	}

	if cfg.RSSPollInterval < time.Minute {
		return fmt.Errorf("rss_poll_interval too short: %v (minimum 1 minute)", cfg.RSSPollInterval)
	}

	if cfg.RSSRetryDelay < time.Second {
		return fmt.Errorf("rss_retry_delay too short: %v (minimum 1 second)", cfg.RSSRetryDelay)
	}

	// Validate Discord delay
	if cfg.DiscordDelay < 0 {
		return fmt.Errorf("discord_delay cannot be negative: %v", cfg.DiscordDelay)
	}
	if cfg.DiscordDelay > 30*time.Second {
		return fmt.Errorf("discord_delay too long: %v (maximum 30 seconds)", cfg.DiscordDelay)
	}

	// Validate retry count
	if cfg.MaxRSSWorkers < 1 || cfg.MaxRSSWorkers > 10 {
		return fmt.Errorf("max_rss_workers out of range: %d (must be 1-10)", cfg.MaxRSSWorkers)
	}

	return nil
}

// validateWebhook validates a webhook configuration
func validateWebhook(name string, webhook WebhookConfig) error {
	if webhook.Enabled {
		if webhook.URL == "" {
			return fmt.Errorf("%s webhook is enabled but URL is empty", name)
		}
		if !strings.HasPrefix(webhook.URL, "https://discord.com/api/webhooks/") {
			return fmt.Errorf("%s webhook URL is not a valid Discord webhook URL", name)
		}
	}
	return nil
}
