package status

import "time"

// Status represents the complete status tracking information
type Status struct {
	LastUpdated time.Time           `json:"last_updated"`
	Feeds       map[string]FeedInfo `json:"feeds"`
	API         APIInfo             `json:"api"`
}

// FeedInfo contains status information for a single RSS feed
type FeedInfo struct {
	LastCheck    time.Time  `json:"last_check"`
	LastSuccess  *time.Time `json:"last_success"`
	LastError    *string    `json:"last_error"`
	EntriesFound int        `json:"entries_found"`
	SuccessRate  float64    `json:"success_rate"`
	// Removed ProcessedItems - now handled by ParsedItems/SentItems in tracker.go
}

// APIInfo contains status information for the ransomware API
type APIInfo struct {
	LastCheck    time.Time  `json:"last_check"`
	LastSuccess  *time.Time `json:"last_success"`
	LastError    *string    `json:"last_error"`
	EntriesFound int        `json:"entries_found"`
	// Removed ProcessedItems - now handled by FetchedItems/SentItems in tracker.go
}

// NewStatus creates a new empty status structure
func NewStatus() *Status {
	return &Status{
		LastUpdated: time.Now(),
		Feeds:       make(map[string]FeedInfo),
		API:         APIInfo{},
	}
}
