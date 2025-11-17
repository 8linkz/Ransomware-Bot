package scheduler

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"Ransomware-Bot/internal/api"
	"Ransomware-Bot/internal/config"
	"Ransomware-Bot/internal/discord"
	"Ransomware-Bot/internal/rss"
	"Ransomware-Bot/internal/slack"
	"Ransomware-Bot/internal/status"

	log "github.com/sirupsen/logrus"
)

// Scheduler manages the execution of API polling and RSS feed checking
type Scheduler struct {
	config             *config.Config
	apiClient          *api.Client
	rssParser          *rss.Parser
	discordWebhookSender *discord.WebhookSender
	slackWebhookSender   *slack.WebhookSender
	statusTracker      *status.Tracker

	// Channels for graceful shutdown
	stopChan chan struct{}
	wg       sync.WaitGroup

	// Tickers for scheduled tasks
	apiTicker *time.Ticker
	rssTicker *time.Ticker
}

// New creates a new scheduler instance
func New(cfg *config.Config) (*Scheduler, error) {
	// Initialize API client
	apiClient, err := api.NewClient(cfg.APIKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create API client: %w", err)
	}

	// Initialize status tracker FIRST
	statusTracker, err := status.NewTracker("data")
	if err != nil {
		return nil, fmt.Errorf("failed to create status tracker: %w", err)
	}

	// Initialize RSS parser with status tracker for persistent deduplication
	rssParser := rss.NewParser(cfg.RSSRetryCount, cfg.RSSRetryDelay, statusTracker)

	// Initialize webhook senders
	discordWebhookSender := discord.NewWebhookSender()
	slackWebhookSender := slack.NewWebhookSender()

	return &Scheduler{
		config:               cfg,
		apiClient:            apiClient,
		rssParser:            rssParser,
		discordWebhookSender: discordWebhookSender,
		slackWebhookSender:   slackWebhookSender,
		statusTracker:        statusTracker,
		stopChan:             make(chan struct{}),
	}, nil
}

// Start begins the scheduler operations
func (s *Scheduler) Start(ctx context.Context) error {
	log.Info("Starting scheduler")

	// Print status summary at startup
	log.Info("Starting scheduler - status tracker initialized")

	// Create tickers for periodic tasks
	s.apiTicker = time.NewTicker(s.config.APIPollInterval)
	s.rssTicker = time.NewTicker(s.config.RSSPollInterval)

	// Start API polling goroutine
	s.wg.Add(1)
	go s.runAPIPoller(ctx)

	// Start RSS feed checking goroutine
	s.wg.Add(1)
	go s.runRSSChecker(ctx)

	// Run initial checks immediately
	go s.checkAPIOnce(ctx)
	go s.checkRSSOnce(ctx)

	log.Info("Scheduler started successfully")
	return nil
}

// Stop gracefully shuts down the scheduler
func (s *Scheduler) Stop() {
	log.Info("Stopping scheduler")

	// Signal all goroutines to stop
	close(s.stopChan)

	// Stop tickers
	if s.apiTicker != nil {
		s.apiTicker.Stop()
	}
	if s.rssTicker != nil {
		s.rssTicker.Stop()
	}

	// Wait for all goroutines to finish
	s.wg.Wait()

	// Print final status summary
	log.Info("Scheduler stopped - final status logged")

	log.Info("Scheduler stopped")
}

// runAPIPoller runs the API polling loop
func (s *Scheduler) runAPIPoller(ctx context.Context) {
	defer s.wg.Done()

	log.Info("API poller started")

	for {
		select {
		case <-ctx.Done():
			log.Info("API poller stopping due to context cancellation")
			return
		case <-s.stopChan:
			log.Info("API poller stopping due to stop signal")
			return
		case <-s.apiTicker.C:
			s.checkAPIOnce(ctx)
		}
	}
}

