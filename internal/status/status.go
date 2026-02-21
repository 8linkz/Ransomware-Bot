package status

import "time"

// FeedInfo contains status information for a single RSS feed
type FeedInfo struct {
	LastCheck    time.Time  `json:"last_check"`
	LastSuccess  *time.Time `json:"last_success"`
	LastError    *string    `json:"last_error"`
	EntriesFound int        `json:"entries_found"`
	SuccessRate  float64    `json:"success_rate"`
}
