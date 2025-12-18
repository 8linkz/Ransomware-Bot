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

	// Feed type lookup map for O(1) categorization
	feedTypeMap map[string]string

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
	rssParser := rss.NewParser(cfg.RSSRetryCount, cfg.RSSRetryDelay, cfg.RSSWorkerTimeout, statusTracker)

	// Initialize webhook senders
	discordWebhookSender, err := discord.NewWebhookSender()
	if err != nil {
		return nil, fmt.Errorf("failed to create Discord webhook sender: %w", err)
	}
	slackWebhookSender := slack.NewWebhookSender()

	// Build feed type lookup map for O(1) categorization
	feedTypeMap := buildFeedTypeMap(cfg)

	return &Scheduler{
		config:               cfg,
		apiClient:            apiClient,
		rssParser:            rssParser,
		discordWebhookSender: discordWebhookSender,
		slackWebhookSender:   slackWebhookSender,
		statusTracker:        statusTracker,
		feedTypeMap:          feedTypeMap,
		stopChan:             make(chan struct{}),
	}, nil
}

// buildFeedTypeMap creates a lookup map for O(1) feed type categorization
func buildFeedTypeMap(cfg *config.Config) map[string]string {
	feedTypeMap := make(map[string]string)

	for _, url := range cfg.Feeds.GeneralFeeds {
		feedTypeMap[url] = "general"
	}
	for _, url := range cfg.Feeds.GovernmentFeeds {
		feedTypeMap[url] = "government"
	}
	for _, url := range cfg.Feeds.RansomwareFeeds {
		feedTypeMap[url] = "ransomware"
	}

	return feedTypeMap
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

	// Run initial checks immediately (tracked in WaitGroup for graceful shutdown)
	s.wg.Add(2)
	go func() {
		defer s.wg.Done()
		s.checkAPIOnce(ctx)
	}()
	go func() {
		defer s.wg.Done()
		s.checkRSSOnce(ctx)
	}()

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

	// Wait for all goroutines to finish with timeout to prevent deadlock
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Info("All goroutines stopped gracefully")
	case <-time.After(30 * time.Second):
		log.Warn("Timeout waiting for goroutines to stop - forcing shutdown")
	}

	// Close all clients to release resources
	if s.apiClient != nil {
		if err := s.apiClient.Close(); err != nil {
			log.WithError(err).Warn("Failed to close API client")
		}
	}
	if s.discordWebhookSender != nil {
		if err := s.discordWebhookSender.Close(); err != nil {
			log.WithError(err).Warn("Failed to close Discord webhook sender")
		}
	}
	if s.slackWebhookSender != nil {
		if err := s.slackWebhookSender.Close(); err != nil {
			log.WithError(err).Warn("Failed to close Slack webhook sender")
		}
	}

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

	// Create a timeout context for this check (5 minutes max)
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	log.Debug("Checking API for new ransomware data")

	// Get all ransomware entries from API
	allEntries, err := s.apiClient.GetLatestEntries(checkCtx)
	if err != nil {
		log.WithError(err).Error("Failed to get API data")
		s.statusTracker.UpdateAPIStatus(false, 0, err.Error())
		return
	}

	// Update status with success
	s.statusTracker.UpdateAPIStatus(true, len(allEntries), "")

	// Send entries individually to Discord (filter by what's already sent to this webhook)
	if s.config.DiscordWebhooks.Ransomware.Enabled {
		webhookURL := s.config.DiscordWebhooks.Ransomware.URL
		var unsent []api.RansomwareEntry
		for _, entry := range allEntries {
			key := api.GenerateEntryKey(entry)
			if !s.statusTracker.IsAPIItemSentToWebhook(key, webhookURL) {
				unsent = append(unsent, entry)
			}
		}

		log.WithFields(log.Fields{
			"webhook_enabled": true,
			"unsent_count":    len(unsent),
			"total_fetched":   len(allEntries),
		}).Debug("Checking Discord unsent items")

		if len(unsent) > 0 {
			log.WithFields(log.Fields{
				"total_entries":  len(allEntries),
				"discord_unsent": len(unsent),
			}).Info("Found unsent API entries for Discord")
			s.bulkSendAPIEntriesToDiscord(checkCtx, unsent, webhookURL)
		} else {
			log.Debug("No unsent Discord items found")
		}
	}

	// Send entries individually to Slack (filter by what's already sent to this webhook)
	if s.config.SlackWebhooks.Ransomware.Enabled {
		webhookURL := s.config.SlackWebhooks.Ransomware.URL
		var unsent []api.RansomwareEntry
		for _, entry := range allEntries {
			key := api.GenerateEntryKey(entry)
			if !s.statusTracker.IsAPIItemSentToWebhook(key, webhookURL) {
				unsent = append(unsent, entry)
			}
		}

		log.WithFields(log.Fields{
			"webhook_enabled": true,
			"unsent_count":    len(unsent),
			"total_fetched":   len(allEntries),
		}).Debug("Checking Slack unsent items")

		if len(unsent) > 0 {
			log.WithFields(log.Fields{
				"total_entries": len(allEntries),
				"slack_unsent":  len(unsent),
			}).Info("Found unsent API entries for Slack")
			s.bulkSendAPIEntriesToSlack(checkCtx, unsent, webhookURL)
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
			key := api.GenerateEntryKey(entry)
			title := entry.Group + " -> " + entry.Victim
			s.statusTracker.MarkAPIItemSent(key, title, webhookURL)
			successCount++
		}
	}

	log.WithFields(log.Fields{
		"total_sent":    successCount,
		"total_entries": len(entries),
	}).Info("Individual send to Discord completed")

	// Perform cleanup once after batch processing (more efficient than per-item cleanup)
	if successCount > 0 {
		s.statusTracker.CleanupOldEntries()
	}
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
			key := api.GenerateEntryKey(entry)
			title := entry.Group + " -> " + entry.Victim
			s.statusTracker.MarkAPIItemSent(key, title, webhookURL)
			successCount++
		}
	}

	log.WithFields(log.Fields{
		"total_sent":    successCount,
		"total_entries": len(entries),
	}).Info("Individual send to Slack completed")

	// Perform cleanup once after batch processing (more efficient than per-item cleanup)
	if successCount > 0 {
		s.statusTracker.CleanupOldEntries()
	}
}