// runRSSChecker runs the RSS feed checking loop
func (s *Scheduler) runRSSChecker(ctx context.Context) {
	defer s.wg.Done()

	log.Info("RSS checker started")

	for {
		select {
		case <-ctx.Done():
			log.Info("RSS checker stopping due to context cancellation")
			return
		case <-s.stopChan:
			log.Info("RSS checker stopping due to stop signal")
			return
		case <-s.rssTicker.C:
			s.checkRSSOnce(ctx)
		}
	}
}

// checkAPIOnce performs a single API check with individual sending
func (s *Scheduler) checkAPIOnce(ctx context.Context) {
	if s.config.APIKey == "" {
		log.Debug("API key not configured, skipping API check")
		return
	}

	log.Debug("Checking API for new ransomware data")

	// Get new ransomware entries from API (marks as fetched)
	newEntries, err := s.apiClient.GetLatestEntries(ctx, s.statusTracker)
	if err != nil {
		log.WithError(err).Error("Failed to get API data")
		s.statusTracker.UpdateAPIStatus(false, 0, err.Error())
		return
	}

	// Update status with success
	s.statusTracker.UpdateAPIStatus(true, len(newEntries), "")

	// Send entries individually to Discord (webhook-specific tracking)
	if s.config.DiscordWebhooks.Ransomware.Enabled {
		discordUnsent := s.statusTracker.GetUnsentAPIItemsForWebhook(s.config.DiscordWebhooks.Ransomware.URL)
		log.WithFields(log.Fields{
			"webhook_enabled": true,
			"unsent_count":    len(discordUnsent),
			"total_fetched":   len(newEntries),
		}).Debug("Checking Discord unsent items")

		if len(discordUnsent) > 0 {
			log.WithFields(log.Fields{
				"new_entries":    len(newEntries),
				"discord_unsent": len(discordUnsent),
			}).Info("Found unsent API entries for Discord")
			// Convert StoredRansomwareEntry to api.RansomwareEntry
			apiEntries := s.convertStoredToAPIEntries(discordUnsent)
			s.batchSendAPIEntriesToDiscord(ctx, apiEntries, s.config.DiscordWebhooks.Ransomware.URL)
		} else {
			log.Debug("No unsent Discord items found")
		}
	}

	// Send entries individually to Slack (webhook-specific tracking)
	if s.config.SlackWebhooks.Ransomware.Enabled {
		slackUnsent := s.statusTracker.GetUnsentAPIItemsForWebhook(s.config.SlackWebhooks.Ransomware.URL)
		log.WithFields(log.Fields{
			"webhook_enabled": true,
			"unsent_count":    len(slackUnsent),
			"total_fetched":   len(newEntries),
		}).Debug("Checking Slack unsent items")

		if len(slackUnsent) > 0 {
			log.WithFields(log.Fields{
				"new_entries":  len(newEntries),
				"slack_unsent": len(slackUnsent),
			}).Info("Found unsent API entries for Slack")
			// Convert StoredRansomwareEntry to api.RansomwareEntry
			apiEntries := s.convertStoredToAPIEntries(slackUnsent)
			s.batchSendAPIEntriesToSlack(ctx, apiEntries, s.config.SlackWebhooks.Ransomware.URL)
		} else {
			log.Debug("No unsent Slack items found")
		}
	}
}


// bulkSendAPIEntriesToDiscord sends API entries to Discord individually with rate limiting
func (s *Scheduler) bulkSendAPIEntriesToDiscord(ctx context.Context, entries []api.RansomwareEntry, webhookURL string) {
	if len(entries) == 0 {
		log.Debug("No entries to send")
		return
	}

	log.WithField("total_entries", len(entries)).Info("Starting individual send to Discord")

	// Send each entry individually with configurable delay
	successCount := 0
	for _, entry := range entries {
		// Check if context is cancelled
		select {
		case <-ctx.Done():
			log.WithField("sent", successCount).Warn("Context cancelled, stopping Discord send")
			return
		default:
		}

		// Configurable delay before sending to Discord
		time.Sleep(s.config.DiscordDelay)
		// Send the entry to Discord webhook
		if err := s.discordWebhookSender.SendRansomwareEntry(ctx, webhookURL, entry, &s.config.Format); err != nil {
			log.WithError(err).WithFields(log.Fields{
				"group":  entry.Group,
				"victim": entry.Victim,
			}).Error("Failed to send ransomware entry to webhook")
		} else {
			// Mark as sent after successful transmission
			key := s.generateAPIEntryKey(entry)
			title := entry.Group + " -> " + entry.Victim
			s.statusTracker.MarkAPIItemSent(key, title, webhookURL)
			successCount++
		}
	}

	log.WithFields(log.Fields{
		"total_sent":    successCount,
		"total_entries": len(entries),
	}).Info("Individual send to Discord completed")
}

