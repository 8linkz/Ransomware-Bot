package config

import "time"

// Config represents the complete application configuration
type Config struct {
	// General configuration
	LogLevel        string        `json:"log_level"`
	LogRotation     LogRotation   `json:"log_rotation"`
	MaxRSSWorkers   int           `json:"max_rss_workers"`
	APIKey          string        `json:"api_key"`
	APIPollInterval time.Duration `json:"api_poll_interval"`
	RSSPollInterval time.Duration `json:"rss_poll_interval"`
	APIStartTime    string        `json:"api_start_time"`
	RSSRetryCount   int           `json:"rss_retry_count"`
	RSSRetryDelay   time.Duration `json:"rss_retry_delay"`
	DiscordDelay    time.Duration `json:"discord_delay"` // Configurable delay before Discord push
	Webhooks        Webhooks      `json:"webhooks"`

	// Feed configuration
	Feeds FeedConfig `json:"feeds"`

	// Format configuration
	Format FormatConfig `json:"format"`
}

// LogRotation defines log rotation settings
type LogRotation struct {
	MaxSizeMB  int  `json:"max_size_mb"`  // Maximum size in MB before rotation
	MaxBackups int  `json:"max_backups"`  // Maximum number of old log files to keep
	MaxAgeDays int  `json:"max_age_days"` // Maximum number of days to retain log files
	Compress   bool `json:"compress"`     // Whether to compress old log files
}

// Webhooks configuration for Discord webhook URLs
type Webhooks struct {
	Ransomware WebhookConfig `json:"ransomware"`
	RSS        WebhookConfig `json:"rss"`
	Government WebhookConfig `json:"government"`
}

// WebhookConfig represents a single webhook configuration
type WebhookConfig struct {
	Enabled bool   `json:"enabled"`
	URL     string `json:"url"`
}

// FeedConfig contains all RSS feed URLs organized by category
type FeedConfig struct {
	RansomwareFeeds []string `json:"ransomware_feeds"`
	GovernmentFeeds []string `json:"government_feeds"`
	GeneralFeeds    []string `json:"general_feeds"`
}

// FormatConfig defines how messages should be formatted
type FormatConfig struct {
	ShowUnicodeFlags bool     `json:"show_unicode_flags"`
	FieldOrder       []string `json:"field_order"`
}

// GeneralConfig represents the main configuration file structure
type GeneralConfig struct {
	LogLevel        string      `json:"log_level"`
	LogRotation     LogRotation `json:"log_rotation"`
	MaxRSSWorkers   int         `json:"max_rss_workers"`
	APIKey          string      `json:"api_key"`
	APIPollInterval string      `json:"api_poll_interval"` // Will be parsed to time.Duration
	RSSPollInterval string      `json:"rss_poll_interval"`
	APIStartTime    string      `json:"api_start_time"`
	RSSRetryCount   int         `json:"rss_retry_count"`
	RSSRetryDelay   string      `json:"rss_retry_delay"` // Will be parsed to time.Duration
	DiscordDelay    string      `json:"discord_delay"`   // Will be parsed to time.Duration
	Webhooks        Webhooks    `json:"webhooks"`
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		LogLevel:      "INFO",
		MaxRSSWorkers: 5, // Default number of RSS workers
		LogRotation: LogRotation{
			MaxSizeMB:  10,   // 10 MB per file
			MaxBackups: 5,    // Keep 5 old files
			MaxAgeDays: 7,    // Delete files older than 7 days
			Compress:   true, // Compress old files
		},
		APIPollInterval: time.Hour,
		RSSPollInterval: 30 * time.Minute, // NEW: Default RSS poll interval
		APIStartTime:    "",
		RSSRetryCount:   3,
		RSSRetryDelay:   2 * time.Second,
		DiscordDelay:    2 * time.Second, // NEW: Default 2 seconds delay before Discord push
		Webhooks: Webhooks{
			Ransomware: WebhookConfig{Enabled: false, URL: ""},
			RSS:        WebhookConfig{Enabled: false, URL: ""},
			Government: WebhookConfig{Enabled: false, URL: ""},
		},
		Feeds: FeedConfig{
			RansomwareFeeds: []string{},
			GovernmentFeeds: []string{},
			GeneralFeeds:    []string{},
		},
		Format: FormatConfig{
			ShowUnicodeFlags: true,
			FieldOrder: []string{
				"country",
				"victim",
				"group",
				"activity",
				"attackdate",
				"discovered",
				"claim_url",
				"url",
				"description",
				"screenshot",
			},
		},
	}
}