// checkRSSOnce performs a single RSS feed check with recovery
func (s *Scheduler) checkRSSOnce(ctx context.Context) {
	// Create a timeout context for this check (10 minutes max for RSS due to multiple feeds)
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	log.Debug("Checking RSS feeds for new entries")

	// Check general feeds for Discord
	if s.config.DiscordWebhooks.RSS.Enabled && len(s.config.Feeds.GeneralFeeds) > 0 {
		s.checkRSSFeeds(checkCtx, s.config.Feeds.GeneralFeeds, s.config.DiscordWebhooks.RSS.URL, "general", "discord")
	}

	// Check government feeds for Discord
	if s.config.DiscordWebhooks.Government.Enabled && len(s.config.Feeds.GovernmentFeeds) > 0 {
		s.checkRSSFeeds(checkCtx, s.config.Feeds.GovernmentFeeds, s.config.DiscordWebhooks.Government.URL, "government", "discord")
	}

	// Check ransomware feeds for Discord
	if s.config.DiscordWebhooks.Ransomware.Enabled && len(s.config.Feeds.RansomwareFeeds) > 0 {
		s.checkRSSFeeds(checkCtx, s.config.Feeds.RansomwareFeeds, s.config.DiscordWebhooks.Ransomware.URL, "ransomware", "discord")
	}

	// Check general feeds for Slack
	if s.config.SlackWebhooks.RSS.Enabled && len(s.config.Feeds.GeneralFeeds) > 0 {
		s.checkRSSFeeds(checkCtx, s.config.Feeds.GeneralFeeds, s.config.SlackWebhooks.RSS.URL, "general", "slack")
	}

	// Check government feeds for Slack
	if s.config.SlackWebhooks.Government.Enabled && len(s.config.Feeds.GovernmentFeeds) > 0 {
		s.checkRSSFeeds(checkCtx, s.config.Feeds.GovernmentFeeds, s.config.SlackWebhooks.Government.URL, "government", "slack")
	}

	// Check ransomware feeds for Slack
	if s.config.SlackWebhooks.Ransomware.Enabled && len(s.config.Feeds.RansomwareFeeds) > 0 {
		s.checkRSSFeeds(checkCtx, s.config.Feeds.RansomwareFeeds, s.config.SlackWebhooks.Ransomware.URL, "ransomware", "slack")
	}

	// Check for unsent RSS items and send them (Recovery Phase)
	s.sendUnsentRSSItems(checkCtx)
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

	// Group by feed type for correct webhook routing using O(1) map lookup
	generalItems := make([]status.StoredRSSEntry, 0, len(unsentRSSItems))
	governmentItems := make([]status.StoredRSSEntry, 0, len(unsentRSSItems))
	ransomwareItems := make([]status.StoredRSSEntry, 0, len(unsentRSSItems))

	// Categorize items by feed URL using map lookup (O(1) per item)
	for _, item := range unsentRSSItems {
		switch s.feedTypeMap[item.FeedURL] {
		case "general":
			generalItems = append(generalItems, item)
		case "government":
			governmentItems = append(governmentItems, item)
		case "ransomware":
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

	// Perform cleanup once after batch processing (more efficient than per-item cleanup)
	if successCount > 0 {
		s.statusTracker.CleanupOldEntries()
	}
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

	// Perform cleanup once after batch processing (more efficient than per-item cleanup)
	if totalNewEntries > 0 {
		s.statusTracker.CleanupOldEntries()
	}
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
