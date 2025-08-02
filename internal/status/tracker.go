package status

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// Tracker handles status tracking and persistence with separate files for API and RSS
type Tracker struct {
	apiStatus *APIStatus
	rssStatus *RSSStatus
	dataDir   string
	mutex     sync.RWMutex
}

// APIStatus represents status for ransomware API with two-phase tracking
type APIStatus struct {
	LastUpdated  time.Time                  `json:"last_updated"`
	LastCheck    time.Time                  `json:"last_check"`
	LastSuccess  *time.Time                 `json:"last_success"`
	LastError    *string                    `json:"last_error"`
	EntriesFound int                        `json:"entries_found"`
	FetchedItems []StoredRansomwareEntry    `json:"fetched_items"` // Changed to slice for sorting
	SentItems    map[string]DiscordSentInfo `json:"sent_items"`    // Items sent to Discord
}

// StoredRansomwareEntry represents a complete ransomware entry for storage
type StoredRansomwareEntry struct {
	Key         string `json:"key"` // Unique key for tracking
	ID          string `json:"id"`
	Group       string `json:"group"`
	Victim      string `json:"victim"`
	Country     string `json:"country"`
	Activity    string `json:"activity"`
	AttackDate  string `json:"attackdate"`
	Discovered  string `json:"discovered"` // Store as string for JSON compatibility
	ClaimURL    string `json:"post_url"`
	URL         string `json:"website"`
	Description string `json:"description"`
	Screenshot  string `json:"screenshot"`
	Published   string `json:"published"` // Store as string for JSON compatibility
}

// DiscordSentInfo contains information about items sent to Discord
type DiscordSentInfo struct {
	Title  string    `json:"title"`
	SentAt time.Time `json:"sent_at"`
}

// RSSStatus represents status for RSS feeds
type RSSStatus struct {
	LastUpdated time.Time           `json:"last_updated"`
	Feeds       map[string]FeedInfo `json:"feeds"`
}

// NewTracker creates a new status tracker instance with separate files
func NewTracker(dataDir string) (*Tracker, error) {
	// Create data directory if it doesn't exist
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	tracker := &Tracker{
		dataDir:   dataDir,
		apiStatus: NewAPIStatus(),
		rssStatus: NewRSSStatus(),
	}

	// Load existing status files if they exist
	if err := tracker.loadAPIStatus(); err != nil {
		log.WithError(err).Warn("Failed to load existing API status, starting fresh")
	}

	if err := tracker.loadRSSStatus(); err != nil {
		log.WithError(err).Warn("Failed to load existing RSS status, starting fresh")
	}

	return tracker, nil
}

// NewAPIStatus creates a new empty API status structure
func NewAPIStatus() *APIStatus {
	return &APIStatus{
		LastUpdated:  time.Now(),
		FetchedItems: make([]StoredRansomwareEntry, 0),
		SentItems:    make(map[string]DiscordSentInfo),
	}
}

// NewRSSStatus creates a new empty RSS status structure
func NewRSSStatus() *RSSStatus {
	return &RSSStatus{
		LastUpdated: time.Now(),
		Feeds:       make(map[string]FeedInfo),
	}
}

// UpdateFeedStatus updates the status for a specific RSS feed
func (t *Tracker) UpdateFeedStatus(feedURL string, success bool, entriesFound int, errorMsg string) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	now := time.Now()

	// Get existing feed info or create new one
	feedInfo, exists := t.rssStatus.Feeds[feedURL]
	if !exists {
		feedInfo = FeedInfo{
			SuccessRate:    1.0, // Start with 100% success rate
			ProcessedItems: make(map[string]string),
		}
	}

	// Update check time
	feedInfo.LastCheck = now
	feedInfo.EntriesFound = entriesFound

	if success {
		// Update success information
		feedInfo.LastSuccess = &now
		feedInfo.LastError = nil

		// Calculate new success rate (simple moving average approach)
		if exists {
			feedInfo.SuccessRate = (feedInfo.SuccessRate * 0.9) + (1.0 * 0.1)
		} else {
			feedInfo.SuccessRate = 1.0
		}
	} else {
		// Update error information
		feedInfo.LastError = &errorMsg

		// Calculate new success rate with failure
		if exists {
			feedInfo.SuccessRate = feedInfo.SuccessRate * 0.9 // Reduce success rate
		} else {
			feedInfo.SuccessRate = 0.0
		}
	}

	// Store updated feed info
	t.rssStatus.Feeds[feedURL] = feedInfo
	t.rssStatus.LastUpdated = now

	// Save RSS status to file
	if err := t.saveRSSStatus(); err != nil {
		log.WithError(err).Error("Failed to save RSS status to file")
	}
}

