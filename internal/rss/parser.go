package rss

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/mmcdole/gofeed"
	log "github.com/sirupsen/logrus"
)

// Parser handles RSS feed parsing with retry logic and deduplication
type Parser struct {
	parser        *gofeed.Parser
	retryCount    int
	retryDelay    time.Duration
	statusTracker StatusTracker // Interface for persistent deduplication
}

// StatusTracker interface for deduplication
type StatusTracker interface {
	IsItemProcessed(feedURL, itemKey string) bool
	MarkItemProcessed(feedURL, itemKey, itemTitle string)
}

// Entry represents a processed RSS feed entry
type Entry struct {
	Title       string    `json:"title"`
	Link        string    `json:"link"`
	Description string    `json:"description"`
	Published   time.Time `json:"published"`
	Author      string    `json:"author"`
	Categories  []string  `json:"categories"`
	GUID        string    `json:"guid"`
	FeedTitle   string    `json:"feed_title"`
	FeedURL     string    `json:"feed_url"`
}

// NewParser creates a new RSS parser with retry configuration
func NewParser(retryCount int, retryDelay time.Duration, statusTracker StatusTracker) *Parser {
	return &Parser{
		parser:        gofeed.NewParser(),
		retryCount:    retryCount,
		retryDelay:    retryDelay,
		statusTracker: statusTracker,
	}
}

// ParseFeed parses an RSS feed and returns new entries
func (p *Parser) ParseFeed(ctx context.Context, feedURL string) ([]Entry, error) {
	var feed *gofeed.Feed
	var err error

	// Retry logic for feed parsing
	for attempt := 0; attempt <= p.retryCount; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		log.WithFields(log.Fields{
			"feed_url":     feedURL,
			"attempt":      attempt + 1,
			"max_attempts": p.retryCount + 1,
		}).Debug("Attempting to parse RSS feed")

		feed, err = p.parser.ParseURL(feedURL)
		if err == nil {
			break
		}

		// Log the error and wait before retrying (unless it's the last attempt)
		if attempt < p.retryCount {
			log.WithFields(log.Fields{
				"feed_url": feedURL,
				"attempt":  attempt + 1,
				"error":    err.Error(),
			}).Warn("RSS feed parsing failed, retrying...")

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(p.retryDelay):
				// Continue to next attempt
			}
		}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to parse RSS feed after %d attempts: %w", p.retryCount+1, err)
	}

	// Convert feed items to our Entry format and filter for new items
	entries := p.processFeedItems(feed, feedURL)

	log.WithFields(log.Fields{
		"feed_url":    feedURL,
		"feed_title":  feed.Title,
		"total_items": len(feed.Items),
		"new_entries": len(entries),
	}).Info("Successfully parsed RSS feed")

	return entries, nil
}

// processFeedItems converts gofeed items to our Entry format and filters for new items
func (p *Parser) processFeedItems(feed *gofeed.Feed, feedURL string) []Entry {
	var newEntries []Entry

	for _, item := range feed.Items {
		// Skip nil items (safety check)
		if item == nil {
			continue
		}

		// Generate unique key for this item
		key := p.generateItemKey(feedURL, item)

		// Skip if we've already processed this item (using persistent storage)
		if p.statusTracker != nil && p.statusTracker.IsItemProcessed(feedURL, key) {
			continue
		}

		// Convert to our Entry format with safe nil checks
		entry := Entry{
			Title:       safeString(item.Title),
			Link:        safeString(item.Link),
			Description: safeString(item.Description),
			Author:      getAuthorName(item),
			Categories:  safeCategories(item.Categories),
			GUID:        safeString(item.GUID),
			FeedTitle:   safeString(feed.Title),
			FeedURL:     feedURL,
		}

		// Parse publication date safely
		entry.Published = getPublishedDate(item)

		// Mark as processed in persistent storage
		if p.statusTracker != nil {
			p.statusTracker.MarkItemProcessed(feedURL, key, entry.Title)
		}

		newEntries = append(newEntries, entry)
	}

	return newEntries
}

// generateItemKey creates a unique key for an RSS item
func (p *Parser) generateItemKey(feedURL string, item *gofeed.Item) string {
	if item == nil {
		return feedURL + ":nil-item"
	}

	// Prefer GUID if available
	if item.GUID != "" {
		return feedURL + ":" + item.GUID
	}

	// Fallback to link if available
	if item.Link != "" {
		return feedURL + ":" + item.Link
	}

	// Fallback to title + publication date
	dateStr := ""
	if item.PublishedParsed != nil {
		dateStr = item.PublishedParsed.Format(time.RFC3339)
	} else if item.UpdatedParsed != nil {
		dateStr = item.UpdatedParsed.Format(time.RFC3339)
	}

	title := ""
	if item.Title != "" {
		title = item.Title
	}

	return feedURL + ":" + title + ":" + dateStr
}

// getAuthorName safely extracts author name from RSS item
func getAuthorName(item *gofeed.Item) string {
	if item == nil {
		return ""
	}

	// Try primary author field
	if item.Author != nil && item.Author.Name != "" {
		return item.Author.Name
	}

	// Try authors array
	if len(item.Authors) > 0 && item.Authors[0] != nil && item.Authors[0].Name != "" {
		return item.Authors[0].Name
	}

	// Return empty string if no author found
	return ""
}

// getPublishedDate safely extracts publication date from RSS item
func getPublishedDate(item *gofeed.Item) time.Time {
	if item == nil {
		return time.Now()
	}

	// Prefer parsed published date
	if item.PublishedParsed != nil {
		return *item.PublishedParsed
	}

	// Fallback to updated date
	if item.UpdatedParsed != nil {
		return *item.UpdatedParsed
	}

	// Fallback to current time if no date is available
	return time.Now()
}

// safeString returns empty string if input is nil or empty
func safeString(s string) string {
	return s // strings are safe by default in Go
}

// safeCategories safely extracts categories from RSS item
func safeCategories(categories []string) []string {
	if categories == nil {
		return []string{}
	}
	return categories
}

// ClearCache clears the processed items cache (now delegates to status tracker)
func (p *Parser) ClearCache() {
	log.Info("Clear cache requested - processed items are now managed by status tracker")
}

// GetCacheSize returns the current size of the processed items cache
func (p *Parser) GetCacheSize() int {
	log.Info("Cache size requested - processed items are now managed by status tracker")
	return 0 // Not applicable anymore
}

// ParseMultipleFeeds parses multiple RSS feeds concurrently
func (p *Parser) ParseMultipleFeeds(ctx context.Context, feedURLs []string) (map[string][]Entry, error) {
	results := make(map[string][]Entry)
	var mutex sync.Mutex
	var wg sync.WaitGroup

	// Channel to collect errors
	errChan := make(chan error, len(feedURLs))

	for _, feedURL := range feedURLs {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()

			entries, err := p.ParseFeed(ctx, url)
			if err != nil {
				errChan <- fmt.Errorf("failed to parse %s: %w", url, err)
				return
			}

			mutex.Lock()
			results[url] = entries
			mutex.Unlock()
		}(feedURL)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errChan)

	// Collect any errors
	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}

	// Return results even if some feeds failed
	if len(errors) > 0 {
		log.WithField("error_count", len(errors)).Warn("Some RSS feeds failed to parse")
		for _, err := range errors {
			log.WithError(err).Error("RSS parsing error")
		}
	}

	return results, nil
}
