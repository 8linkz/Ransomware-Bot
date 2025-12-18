// Package config provides comprehensive configuration management for the Discord bot
// with validation, defaults, and multi-file JSON configuration support.
//
// Configuration Structure:
// - config_general.json: Core settings, API keys, webhooks, logging
// - config_feeds.json: RSS feed URLs organized by category
// - config_format.json: Message formatting and display options
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
//
// Loading strategy:
// 1. Start with sensible defaults from DefaultConfig()
// 2. Override with values from config_general.json (required)
// 3. Merge optional config_feeds.json and config_format.json
// 4. Validate all settings for security and operational requirements
//
// This approach ensures the bot can start even with minimal configuration
// while providing extensive customization options for advanced users.
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
	cfg.DiscordWebhooks = generalCfg.DiscordWebhooks
	cfg.SlackWebhooks = generalCfg.SlackWebhooks

	// Apply log rotation settings (use defaults if not specified)
	// Prevents misconfiguration that could exhaust disk space
	// or create too many backup files
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
	// Supports Go duration format: "1h", "30m", "2s"
	// Validation ensures minimum intervals to prevent API abuse
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

	// Parse Slack delay duration
	if generalCfg.SlackDelay != "" {
		duration, err := time.ParseDuration(generalCfg.SlackDelay)
		if err != nil {
			return fmt.Errorf("invalid slack_delay format: %w", err)
		}
		cfg.SlackDelay = duration
	}

	// Parse RSS worker timeout duration
	if generalCfg.RSSWorkerTimeout != "" {
		duration, err := time.ParseDuration(generalCfg.RSSWorkerTimeout)
		if err != nil {
			return fmt.Errorf("invalid rss_worker_timeout format: %w", err)
		}
		cfg.RSSWorkerTimeout = duration
	}

	return nil
}