// UpdateAPIStatus updates the status for the ransomware API
func (t *Tracker) UpdateAPIStatus(success bool, entriesFound int, errorMsg string) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	now := time.Now()

	// Update API check time
	t.apiStatus.LastCheck = now
	t.apiStatus.EntriesFound = entriesFound

	if success {
		// Update success information
		t.apiStatus.LastSuccess = &now
		t.apiStatus.LastError = nil
	} else {
		// Update error information
		t.apiStatus.LastError = &errorMsg
	}

	t.apiStatus.LastUpdated = now

	// Save API status to file
	if err := t.saveAPIStatus(); err != nil {
		log.WithError(err).Error("Failed to save API status to file")
	}
}

// IsItemProcessed checks if an RSS item was already processed
func (t *Tracker) IsItemProcessed(feedURL, itemKey string) bool {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	feedInfo, exists := t.rssStatus.Feeds[feedURL]
	if !exists {
		return false
	}

	_, processed := feedInfo.ProcessedItems[itemKey]
	return processed
}

// MarkItemProcessed marks an RSS item as processed
func (t *Tracker) MarkItemProcessed(feedURL, itemKey, itemTitle string) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	// Get or create feed info
	feedInfo, exists := t.rssStatus.Feeds[feedURL]
	if !exists {
		feedInfo = FeedInfo{
			SuccessRate:    1.0,
			ProcessedItems: make(map[string]string),
		}
	}

	// Ensure ProcessedItems map exists
	if feedInfo.ProcessedItems == nil {
		feedInfo.ProcessedItems = make(map[string]string)
	}

	// Mark item as processed
	feedInfo.ProcessedItems[itemKey] = itemTitle

	// Store updated feed info
	t.rssStatus.Feeds[feedURL] = feedInfo
	t.rssStatus.LastUpdated = time.Now()

	// Clean up old entries to prevent status file from growing too large
	t.cleanupOldRSSProcessedItems(feedURL)

	// Save RSS status to file
	if err := t.saveRSSStatus(); err != nil {
		log.WithError(err).Error("Failed to save RSS status to file")
	}
}

// IsAPIItemFetched checks if an API item was already fetched from the API
func (t *Tracker) IsAPIItemFetched(itemKey string) bool {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	// Search through the slice for the key
	for _, entry := range t.apiStatus.FetchedItems {
		if entry.Key == itemKey {
			return true
		}
	}
	return false
}

// MarkAPIItemFetched marks an API item as fetched from the API with complete entry data
func (t *Tracker) MarkAPIItemFetched(itemKey string, entry StoredRansomwareEntry) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	// Ensure FetchedItems slice exists
	if t.apiStatus.FetchedItems == nil {
		t.apiStatus.FetchedItems = make([]StoredRansomwareEntry, 0)
	}

	// Check if entry already exists (avoid duplicates)
	for i, existing := range t.apiStatus.FetchedItems {
		if existing.Key == itemKey {
			// Update existing entry
			entry.Key = itemKey
			t.apiStatus.FetchedItems[i] = entry
			t.sortFetchedItems()
			t.apiStatus.LastUpdated = time.Now()

			// Save API status to file
			if err := t.saveAPIStatus(); err != nil {
				log.WithError(err).Error("Failed to save API status to file")
			}
			return
		}
	}

	// Add new entry with key
	entry.Key = itemKey
	t.apiStatus.FetchedItems = append(t.apiStatus.FetchedItems, entry)

	// Sort by discovered time (oldest first) to maintain chronological order
	t.sortFetchedItems()

	t.apiStatus.LastUpdated = time.Now()

	// Clean up old entries
	t.cleanupOldAPIFetchedItems()

	// Save API status to file
	if err := t.saveAPIStatus(); err != nil {
		log.WithError(err).Error("Failed to save API status to file")
	}
}