// batchSendAPIEntriesToDiscord sends API entries to Discord in batches (10 per message)
func (s *Scheduler) batchSendAPIEntriesToDiscord(ctx context.Context, entries []api.RansomwareEntry, webhookURL string) {
	if len(entries) == 0 {
		log.Debug("No entries to send")
		return
	}

	log.WithField("total_entries", len(entries)).Info("Starting batch send to Discord")

	// Send entries in batches using the new batch method
	if err := s.discordWebhookSender.SendRansomwareEntriesBatch(ctx, webhookURL, entries, &s.config.Format); err != nil {
		log.WithError(err).Error("Failed to send ransomware entries batch to Discord")
		return
	}

	// Mark all entries as sent after successful transmission
	successCount := 0
	for _, entry := range entries {
		key := s.generateAPIEntryKey(entry)
		title := entry.Group + " -> " + entry.Victim
		s.statusTracker.MarkAPIItemSent(key, title, webhookURL)
		successCount++
	}

	log.WithFields(log.Fields{
		"total_sent":    successCount,
		"total_entries": len(entries),
	}).Info("Batch send to Discord completed")
}

// bulkSendAPIEntriesToSlack sends API entries to Slack individually with rate limiting
func (s *Scheduler) bulkSendAPIEntriesToSlack(ctx context.Context, entries []api.RansomwareEntry, webhookURL string) {
	if len(entries) == 0 {
		log.Debug("No entries to send")
		return
	}

	log.WithField("total_entries", len(entries)).Info("Starting individual send to Slack")

	// Send each entry individually with configurable delay
	successCount := 0
	for _, entry := range entries {
		// Check if context is cancelled
		select {
		case <-ctx.Done():
			log.WithField("sent", successCount).Warn("Context cancelled, stopping Slack send")
			return
		default:
		}

		// Configurable delay before sending to Slack
		time.Sleep(s.config.SlackDelay)
		// Send the entry to Slack webhook
		if err := s.slackWebhookSender.SendRansomwareEntry(ctx, webhookURL, entry, &s.config.Format); err != nil {
			log.WithError(err).WithFields(log.Fields{
				"group":  entry.Group,
				"victim": entry.Victim,
			}).Error("Failed to send ransomware entry to Slack webhook")
		} else {
			// Mark as sent after successful transmission
			key := s.generateAPIEntryKey(entry)
			title := entry.Group + " -> " + entry.Victim
			s.statusTracker.MarkAPIItemSent(key, title, webhookURL)
			successCount++
		}
	}

	log.WithFields(log.Fields{
		"total_sent":    successCount,
		"total_entries": len(entries),
	}).Info("Individual send to Slack completed")
}

// batchSendAPIEntriesToSlack sends API entries to Slack with rate limiting (batches of 10 for logging)
func (s *Scheduler) batchSendAPIEntriesToSlack(ctx context.Context, entries []api.RansomwareEntry, webhookURL string) {
	if len(entries) == 0 {
		log.Debug("No entries to send")
		return
	}

	log.WithField("total_entries", len(entries)).Info("Starting batch send to Slack")

	// Send entries using batch method (still sends individually but with batch tracking)
	if err := s.slackWebhookSender.SendRansomwareEntriesBatch(ctx, webhookURL, entries, &s.config.Format); err != nil {
		log.WithError(err).Error("Failed to send ransomware entries batch to Slack")
		return
	}

	// Mark all entries as sent after successful transmission
	successCount := 0
	for _, entry := range entries {
		key := s.generateAPIEntryKey(entry)
		title := entry.Group + " -> " + entry.Victim
		s.statusTracker.MarkAPIItemSent(key, title, webhookURL)
		successCount++
	}

	log.WithFields(log.Fields{
		"total_sent":    successCount,
		"total_entries": len(entries),
	}).Info("Batch send to Slack completed")
}

