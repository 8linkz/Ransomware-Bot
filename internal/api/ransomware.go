package api

import (
	"context"
	"strings"
	"time"

	"Ransomware-Bot/internal/status"

	log "github.com/sirupsen/logrus"
)

// CustomTime handles the API's time format
type CustomTime struct {
	time.Time
}

// UnmarshalJSON parses the API's time format
func (ct *CustomTime) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), "\"")
	if s == "null" || s == "" {
		ct.Time = time.Time{}
		return nil
	}

	// Parse the API format: "2025-08-02 12:52:04.158280"
	t, err := time.Parse("2006-01-02 15:04:05.999999", s)
	if err != nil {
		// Fallback to format without microseconds: "2025-08-02 12:52:04"
		t, err = time.Parse("2006-01-02 15:04:05", s)
		if err != nil {
			return err
		}
	}
	ct.Time = t
	return nil
}

const (
	// RansomwareLiveAPI is the base URL for the ransomware.live API
	RansomwareLiveAPI = "https://api-pro.ransomware.live"
)

// RansomwareEntry represents a single ransomware incident from the API
type RansomwareEntry struct {
	ID          string     `json:"id"`
	Group       string     `json:"group"`
	Victim      string     `json:"victim"`
	Country     string     `json:"country"`
	Activity    string     `json:"activity"`
	AttackDate  string     `json:"attackdate"`
	Discovered  CustomTime `json:"discovered"`
	ClaimURL    string     `json:"post_url"` // Maps to post_url from API
	URL         string     `json:"website"`  // Maps to website from API
	Description string     `json:"description"`
	Screenshot  string     `json:"screenshot"`
	Published   CustomTime `json:"published"`
}

// RansomwareResponse represents the API response structure
type RansomwareResponse struct {
	Client  string            `json:"client"`
	Count   int               `json:"count"`
	Order   string            `json:"order"`
	Victims []RansomwareEntry `json:"victims"`
}

// StatusTracker interface for API deduplication with two-phase tracking
type StatusTracker interface {
	IsAPIItemFetched(itemKey string) bool
	MarkAPIItemFetched(itemKey string, entry status.StoredRansomwareEntry)
	IsAPIItemSent(itemKey string) bool
	MarkAPIItemSent(itemKey, itemTitle, webhookURL string)
}

// GetLatestEntries fetches the latest ransomware entries from the API
func (c *Client) GetLatestEntries(ctx context.Context, statusTracker StatusTracker) ([]RansomwareEntry, error) {
	start := time.Now()
	url := RansomwareLiveAPI + "/victims/recent"

	log.WithField("url", url).Debug("Fetching latest ransomware entries")

	var response RansomwareResponse
	err := c.makeRequest(ctx, url, &response)

	// Log the request
	c.logRequest(url, time.Since(start), err)

	if err != nil {
		return nil, err
	}

	// Filter out already processed entries using persistent storage
	newEntries := filterNewEntries(response.Victims, statusTracker)

	log.WithFields(log.Fields{
		"total_entries": len(response.Victims),
		"new_entries":   len(newEntries),
	}).Info("Processed ransomware API response")

	return newEntries, nil
}

// GetRecentByGroup fetches recent entries for a specific ransomware group
func (c *Client) GetRecentByGroup(ctx context.Context, groupName string, statusTracker StatusTracker) ([]RansomwareEntry, error) {
	start := time.Now()
	url := RansomwareLiveAPI + "/group/" + groupName

	log.WithFields(log.Fields{
		"url":   url,
		"group": groupName,
	}).Debug("Fetching entries for ransomware group")

	var response RansomwareResponse
	err := c.makeRequest(ctx, url, &response)

	// Log the request
	c.logRequest(url, time.Since(start), err)

	if err != nil {
		return nil, err
	}

	// Filter out already processed entries
	newEntries := filterNewEntries(response.Victims, statusTracker)

	log.WithFields(log.Fields{
		"group":         groupName,
		"total_entries": len(response.Victims),
		"new_entries":   len(newEntries),
	}).Info("Processed group-specific ransomware API response")

	return newEntries, nil
}

// filterNewEntries filters out entries that have already been fetched from API
// Uses persistent storage via StatusTracker for two-phase tracking
func filterNewEntries(entries []RansomwareEntry, statusTracker StatusTracker) []RansomwareEntry {
	var newEntries []RansomwareEntry

	for _, entry := range entries {
		// Create a unique key for this entry
		key := generateEntryKey(entry)

		// Check if we've already fetched this entry from API using persistent storage
		if statusTracker != nil && statusTracker.IsAPIItemFetched(key) {
			continue
		}

		// Mark as fetched from API in persistent storage
		if statusTracker != nil {
			storedEntry := convertToStoredEntry(entry)
			statusTracker.MarkAPIItemFetched(key, storedEntry)
		}

		newEntries = append(newEntries, entry)
	}

	return newEntries
}

// convertToStoredEntry converts api.RansomwareEntry to status.StoredRansomwareEntry for persistence
func convertToStoredEntry(entry RansomwareEntry) status.StoredRansomwareEntry {
	// Convert time fields to strings for JSON storage
	discoveredStr := ""
	if !entry.Discovered.Time.IsZero() {
		discoveredStr = entry.Discovered.Time.Format("2006-01-02 15:04:05.999999")
	}

	publishedStr := ""
	if !entry.Published.Time.IsZero() {
		publishedStr = entry.Published.Time.Format("2006-01-02 15:04:05.999999")
	}

	return status.StoredRansomwareEntry{
		ID:          entry.ID,
		Group:       entry.Group,
		Victim:      entry.Victim,
		Country:     entry.Country,
		Activity:    entry.Activity,
		AttackDate:  entry.AttackDate,
		Discovered:  discoveredStr,
		ClaimURL:    entry.ClaimURL,
		URL:         entry.URL,
		Description: entry.Description,
		Screenshot:  entry.Screenshot,
		Published:   publishedStr,
	}
}

// generateEntryKey creates a unique key for an entry to use in deduplication
func generateEntryKey(entry RansomwareEntry) string {
	// Use a combination of fields that should be unique per entry
	if entry.ID != "" {
		return "id:" + entry.ID
	}

	// Fallback to combination of group, victim, and discovery time
	return entry.Group + ":" + entry.Victim + ":" + entry.Discovered.Time.Format(time.RFC3339)
}