// sortFetchedItems sorts the fetched items by discovered time (oldest first)
func (t *Tracker) sortFetchedItems() {
	sort.Slice(t.apiStatus.FetchedItems, func(i, j int) bool {
		// Parse discovered times for comparison
		timeI, errI := time.Parse("2006-01-02 15:04:05.999999", t.apiStatus.FetchedItems[i].Discovered)
		if errI != nil {
			timeI, _ = time.Parse("2006-01-02 15:04:05", t.apiStatus.FetchedItems[i].Discovered)
		}

		timeJ, errJ := time.Parse("2006-01-02 15:04:05.999999", t.apiStatus.FetchedItems[j].Discovered)
		if errJ != nil {
			timeJ, _ = time.Parse("2006-01-02 15:04:05", t.apiStatus.FetchedItems[j].Discovered)
		}

		return timeI.Before(timeJ)
	})
}

// IsAPIItemSent checks if an API item was already sent to Discord
func (t *Tracker) IsAPIItemSent(itemKey string) bool {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	_, sent := t.apiStatus.SentItems[itemKey]
	return sent
}

// MarkAPIItemSent marks an API item as sent to Discord
func (t *Tracker) MarkAPIItemSent(itemKey, itemTitle, webhookURL string) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	// Ensure SentItems map exists
	if t.apiStatus.SentItems == nil {
		t.apiStatus.SentItems = make(map[string]DiscordSentInfo)
	}

	// Mark item as sent (without storing webhook URL for security)
	t.apiStatus.SentItems[itemKey] = DiscordSentInfo{
		Title:  itemTitle,
		SentAt: time.Now(),
	}
	t.apiStatus.LastUpdated = time.Now()

	// Clean up old entries
	t.cleanupOldAPISentItems()

	// Save API status to file
	if err := t.saveAPIStatus(); err != nil {
		log.WithError(err).Error("Failed to save API status to file")
	}
}

// IsAPIItemProcessed checks if an API item was already processed (for backward compatibility)
func (t *Tracker) IsAPIItemProcessed(itemKey string) bool {
	return t.IsAPIItemFetched(itemKey)
}

// MarkAPIItemProcessed marks an API item as processed (for backward compatibility)
func (t *Tracker) MarkAPIItemProcessed(itemKey string, entry interface{}) {
	// Convert interface{} to StoredRansomwareEntry for backward compatibility
	if storedEntry, ok := entry.(StoredRansomwareEntry); ok {
		t.MarkAPIItemFetched(itemKey, storedEntry)
	} else {
		log.WithField("item_key", itemKey).Warn("Invalid entry type for MarkAPIItemProcessed")
	}
}

// GetUnsentAPIItems returns a map of complete API entries that were fetched but not yet sent (DEPRECATED)
func (t *Tracker) GetUnsentAPIItems() map[string]StoredRansomwareEntry {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	unsent := make(map[string]StoredRansomwareEntry)

	// Check each fetched item to see if it was sent
	for _, entry := range t.apiStatus.FetchedItems {
		if _, sent := t.apiStatus.SentItems[entry.Key]; !sent {
			unsent[entry.Key] = entry
		}
	}

	return unsent
}

// GetUnsentAPIItemsSorted returns a sorted slice of complete API entries that were fetched but not yet sent
func (t *Tracker) GetUnsentAPIItemsSorted() []StoredRansomwareEntry {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	var unsent []StoredRansomwareEntry

	// Check each fetched item to see if it was sent (items are already sorted by discovered time)
	for _, entry := range t.apiStatus.FetchedItems {
		if _, sent := t.apiStatus.SentItems[entry.Key]; !sent {
			unsent = append(unsent, entry)
		}
	}

	log.WithField("unsent_count", len(unsent)).Debug("Retrieved unsent API items in chronological order")
	return unsent
}

// cleanupOldRSSProcessedItems removes old processed items to keep RSS status file manageable
func (t *Tracker) cleanupOldRSSProcessedItems(feedURL string) {
	const maxProcessedItems = 500 // Keep max 500 processed items per feed

	feedInfo := t.rssStatus.Feeds[feedURL]
	if len(feedInfo.ProcessedItems) > maxProcessedItems {
		// Keep only the most recent items (simple approach: remove randomly)
		newMap := make(map[string]string)
		count := 0
		for k, v := range feedInfo.ProcessedItems {
			if count >= maxProcessedItems {
				break
			}
			newMap[k] = v
			count++
		}
		feedInfo.ProcessedItems = newMap
		t.rssStatus.Feeds[feedURL] = feedInfo

		log.WithField("feed_url", feedURL).Debug("Cleaned up old RSS processed items")
	}
}