// generateAPIEntryKey creates a unique key for an API entry (same logic as in api package)
func (s *Scheduler) generateAPIEntryKey(entry api.RansomwareEntry) string {
	// Use ID if available
	if entry.ID != "" {
		return "id:" + entry.ID
	}

	// Use a stable combination that won't change
	// Group + Victim should be unique enough for ransomware incidents
	baseKey := entry.Group + ":" + entry.Victim

	// Add country if available for additional uniqueness
	if entry.Country != "" {
		baseKey += ":" + entry.Country
	}

	// Only add attack date if available (more stable than discovered time)
	if entry.AttackDate != "" {
		baseKey += ":" + entry.AttackDate
	}

	return baseKey
}

// checkRSSOnce performs a single RSS feed check with recovery
func (s *Scheduler) checkRSSOnce(ctx context.Context) {
	log.Debug("Checking RSS feeds for new entries")

	// Check general feeds for Discord
	if s.config.DiscordWebhooks.RSS.Enabled && len(s.config.Feeds.GeneralFeeds) > 0 {
		s.checkRSSFeeds(ctx, s.config.Feeds.GeneralFeeds, s.config.DiscordWebhooks.RSS.URL, "general", "discord")
	}

	// Check government feeds for Discord
	if s.config.DiscordWebhooks.Government.Enabled && len(s.config.Feeds.GovernmentFeeds) > 0 {
		s.checkRSSFeeds(ctx, s.config.Feeds.GovernmentFeeds, s.config.DiscordWebhooks.Government.URL, "government", "discord")
	}

	// Check ransomware feeds for Discord
	if s.config.DiscordWebhooks.Ransomware.Enabled && len(s.config.Feeds.RansomwareFeeds) > 0 {
		s.checkRSSFeeds(ctx, s.config.Feeds.RansomwareFeeds, s.config.DiscordWebhooks.Ransomware.URL, "ransomware", "discord")
	}

	// Check general feeds for Slack
	if s.config.SlackWebhooks.RSS.Enabled && len(s.config.Feeds.GeneralFeeds) > 0 {
		s.checkRSSFeeds(ctx, s.config.Feeds.GeneralFeeds, s.config.SlackWebhooks.RSS.URL, "general", "slack")
	}

	// Check government feeds for Slack
	if s.config.SlackWebhooks.Government.Enabled && len(s.config.Feeds.GovernmentFeeds) > 0 {
		s.checkRSSFeeds(ctx, s.config.Feeds.GovernmentFeeds, s.config.SlackWebhooks.Government.URL, "government", "slack")
	}

	// Check ransomware feeds for Slack
	if s.config.SlackWebhooks.Ransomware.Enabled && len(s.config.Feeds.RansomwareFeeds) > 0 {
		s.checkRSSFeeds(ctx, s.config.Feeds.RansomwareFeeds, s.config.SlackWebhooks.Ransomware.URL, "ransomware", "slack")
	}

	// Check for unsent RSS items and send them (Recovery Phase)
	s.sendUnsentRSSItems(ctx)
}

