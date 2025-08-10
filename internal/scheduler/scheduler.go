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
	"Ransomware-Bot/internal/status"

	log "github.com/sirupsen/logrus"
)

// Scheduler manages the execution of API polling and RSS feed checking
type Scheduler struct {
	config        *config.Config
	apiClient     *api.Client
	rssParser     *rss.Parser
	webhookSender *discord.WebhookSender
	statusTracker *status.Tracker

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

	// Initialize webhook sender
	webhookSender := discord.NewWebhookSender()

	return &Scheduler{
		config:        cfg,
		apiClient:     apiClient,
		rssParser:     rssParser,
		webhookSender: webhookSender,
		statusTracker: statusTracker,
		stopChan:      make(chan struct{}),
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

	// Get ALL entries that were fetched but not yet sent to Discord (already sorted by storage)
	allUnsentEntries := s.getAllUnsentAPIEntries()

	if len(allUnsentEntries) == 0 {
		log.Debug("No unsent API entries found")
		return
	}

	log.WithFields(log.Fields{
		"new_entries":  len(newEntries),
		"total_unsent": len(allUnsentEntries),
	}).Info("Found unsent API entries")

	// Send entries individually to Discord (already in correct chronological order)
	if s.config.Webhooks.Ransomware.Enabled {
		s.bulkSendAPIEntriesToDiscord(allUnsentEntries, s.config.Webhooks.Ransomware.URL)
	}
}

// getAllUnsentAPIEntries returns ALL entries that were fetched but not yet sent to Discord
// Data is already sorted by discovered time in storage, so no additional sorting needed
func (s *Scheduler) getAllUnsentAPIEntries() []api.RansomwareEntry {
	// Get all unsent items from status tracker (stored in chronological order)
	unsentList := s.statusTracker.GetUnsentAPIItemsSorted()

	var unsentEntries []api.RansomwareEntry

	// Convert StoredRansomwareEntry back to api.RansomwareEntry
	for _, storedEntry := range unsentList {
		entry := s.convertStoredToAPIEntry(storedEntry)
		unsentEntries = append(unsentEntries, entry)
	}

	log.WithField("unsent_count", len(unsentEntries)).Debug("Retrieved all unsent API entries in chronological order")
	return unsentEntries
}

// convertStoredToAPIEntry converts a StoredRansomwareEntry back to api.RansomwareEntry
func (s *Scheduler) convertStoredToAPIEntry(stored status.StoredRansomwareEntry) api.RansomwareEntry {
	// Convert stored entry back to API entry format
	// Note: We need to parse the time strings back to CustomTime
	discovered, err := time.Parse("2006-01-02 15:04:05.999999", stored.Discovered)
	if err != nil {
		// Fallback to simpler format if parsing fails
		discovered, _ = time.Parse("2006-01-02 15:04:05", stored.Discovered)
	}

	published, err := time.Parse("2006-01-02 15:04:05.999999", stored.Published)
	if err != nil {
		// Fallback to simpler format if parsing fails
		published, _ = time.Parse("2006-01-02 15:04:05", stored.Published)
	}

	return api.RansomwareEntry{
		ID:          stored.ID,
		Group:       stored.Group,
		Victim:      stored.Victim,
		Country:     stored.Country,
		Activity:    stored.Activity,
		AttackDate:  stored.AttackDate,
		Discovered:  api.CustomTime{Time: discovered},
		ClaimURL:    stored.ClaimURL,
		URL:         stored.URL,
		Description: stored.Description,
		Screenshot:  stored.Screenshot,
		Published:   api.CustomTime{Time: published},
	}
}

// bulkSendAPIEntriesToDiscord sends API entries to Discord individually with rate limiting
func (s *Scheduler) bulkSendAPIEntriesToDiscord(entries []api.RansomwareEntry, webhookURL string) {
	if len(entries) == 0 {
		log.Debug("No entries to send")
		return
	}

	log.WithField("total_entries", len(entries)).Info("Starting individual send to Discord")

	// Send each entry individually with configurable delay
	successCount := 0
	for _, entry := range entries {
		// Configurable delay before sending to Discord
		time.Sleep(s.config.DiscordDelay)
		// Send the entry to Discord webhook
		if err := s.webhookSender.SendRansomwareEntry(webhookURL, entry, &s.config.Format); err != nil {
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

// generateAPIEntryKey creates a unique key for an API entry (same logic as in api package)
func (s *Scheduler) generateAPIEntryKey(entry api.RansomwareEntry) string {
	// Use a combination of fields that should be unique per entry
	if entry.ID != "" {
		return "id:" + entry.ID
	}

	// Fallback to combination of group, victim, and discovery time
	return entry.Group + ":" + entry.Victim + ":" + entry.Discovered.Time.Format(time.RFC3339)
}

// checkRSSOnce performs a single RSS feed check with recovery
func (s *Scheduler) checkRSSOnce(ctx context.Context) {
	log.Debug("Checking RSS feeds for new entries")

	// Check general feeds
	if s.config.Webhooks.RSS.Enabled && len(s.config.Feeds.GeneralFeeds) > 0 {
		s.checkRSSFeeds(ctx, s.config.Feeds.GeneralFeeds, s.config.Webhooks.RSS.URL, "general")
	}

	// Check government feeds
	if s.config.Webhooks.Government.Enabled && len(s.config.Feeds.GovernmentFeeds) > 0 {
		s.checkRSSFeeds(ctx, s.config.Feeds.GovernmentFeeds, s.config.Webhooks.Government.URL, "government")
	}

	// Check ransomware feeds
	if s.config.Webhooks.Ransomware.Enabled && len(s.config.Feeds.RansomwareFeeds) > 0 {
		s.checkRSSFeeds(ctx, s.config.Feeds.RansomwareFeeds, s.config.Webhooks.Ransomware.URL, "ransomware")
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

	log.WithField("unsent_count", len(unsentRSSItems)).Info("Found unsent RSS items, sending to Discord")

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

	// Send to appropriate webhooks
	if len(generalItems) > 0 && s.config.Webhooks.RSS.Enabled {
		s.sendRSSItemsToDiscord(generalItems, s.config.Webhooks.RSS.URL, "general")
	}

	if len(governmentItems) > 0 && s.config.Webhooks.Government.Enabled {
		s.sendRSSItemsToDiscord(governmentItems, s.config.Webhooks.Government.URL, "government")
	}

	if len(ransomwareItems) > 0 && s.config.Webhooks.Ransomware.Enabled {
		s.sendRSSItemsToDiscord(ransomwareItems, s.config.Webhooks.Ransomware.URL, "ransomware")
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

// sendRSSItemsToDiscord sends RSS items to Discord with Phase 2 tracking
func (s *Scheduler) sendRSSItemsToDiscord(items []status.StoredRSSEntry, webhookURL string, feedType string) {
	if len(items) == 0 {
		log.Debug("No RSS items to send")
		return
	}

	log.WithFields(log.Fields{
		"total_items": len(items),
		"feed_type":   feedType,
	}).Info("Starting RSS items send to Discord")

	successCount := 0
	for _, storedItem := range items {
		// Convert stored RSS entry back to rss.Entry for sending
		rssEntry := s.convertStoredToRSSEntry(storedItem)

		// Configurable delay before sending to Discord
		time.Sleep(s.config.DiscordDelay)

		if err := s.webhookSender.SendRSSEntry(webhookURL, rssEntry); err != nil {
			log.WithError(err).WithFields(log.Fields{
				"title":    storedItem.Title,
				"feed_url": storedItem.FeedURL,
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
	}).Info("RSS items send to Discord completed")
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
func (s *Scheduler) checkRSSFeeds(ctx context.Context, feedURLs []string, webhookURL string, feedType string) {
	if len(feedURLs) == 0 {
		log.WithField("feed_type", feedType).Debug("No RSS feeds configured")
		return
	}

	log.WithFields(log.Fields{
		"feed_count":  len(feedURLs),
		"feed_type":   feedType,
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
			// Configurable delay before sending to Discord
			time.Sleep(s.config.DiscordDelay)

			if err := s.webhookSender.SendRSSEntry(webhookURL, entry); err != nil {
				log.WithError(err).WithField("feed_url", feedURL).Error("Failed to send RSS entry to webhook")
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
	dateStr := entry.Published.Format(time.RFC3339)
	return feedURL + ":" + entry.Title + ":" + dateStr
}