// cleanupOldAPIFetchedItems removes old fetched items
func (t *Tracker) cleanupOldAPIFetchedItems() {
	const maxFetchedItems = 1000 // Keep max 1000 fetched API items

	if len(t.apiStatus.FetchedItems) > maxFetchedItems {
		// Keep only the most recent items (slice is already sorted by time)
		t.apiStatus.FetchedItems = t.apiStatus.FetchedItems[len(t.apiStatus.FetchedItems)-maxFetchedItems:]
		log.Debug("Cleaned up old API fetched items")
	}
}

// cleanupOldAPISentItems removes old sent items
func (t *Tracker) cleanupOldAPISentItems() {
	const maxSentItems = 1000 // Keep max 1000 sent API items

	if len(t.apiStatus.SentItems) > maxSentItems {
		newMap := make(map[string]DiscordSentInfo)
		count := 0
		for k, v := range t.apiStatus.SentItems {
			if count >= maxSentItems {
				break
			}
			newMap[k] = v
			count++
		}
		t.apiStatus.SentItems = newMap

		log.Debug("Cleaned up old API sent items")
	}
}

// GetFeedStatus returns status for a specific feed
func (t *Tracker) GetFeedStatus(feedURL string) (FeedInfo, bool) {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	feedInfo, exists := t.rssStatus.Feeds[feedURL]
	return feedInfo, exists
}

// GetAPIStatus returns the current API status
func (t *Tracker) GetAPIStatus() APIStatus {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	return *t.apiStatus
}

// saveAPIStatus saves the API status to JSON file
func (t *Tracker) saveAPIStatus() error {
	filePath := filepath.Join(t.dataDir, "api_status.json")

	data, err := json.MarshalIndent(t.apiStatus, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal API status: %w", err)
	}

	// Write to temporary file first, then rename for atomic update
	tempFile := filePath + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temporary API status file: %w", err)
	}

	if err := os.Rename(tempFile, filePath); err != nil {
		return fmt.Errorf("failed to rename API status file: %w", err)
	}

	log.WithField("file", filePath).Debug("API status saved to file with sorted entries")
	return nil
}

// saveRSSStatus saves the RSS status to JSON file
func (t *Tracker) saveRSSStatus() error {
	filePath := filepath.Join(t.dataDir, "rss_status.json")

	data, err := json.MarshalIndent(t.rssStatus, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal RSS status: %w", err)
	}

	// Write to temporary file first, then rename for atomic update
	tempFile := filePath + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temporary RSS status file: %w", err)
	}

	if err := os.Rename(tempFile, filePath); err != nil {
		return fmt.Errorf("failed to rename RSS status file: %w", err)
	}

	log.WithField("file", filePath).Debug("RSS status saved to file")
	return nil
}

// loadAPIStatus loads API status from JSON file if it exists
func (t *Tracker) loadAPIStatus() error {
	filePath := filepath.Join(t.dataDir, "api_status.json")

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		log.WithField("file", filePath).Info("API status file does not exist, starting with empty status")
		return nil
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read API status file: %w", err)
	}

	var loadedStatus APIStatus
	if err := json.Unmarshal(data, &loadedStatus); err != nil {
		return fmt.Errorf("failed to unmarshal API status: %w", err)
	}

	// Ensure data structures exist
	if loadedStatus.FetchedItems == nil {
		loadedStatus.FetchedItems = make([]StoredRansomwareEntry, 0)
	}
	if loadedStatus.SentItems == nil {
		loadedStatus.SentItems = make(map[string]DiscordSentInfo)
	}

	// Migrate old map-based storage to new slice-based storage if needed
	t.migrateOldFormat(&loadedStatus)

	// Sort fetched items by discovered time to ensure chronological order
	t.apiStatus = &loadedStatus
	t.sortFetchedItems()

	// Count unsent items
	unsentCount := 0
	for _, entry := range t.apiStatus.FetchedItems {
		if _, sent := t.apiStatus.SentItems[entry.Key]; !sent {
			unsentCount++
		}
	}

	log.WithFields(log.Fields{
		"file":          filePath,
		"last_updated":  t.apiStatus.LastUpdated,
		"fetched_items": len(t.apiStatus.FetchedItems),
		"sent_items":    len(t.apiStatus.SentItems),
		"unsent_items":  unsentCount,
	}).Info("API status loaded from file in chronological order")

	return nil
}