// sendUnsentRSSItems sends all RSS items that were parsed but not yet sent to Discord
func (s *Scheduler) sendUnsentRSSItems(ctx context.Context) {
	// Check if context is cancelled before starting
	select {
	case <-ctx.Done():
		log.Debug("RSS unsent items sending cancelled due to context")
		return
	default:
	}

	// Get all unsent RSS items from status tracker (already sorted chronologically)
	unsentRSSItems := s.statusTracker.GetUnsentRSSItemsSorted()

	if len(unsentRSSItems) == 0 {
		log.Debug("No unsent RSS items found")
		return
	}

	log.WithField("unsent_count", len(unsentRSSItems)).Info("Found unsent RSS items, sending to webhooks")

	// Group by feed type for correct webhook routing
	generalItems := []status.StoredRSSEntry{}
	governmentItems := []status.StoredRSSEntry{}
	ransomwareItems := []status.StoredRSSEntry{}

	// Categorize items by feed URL
	for _, item := range unsentRSSItems {
		if s.isGeneralFeed(item.FeedURL) {
			generalItems = append(generalItems, item)
		} else if s.isGovernmentFeed(item.FeedURL) {
			governmentItems = append(governmentItems, item)
		} else if s.isRansomwareFeed(item.FeedURL) {
			ransomwareItems = append(ransomwareItems, item)
		}
	}

	// Send to appropriate Discord webhooks
	if len(generalItems) > 0 && s.config.DiscordWebhooks.RSS.Enabled {
		s.sendRSSItemsToWebhook(ctx, generalItems, s.config.DiscordWebhooks.RSS.URL, "general", "discord")
	}

	if len(governmentItems) > 0 && s.config.DiscordWebhooks.Government.Enabled {
		s.sendRSSItemsToWebhook(ctx, governmentItems, s.config.DiscordWebhooks.Government.URL, "government", "discord")
	}

	if len(ransomwareItems) > 0 && s.config.DiscordWebhooks.Ransomware.Enabled {
		s.sendRSSItemsToWebhook(ctx, ransomwareItems, s.config.DiscordWebhooks.Ransomware.URL, "ransomware", "discord")
	}

	// Send to appropriate Slack webhooks
	if len(generalItems) > 0 && s.config.SlackWebhooks.RSS.Enabled {
		s.sendRSSItemsToWebhook(ctx, generalItems, s.config.SlackWebhooks.RSS.URL, "general", "slack")
	}

	if len(governmentItems) > 0 && s.config.SlackWebhooks.Government.Enabled {
		s.sendRSSItemsToWebhook(ctx, governmentItems, s.config.SlackWebhooks.Government.URL, "government", "slack")
	}

	if len(ransomwareItems) > 0 && s.config.SlackWebhooks.Ransomware.Enabled {
		s.sendRSSItemsToWebhook(ctx, ransomwareItems, s.config.SlackWebhooks.Ransomware.URL, "ransomware", "slack")
	}
}

// isGeneralFeed checks if a feed URL belongs to general feeds
func (s *Scheduler) isGeneralFeed(feedURL string) bool {
	for _, generalFeedURL := range s.config.Feeds.GeneralFeeds {
		if feedURL == generalFeedURL {
			return true
		}
	}
	return false
}

// isGovernmentFeed checks if a feed URL belongs to government feeds
func (s *Scheduler) isGovernmentFeed(feedURL string) bool {
	for _, govFeedURL := range s.config.Feeds.GovernmentFeeds {
		if feedURL == govFeedURL {
			return true
		}
	}
	return false
}

// isRansomwareFeed checks if a feed URL belongs to ransomware feeds
func (s *Scheduler) isRansomwareFeed(feedURL string) bool {
	for _, ransomwareFeedURL := range s.config.Feeds.RansomwareFeeds {
		if feedURL == ransomwareFeedURL {
			return true
		}
	}
	return false
}

