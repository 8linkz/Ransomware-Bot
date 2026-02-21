// Package config defines all configuration structures and default values
// for the Ransomware-Bot.
//
// Configuration Philosophy:
// - Sensible defaults allow minimal setup
// - Extensive customization options for advanced users
// - Clear separation between general, feed, and format settings
// - Built-in validation prevents misconfigurations
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
	RSSRetryCount    int           `json:"rss_retry_count"`
	RSSRetryDelay    time.Duration `json:"rss_retry_delay"`
	RSSWorkerTimeout time.Duration `json:"rss_worker_timeout"` // Timeout for RSS worker pool results
	RSSMaxItemAge    time.Duration `json:"rss_max_item_age"`  // Max age of RSS items to send (0 = disabled)
	DataDir          string        `json:"data_dir"`           // Directory for persistent status data (resolved to absolute path)
	DiscordDelay     time.Duration `json:"discord_delay"`      // Configurable delay before Discord push
	RetryMaxAttempts int           `json:"retry_max_attempts"` // Max send retries per item before dead-letter (0 = unlimited)
	RetryWindow      time.Duration `json:"retry_window"`       // Max time to retry an item before dead-letter (0 = unlimited)
	DiscordWebhooks DiscordWebhooks `json:"discord_webhooks"`
	SlackDelay      time.Duration   `json:"slack_delay"`    // Configurable delay before Slack push
	SlackWebhooks   SlackWebhooks   `json:"slack_webhooks"`

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

// DiscordWebhooks configuration for Discord webhook URLs
type DiscordWebhooks struct {
	Ransomware WebhookConfig `json:"ransomware"`
	RSS        WebhookConfig `json:"rss"`
	Government WebhookConfig `json:"government"`
}

// SlackWebhooks configuration for Slack webhook URLs
type SlackWebhooks struct {
	Ransomware WebhookConfig `json:"ransomware"`
	RSS        WebhookConfig `json:"rss"`
	Government WebhookConfig `json:"government"`
}

// WebhookConfig represents a single webhook configuration
type WebhookConfig struct {
	Enabled    bool            `json:"enabled"`
	URL        string          `json:"url"`
	Filters    *WebhookFilters `json:"filters,omitempty"`
	QuietHours *QuietHours     `json:"quiet_hours,omitempty"`
}

// QuietHours defines a time window during which message delivery is paused.
// nil or enabled=false means no quiet hours (always deliver). Messages remain
// unsent and are picked up by the next polling cycle after the window ends.
type QuietHours struct {
	Enabled  bool   `json:"enabled"`            // Must be true for quiet hours to take effect
	Start    string `json:"start"`              // 24h "HH:MM" (e.g. "22:00") or 12h (e.g. "10pm", "10:00 PM")
	End      string `json:"end"`                // 24h "HH:MM" (e.g. "07:00") or 12h (e.g. "7am", "7:00 AM")
	Timezone string `json:"timezone,omitempty"` // IANA timezone, e.g. "Europe/Berlin" (default: "UTC")
}

// WebhookFilters defines per-webhook include/exclude filter rules.
// All specified field groups are AND-combined; values within a group are OR-combined.
// nil (no filters block) means accept everything (backward-compatible default).
type WebhookFilters struct {
	// Include (whitelist) — if set, entry MUST match at least one value
	IncludeGroups     []string `json:"include_groups,omitempty"`
	IncludeCountries  []string `json:"include_countries,omitempty"`
	IncludeActivities []string `json:"include_activities,omitempty"`
	IncludeKeywords   []string `json:"include_keywords,omitempty"`
	IncludeCategories []string `json:"include_categories,omitempty"` // RSS only

	// Exclude (blacklist) — if matched, entry is dropped
	ExcludeGroups     []string `json:"exclude_groups,omitempty"`
	ExcludeCountries  []string `json:"exclude_countries,omitempty"`
	ExcludeActivities []string `json:"exclude_activities,omitempty"`
	ExcludeKeywords   []string `json:"exclude_keywords,omitempty"`
	ExcludeCategories []string `json:"exclude_categories,omitempty"` // RSS only

	// keyword_match controls how keywords are matched: "literal" (default) or "regex"
	KeywordMatchMode string `json:"keyword_match,omitempty"`
}