// migrateOldFormat migrates old map-based storage to new slice-based storage
func (t *Tracker) migrateOldFormat(status *APIStatus) {
	// This handles migration from old map[string]StoredRansomwareEntry format
	// to new []StoredRansomwareEntry format if the JSON contains the old format

	// If we have an empty slice but the old format might be in the JSON,
	// we'll just ensure the slice exists and is properly initialized
	if status.FetchedItems == nil {
		status.FetchedItems = make([]StoredRansomwareEntry, 0)
	}

	// Ensure all entries have keys
	for i, entry := range status.FetchedItems {
		if entry.Key == "" {
			// Generate key from entry data
			entry.Key = t.generateKeyFromEntry(entry)
			status.FetchedItems[i] = entry
		}
	}
}

// generateKeyFromEntry generates a unique key for an entry (used in migration)
func (t *Tracker) generateKeyFromEntry(entry StoredRansomwareEntry) string {
	if entry.ID != "" {
		return "id:" + entry.ID
	}
	return entry.Group + ":" + entry.Victim + ":" + entry.Discovered
}

// loadRSSStatus loads RSS status from JSON file if it exists
func (t *Tracker) loadRSSStatus() error {
	filePath := filepath.Join(t.dataDir, "rss_status.json")

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		log.WithField("file", filePath).Info("RSS status file does not exist, starting with empty status")
		return nil
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read RSS status file: %w", err)
	}

	var loadedStatus RSSStatus
	if err := json.Unmarshal(data, &loadedStatus); err != nil {
		return fmt.Errorf("failed to unmarshal RSS status: %w", err)
	}

	// Ensure all maps are initialized
	if loadedStatus.Feeds == nil {
		loadedStatus.Feeds = make(map[string]FeedInfo)
	}

	// Ensure ProcessedItems maps exist for all feeds
	for feedURL, feedInfo := range loadedStatus.Feeds {
		if feedInfo.ProcessedItems == nil {
			feedInfo.ProcessedItems = make(map[string]string)
			loadedStatus.Feeds[feedURL] = feedInfo
		}
	}

	t.rssStatus = &loadedStatus

	// Count total processed items for info
	totalProcessed := 0
	for _, feedInfo := range t.rssStatus.Feeds {
		totalProcessed += len(feedInfo.ProcessedItems)
	}

	log.WithFields(log.Fields{
		"file":            filePath,
		"feeds":           len(t.rssStatus.Feeds),
		"last_updated":    t.rssStatus.LastUpdated,
		"processed_items": totalProcessed,
	}).Info("RSS status loaded from file")

	return nil
}

// PrintSummary logs a summary of the current status
func (t *Tracker) PrintSummary() {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	// Count total processed items
	totalFetched := len(t.apiStatus.FetchedItems)
	totalSent := len(t.apiStatus.SentItems)
	totalUnsent := 0

	for _, entry := range t.apiStatus.FetchedItems {
		if _, sent := t.apiStatus.SentItems[entry.Key]; !sent {
			totalUnsent++
		}
	}

	totalRSSProcessed := 0
	for _, feedInfo := range t.rssStatus.Feeds {
		totalRSSProcessed += len(feedInfo.ProcessedItems)
	}

	log.WithFields(log.Fields{
		"total_feeds":         len(t.rssStatus.Feeds),
		"api_last_updated":    t.apiStatus.LastUpdated,
		"rss_last_updated":    t.rssStatus.LastUpdated,
		"api_fetched_items":   totalFetched,
		"api_sent_items":      totalSent,
		"api_unsent_items":    totalUnsent,
		"rss_processed_items": totalRSSProcessed,
	}).Info("Status summary")

	// Log API status
	if t.apiStatus.LastSuccess != nil {
		log.WithFields(log.Fields{
			"last_success":  *t.apiStatus.LastSuccess,
			"entries_found": t.apiStatus.EntriesFound,
			"fetched_items": len(t.apiStatus.FetchedItems),
			"sent_items":    len(t.apiStatus.SentItems),
			"unsent_items":  totalUnsent,
		}).Info("API status details")
	}

	// Log feed statuses
	for feedURL, feedInfo := range t.rssStatus.Feeds {
		fields := log.Fields{
			"feed_url":        feedURL,
			"success_rate":    fmt.Sprintf("%.1f%%", feedInfo.SuccessRate*100),
			"last_check":      feedInfo.LastCheck,
			"processed_items": len(feedInfo.ProcessedItems),
		}

		if feedInfo.LastSuccess != nil {
			fields["last_success"] = *feedInfo.LastSuccess
		}

		if feedInfo.LastError != nil {
			fields["last_error"] = *feedInfo.LastError
		}

		log.WithFields(fields).Debug("Feed status")
	}
}