// sendRSSItemsToWebhook sends RSS items to a webhook (Discord or Slack) with Phase 2 tracking
func (s *Scheduler) sendRSSItemsToWebhook(ctx context.Context, items []status.StoredRSSEntry, webhookURL string, feedType string, messenger string) {
	if len(items) == 0 {
		log.Debug("No RSS items to send")
		return
	}

	log.WithFields(log.Fields{
		"total_items": len(items),
		"feed_type":   feedType,
		"messenger":   messenger,
	}).Info("Starting RSS items send to webhook")

	successCount := 0
	for _, storedItem := range items {
		// Check if context is cancelled
		select {
		case <-ctx.Done():
			log.WithFields(log.Fields{
				"sent":      successCount,
				"messenger": messenger,
			}).Warn("Context cancelled, stopping RSS send")
			return
		default:
		}

		// Convert stored RSS entry back to rss.Entry for sending
		rssEntry := s.convertStoredToRSSEntry(storedItem)

		var err error

		switch messenger {
		case "discord":
			// Configurable delay before sending to Discord
			time.Sleep(s.config.DiscordDelay)
			err = s.discordWebhookSender.SendRSSEntry(ctx, webhookURL, rssEntry)
		case "slack":
			// Configurable delay before sending to Slack
			time.Sleep(s.config.SlackDelay)
			err = s.slackWebhookSender.SendRSSEntry(ctx, webhookURL, rssEntry, &s.config.Format)
		}

		if err != nil {
			log.WithError(err).WithFields(log.Fields{
				"title":     storedItem.Title,
				"feed_url":  storedItem.FeedURL,
				"messenger": messenger,
			}).Error("Failed to send RSS entry to webhook")
		} else {
			// Mark as sent after successful transmission (Phase 2)
			s.statusTracker.MarkRSSItemSent(storedItem.Key, storedItem.Title, storedItem.FeedTitle)
			successCount++
		}
	}

	log.WithFields(log.Fields{
		"total_sent":  successCount,
		"total_items": len(items),
		"feed_type":   feedType,
		"messenger":   messenger,
	}).Info("RSS items send to webhook completed")
}

// convertStoredToRSSEntry converts a StoredRSSEntry back to rss.Entry for Discord sending
func (s *Scheduler) convertStoredToRSSEntry(stored status.StoredRSSEntry) rss.Entry {
	// Parse published time from stored string
	published, err := time.Parse("2006-01-02 15:04:05.999999", stored.Published)
	if err != nil {
		// Fallback to simpler format if parsing fails
		published, _ = time.Parse("2006-01-02 15:04:05", stored.Published)
	}

	return rss.Entry{
		Title:       stored.Title,
		Link:        stored.Link,
		Description: stored.Description,
		Published:   published,
		Author:      stored.Author,
		Categories:  stored.Categories,
		GUID:        stored.GUID,
		FeedTitle:   stored.FeedTitle,
		FeedURL:     stored.FeedURL,
	}
}

// checkRSSFeeds checks a specific set of RSS feeds with worker pool limiting
func (s *Scheduler) checkRSSFeeds(ctx context.Context, feedURLs []string, webhookURL string, feedType string, messenger string) {
	if len(feedURLs) == 0 {
		log.WithField("feed_type", feedType).Debug("No RSS feeds configured")
		return
	}

	log.WithFields(log.Fields{
		"feed_count":  len(feedURLs),
		"feed_type":   feedType,
		"messenger":   messenger,
		"max_workers": s.config.MaxRSSWorkers,
	}).Debug("Checking RSS feeds with worker pool")

	// Parse multiple RSS feeds with worker pool limiting
	results, err := s.rssParser.ParseMultipleFeeds(ctx, feedURLs, s.config.MaxRSSWorkers)
	if err != nil {
		log.WithError(err).WithField("feed_type", feedType).Error("Failed to parse RSS feeds with worker pool")
		return
	}

	// Process results for each feed
	totalNewEntries := 0
	for feedURL, entries := range results {
		// Update feed status
		s.statusTracker.UpdateFeedStatus(feedURL, true, len(entries), "")

		if len(entries) == 0 {
			log.WithField("feed_url", feedURL).Debug("No new RSS entries found")
			continue
		}

		log.WithFields(log.Fields{
			"feed_url": feedURL,
			"count":    len(entries),
		}).Info("Found new RSS entries")

		// Sort RSS entries by published date (oldest first)
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Published.Before(entries[j].Published)
		})

		// Send entries to webhook in chronological order with configurable delay
		for _, entry := range entries {
			// Check if context is cancelled
			select {
			case <-ctx.Done():
				log.WithField("messenger", messenger).Warn("Context cancelled, stopping RSS entry send")
				return
			default:
			}

			var err error

			switch messenger {
			case "discord":
				// Configurable delay before sending to Discord
				time.Sleep(s.config.DiscordDelay)
				err = s.discordWebhookSender.SendRSSEntry(ctx, webhookURL, entry)
			case "slack":
				// Configurable delay before sending to Slack
				time.Sleep(s.config.SlackDelay)
				err = s.slackWebhookSender.SendRSSEntry(ctx, webhookURL, entry, &s.config.Format)
			}

			if err != nil {
				log.WithError(err).WithFields(log.Fields{
					"feed_url":  feedURL,
					"messenger": messenger,
				}).Error("Failed to send RSS entry to webhook")
			} else {
				// Mark as sent after successful transmission (Phase 2 for new entries)
				key := s.generateRSSEntryKey(feedURL, entry)
				s.statusTracker.MarkRSSItemSent(key, entry.Title, entry.FeedTitle)
			}
		}

		totalNewEntries += len(entries)
	}

	log.WithFields(log.Fields{
		"feed_type":     feedType,
		"total_feeds":   len(feedURLs),
		"total_entries": totalNewEntries,
		"workers_used":  s.config.MaxRSSWorkers,
	}).Info("RSS feed processing completed with worker pool")
}