// FeedConfig contains all RSS feed URLs organized by category
type FeedConfig struct {
	RansomwareFeeds []string `json:"ransomware_feeds"`
	GovernmentFeeds []string `json:"government_feeds"`
	GeneralFeeds    []string `json:"general_feeds"`
}

// FormatConfig defines how messages should be formatted
type FormatConfig struct {
	ShowUnicodeFlags bool              `json:"show_unicode_flags"`
	ShowEmptyFields  bool              `json:"show_empty_fields"`  // Show placeholder for missing fields
	EmptyFieldText   string            `json:"empty_field_text"`   // Placeholder text for missing fields (e.g. "N/A")
	FieldOrder       []string          `json:"field_order"`        // Discord field order
	Slack            SlackFormatConfig `json:"slack,omitempty"`    // Slack-specific formatting
}

// SlackFormatConfig defines Slack-specific formatting options
type SlackFormatConfig struct {
	TitleText  string   `json:"title_text"`
	RSSText    string   `json:"rss_text"`
	FieldOrder []string `json:"field_order"`
}

// GeneralLogRotation uses pointer fields for JSON parsing so that
// zero-values (0, false) can be distinguished from missing fields (nil).
type GeneralLogRotation struct {
	MaxSizeMB  *int  `json:"max_size_mb"`
	MaxBackups *int  `json:"max_backups"`
	MaxAgeDays *int  `json:"max_age_days"`
	Compress   *bool `json:"compress"`
}

// GeneralConfig represents the main configuration file structure.
// Pointer fields (*int) allow distinguishing "not set" (nil) from "explicitly 0".
type GeneralConfig struct {
	LogLevel        string             `json:"log_level"`
	LogRotation     GeneralLogRotation `json:"log_rotation"`
	MaxRSSWorkers   *int               `json:"max_rss_workers"`
	APIKey          string             `json:"api_key"`
	APIPollInterval string             `json:"api_poll_interval"` // Will be parsed to time.Duration
	RSSPollInterval string             `json:"rss_poll_interval"`
	RSSRetryCount    *int   `json:"rss_retry_count"`
	RSSRetryDelay    string `json:"rss_retry_delay"`    // Will be parsed to time.Duration
	RSSWorkerTimeout string `json:"rss_worker_timeout"` // Will be parsed to time.Duration
	RSSMaxItemAge    string `json:"rss_max_item_age"`   // Will be parsed to time.Duration (0 or empty = disabled)
	DataDir          string `json:"data_dir"`            // Directory for persistent status data
	DiscordDelay     string `json:"discord_delay"`       // Will be parsed to time.Duration
	DiscordWebhooks DiscordWebhooks `json:"discord_webhooks"`
	SlackDelay      string          `json:"slack_delay"` // Will be parsed to time.Duration
	SlackWebhooks   SlackWebhooks   `json:"slack_webhooks"`
	RetryMaxAttempts *int   `json:"retry_max_attempts"`
	RetryWindow      string `json:"retry_window"` // Will be parsed to time.Duration
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
		RSSRetryCount:    3,
		RSSRetryDelay:    2 * time.Second,
		RSSWorkerTimeout: 30 * time.Second, // Default 30 seconds timeout for RSS worker pool
		DataDir:          "./data",         // Default data directory (resolved to absolute path at load time)
		DiscordDelay:     2 * time.Second,  // Default 2 seconds delay before Discord push
		RetryMaxAttempts: 5,               // Default 5 retry cycles before dead-letter
		RetryWindow:      24 * time.Hour,  // Default 24h retry window
		DiscordWebhooks: DiscordWebhooks{
			Ransomware: WebhookConfig{Enabled: false, URL: ""},
			RSS:        WebhookConfig{Enabled: false, URL: ""},
			Government: WebhookConfig{Enabled: false, URL: ""},
		},
		SlackDelay: 2 * time.Second, // Default 2 seconds delay before Slack push
		SlackWebhooks: SlackWebhooks{
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
			ShowEmptyFields:  true,
			EmptyFieldText:   "N/A",
			FieldOrder: []string{
				"country",
				"victim",
				"group",
				"activity",
				"attackdate",
				"discovered",
				"post_url",
				"website",
				"description",
				"screenshot",
			},
		},
	}
}