// loadFeedsConfig loads the RSS feeds configuration
func loadFeedsConfig(cfg *Config, configDir string) error {
	configPath := filepath.Join(configDir, "config_feeds.json")

	data, err := os.ReadFile(configPath)
	if err != nil {
		// Feeds config is optional, log warning and continue
		// This allows the bot to start with API-only mode
		// or with default feed configurations
		log.WithField("path", configPath).Warn("Feeds config file not found, using defaults")
		return nil
	}

	if err := json.Unmarshal(data, &cfg.Feeds); err != nil {
		return fmt.Errorf("failed to parse feeds config file %s: %w", configPath, err)
	}

	// Validate RSS feed URLs
	if err := validateFeedURLs(cfg); err != nil {
		return err
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
//
// Security validations:
// - Webhook URLs must use Discord's official webhook format
// - API keys are checked if webhooks are enabled
// - Intervals have minimum limits to prevent API abuse
// - Log rotation limits prevent disk space exhaustion
//
// Operational validations:
// - Resource limits (workers, retry counts) within reasonable bounds
// - Time intervals long enough to avoid rate limiting
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

	// Warn about potentially problematic log rotation configurations
	if cfg.LogRotation.MaxBackups == 0 && cfg.LogRotation.MaxAgeDays == 0 {
		log.Warn("Log rotation: max_backups=0 and max_age_days=0 will delete all old logs immediately")
	}

	// Warn about high disk usage potential
	maxPotentialDiskUsageMB := cfg.LogRotation.MaxSizeMB * (cfg.LogRotation.MaxBackups + 1)
	if maxPotentialDiskUsageMB > 5000 {
		log.WithFields(log.Fields{
			"max_size_mb":          cfg.LogRotation.MaxSizeMB,
			"max_backups":          cfg.LogRotation.MaxBackups,
			"potential_disk_usage": fmt.Sprintf("%dMB", maxPotentialDiskUsageMB),
		}).Warn("Log rotation configuration may use significant disk space")
	}

	// Validate API key if any webhook is enabled
	hasEnabledDiscordWebhook := cfg.DiscordWebhooks.Ransomware.Enabled || cfg.DiscordWebhooks.RSS.Enabled || cfg.DiscordWebhooks.Government.Enabled
	hasEnabledSlackWebhook := cfg.SlackWebhooks.Ransomware.Enabled || cfg.SlackWebhooks.RSS.Enabled || cfg.SlackWebhooks.Government.Enabled
	if (hasEnabledDiscordWebhook || hasEnabledSlackWebhook) && cfg.APIKey == "" {
		log.Warn("API key is empty but webhooks are enabled")
	}

	// Validate Discord webhook URLs
	if err := validateDiscordWebhook("ransomware", cfg.DiscordWebhooks.Ransomware); err != nil {
		return err
	}
	if err := validateDiscordWebhook("rss", cfg.DiscordWebhooks.RSS); err != nil {
		return err
	}
	if err := validateDiscordWebhook("government", cfg.DiscordWebhooks.Government); err != nil {
		return err
	}

	// Validate Slack webhook URLs
	if err := validateSlackWebhook("ransomware", cfg.SlackWebhooks.Ransomware); err != nil {
		return err
	}
	if err := validateSlackWebhook("rss", cfg.SlackWebhooks.RSS); err != nil {
		return err
	}
	if err := validateSlackWebhook("government", cfg.SlackWebhooks.Government); err != nil {
		return err
	}

	// Validate intervals
	// Minimum 1 minute prevents API abuse and rate limiting
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
	// Minimum 400ms to avoid Discord rate limits (5 requests per 2 seconds)
	if cfg.DiscordDelay < 400*time.Millisecond {
		return fmt.Errorf("discord_delay too short: %v (minimum 400ms to avoid rate limits)", cfg.DiscordDelay)
	}
	// Maximum 30 seconds prevents excessive delays in alert delivery
	if cfg.DiscordDelay > 30*time.Second {
		return fmt.Errorf("discord_delay too long: %v (maximum 30 seconds)", cfg.DiscordDelay)
	}

	// Validate Slack delay
	if cfg.SlackDelay < 0 {
		return fmt.Errorf("slack_delay cannot be negative: %v", cfg.SlackDelay)
	}
	// Minimum 1 second to avoid Slack rate limits (1 message per second per webhook)
	if cfg.SlackDelay < time.Second {
		return fmt.Errorf("slack_delay too short: %v (minimum 1s to avoid rate limits)", cfg.SlackDelay)
	}
	// Maximum 30 seconds prevents excessive delays in alert delivery
	if cfg.SlackDelay > 30*time.Second {
		return fmt.Errorf("slack_delay too long: %v (maximum 30 seconds)", cfg.SlackDelay)
	}

	// Validate retry count
	if cfg.RSSRetryCount < 0 || cfg.RSSRetryCount > 10 {
		return fmt.Errorf("rss_retry_count out of range: %d (must be 0-10)", cfg.RSSRetryCount)
	}

	// Validate RSS workers
	if cfg.MaxRSSWorkers < 1 || cfg.MaxRSSWorkers > 10 {
		return fmt.Errorf("max_rss_workers out of range: %d (must be 1-10)", cfg.MaxRSSWorkers)
	}

	// Validate RSS worker timeout
	if cfg.RSSWorkerTimeout < 5*time.Second || cfg.RSSWorkerTimeout > 5*time.Minute {
		return fmt.Errorf("rss_worker_timeout out of range: %v (must be 5s-5m)", cfg.RSSWorkerTimeout)
	}

	return nil
}

// validateDiscordWebhook validates a Discord webhook configuration
//
// Security checks:
// - Ensures webhook URL is from Discord's official domain
// - Prevents configuration of malicious webhook endpoints
// - URL format validation prevents injection attacks
//
// Only validates format - does not test webhook functionality
func validateDiscordWebhook(name string, webhook WebhookConfig) error {
	if webhook.Enabled {
		if webhook.URL == "" {
			return fmt.Errorf("discord %s webhook is enabled but URL is empty", name)
		}
		if !strings.HasPrefix(webhook.URL, "https://discord.com/api/webhooks/") {
			return fmt.Errorf("discord %s webhook URL is not a valid Discord webhook URL", name)
		}
	}
	return nil
}

// validateSlackWebhook validates a Slack webhook configuration
//
// Security checks:
// - Ensures webhook URL is from Slack's official domain
// - Prevents configuration of malicious webhook endpoints
// - URL format validation prevents injection attacks
//
// Only validates format - does not test webhook functionality
func validateSlackWebhook(name string, webhook WebhookConfig) error {
	if webhook.Enabled {
		if webhook.URL == "" {
			return fmt.Errorf("slack %s webhook is enabled but URL is empty", name)
		}
		if !strings.HasPrefix(webhook.URL, "https://hooks.slack.com/services/") {
			return fmt.Errorf("slack %s webhook URL is not a valid Slack webhook URL (must start with https://hooks.slack.com/services/)", name)
		}
	}
	return nil
}

// validateFeedURLs validates all RSS feed URLs to prevent security issues
//
// Security checks:
// - Only allows http:// and https:// protocols
// - Prevents file:// protocol (local file access)
// - Prevents ftp:// and other protocols
// - Mitigates SSRF attacks by restricting protocols
func validateFeedURLs(cfg *Config) error {
	// Validate all ransomware feeds
	for i, url := range cfg.Feeds.RansomwareFeeds {
		if err := validateFeedURL(url, fmt.Sprintf("ransomware_feeds[%d]", i)); err != nil {
			return err
		}
	}

	// Validate all government feeds
	for i, url := range cfg.Feeds.GovernmentFeeds {
		if err := validateFeedURL(url, fmt.Sprintf("government_feeds[%d]", i)); err != nil {
			return err
		}
	}

	// Validate all general feeds
	for i, url := range cfg.Feeds.GeneralFeeds {
		if err := validateFeedURL(url, fmt.Sprintf("general_feeds[%d]", i)); err != nil {
			return err
		}
	}

	return nil
}

// validateFeedURL validates a single RSS feed URL
func validateFeedURL(url, name string) error {
	if url == "" {
		return fmt.Errorf("feed '%s' has empty URL", name)
	}

	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("feed '%s' URL must use http or https protocol: %s", name, url)
	}

	return nil
}