// convertStoredToAPIEntries converts []status.StoredRansomwareEntry to []api.RansomwareEntry
func (s *Scheduler) convertStoredToAPIEntries(stored []status.StoredRansomwareEntry) []api.RansomwareEntry {
	result := make([]api.RansomwareEntry, 0, len(stored))

	for _, entry := range stored {
		// Parse discovered time with proper error handling
		var discovered time.Time
		if entry.Discovered != "" {
			var err error
			discovered, err = time.Parse("2006-01-02 15:04:05.999999", entry.Discovered)
			if err != nil {
				// Try without microseconds
				discovered, err = time.Parse("2006-01-02 15:04:05", entry.Discovered)
				if err != nil {
					// Log error and use current time as fallback
					log.WithFields(log.Fields{
						"discovered": entry.Discovered,
						"group":      entry.Group,
						"victim":     entry.Victim,
					}).Warn("Failed to parse discovered time, using current time")
					discovered = time.Now()
				}
			}
		} else {
			// Empty string - use current time
			discovered = time.Now()
		}

		// Parse published time with proper error handling
		var published time.Time
		if entry.Published != "" {
			var err error
			published, err = time.Parse("2006-01-02 15:04:05.999999", entry.Published)
			if err != nil {
				// Try without microseconds
				published, err = time.Parse("2006-01-02 15:04:05", entry.Published)
				if err != nil {
					// Log error and leave as zero time
					log.WithFields(log.Fields{
						"published": entry.Published,
						"group":     entry.Group,
						"victim":    entry.Victim,
					}).Warn("Failed to parse published time, leaving empty")
					// published remains zero time
				}
			}
		}

		apiEntry := api.RansomwareEntry{
			ID:          entry.ID,
			Group:       entry.Group,
			Victim:      entry.Victim,
			Country:     entry.Country,
			Activity:    entry.Activity,
			AttackDate:  entry.AttackDate,
			Discovered:  api.CustomTime{Time: discovered},
			ClaimURL:    entry.ClaimURL,
			URL:         entry.URL,
			Description: entry.Description,
			Screenshot:  entry.Screenshot,
			Published:   api.CustomTime{Time: published},
		}

		result = append(result, apiEntry)
	}

	return result
}

// generateRSSEntryKey creates a unique key for an RSS entry (same logic as in rss package)
func (s *Scheduler) generateRSSEntryKey(feedURL string, entry rss.Entry) string {
	// Prefer GUID if available
	if entry.GUID != "" {
		return feedURL + ":" + entry.GUID
	}

	// Fallback to link if available
	if entry.Link != "" {
		return feedURL + ":" + entry.Link
	}

	// Fallback to title + publication date
	// Use the same logic as parser.go - check if Published is zero time
	dateStr := ""
	if !entry.Published.IsZero() {
		dateStr = entry.Published.Format(time.RFC3339)
	}

	return feedURL + ":" + entry.Title + ":" + dateStr
}
