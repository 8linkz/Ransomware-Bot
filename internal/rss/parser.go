package rss

import (
	"Ransomware-Bot/internal/status"
	"context"
	"fmt"
	"regexp"
	"strings"
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

	// New RSS two-phase tracking methods
	IsRSSItemParsed(feedURL, itemKey string) bool
	MarkRSSItemParsed(feedURL, itemKey string, entry status.StoredRSSEntry)
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

// feedResult holds the result of parsing a single RSS feed
type feedResult struct {
	url     string
	entries []Entry
	err     error
}

// workerPool manages concurrent RSS feed processing
type workerPool struct {
	maxWorkers int
	jobs       chan string
	results    chan feedResult
	parser     *Parser
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
			Description: stripHTML(safeString(item.Description)),
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
			storedEntry := convertToStoredRSSEntry(entry, key)
			p.statusTracker.MarkRSSItemParsed(feedURL, key, storedEntry)
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

	// Prefer GUID if available (most stable)
	if item.GUID != "" {
		return feedURL + ":" + item.GUID
	}

	// Fallback to link if available (very stable)
	if item.Link != "" {
		return feedURL + ":" + item.Link
	}

	// Last resort: use title only (without date to avoid duplicates)
	// Normalize title by trimming and converting to lowercase
	if item.Title != "" {
		normalizedTitle := strings.ToLower(strings.TrimSpace(item.Title))
		return feedURL + ":" + normalizedTitle
	}

	// Ultimate fallback
	return feedURL + ":unknown-item"
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

// convertToStoredRSSEntry converts rss.Entry to status.StoredRSSEntry for persistence
func convertToStoredRSSEntry(entry Entry, key string) status.StoredRSSEntry {
	return status.StoredRSSEntry{
		Key:         key,
		FeedURL:     entry.FeedURL,
		Title:       entry.Title,
		Link:        entry.Link,
		Description: entry.Description,
		Published:   entry.Published.Format("2006-01-02 15:04:05.999999"),
		Author:      entry.Author,
		Categories:  entry.Categories,
		GUID:        entry.GUID,
		FeedTitle:   entry.FeedTitle,
		ParsedAt:    time.Now().Format("2006-01-02 15:04:05.999999"),
	}
}

// newWorkerPool creates a new worker pool for RSS feed processing
func newWorkerPool(maxWorkers int, parser *Parser) *workerPool {
	return &workerPool{
		maxWorkers: maxWorkers,
		jobs:       make(chan string, maxWorkers*2), // Buffer for smooth operation
		results:    make(chan feedResult, maxWorkers*2),
		parser:     parser,
	}
}

// worker processes RSS feeds from the jobs channel
func (wp *workerPool) worker(ctx context.Context) {
	for {
		select {
		case url, ok := <-wp.jobs:
			if !ok {
				return // Jobs channel closed, worker should exit
			}

			// Process the RSS feed
			entries, err := wp.parser.ParseFeed(ctx, url)

			// Send result back
			select {
			case wp.results <- feedResult{url: url, entries: entries, err: err}:
			case <-ctx.Done():
				return
			}

		case <-ctx.Done():
			return // Context cancelled, worker should exit
		}
	}
}

// processFeeds starts workers and processes all feeds with controlled concurrency
func (wp *workerPool) processFeeds(ctx context.Context, feedURLs []string) (map[string][]Entry, error) {
	// Start workers
	for i := 0; i < wp.maxWorkers; i++ {
		go wp.worker(ctx)
	}

	// Send all jobs
	go func() {
		defer close(wp.jobs)
		for _, url := range feedURLs {
			select {
			case wp.jobs <- url:
			case <-ctx.Done():
				return
			}
		}
	}()

	// Collect results
	results := make(map[string][]Entry)
	var errors []error

	for i := 0; i < len(feedURLs); i++ {
		select {
		case result := <-wp.results:
			if result.err != nil {
				errors = append(errors, fmt.Errorf("failed to parse %s: %w", result.url, result.err))
				// Include failed feeds with empty results for status tracking
				results[result.url] = []Entry{}
			} else {
				results[result.url] = result.entries
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(30 * time.Second):
			log.WithField("remaining_feeds", len(feedURLs)-i).Error("Worker pool timeout waiting for feed results")
			// Return partial results instead of failing completely
			return results, fmt.Errorf("worker pool timeout after processing %d of %d feeds", i, len(feedURLs))
		}
	}

	// Log any errors but don't fail completely
	if len(errors) > 0 {
		log.WithField("error_count", len(errors)).Warn("Some RSS feeds failed to parse with worker pool")
		for _, err := range errors {
			log.WithError(err).Error("RSS worker pool parsing error")
		}
	}

	return results, nil
}

// ParseMultipleFeeds parses multiple RSS feeds concurrently with worker pool limiting
func (p *Parser) ParseMultipleFeeds(ctx context.Context, feedURLs []string, maxWorkers int) (map[string][]Entry, error) {
	if len(feedURLs) == 0 {
		return make(map[string][]Entry), nil
	}

	// Limit workers to available feeds if fewer feeds than workers
	if maxWorkers > len(feedURLs) {
		maxWorkers = len(feedURLs)
	}

	// Create and use worker pool for controlled concurrency
	pool := newWorkerPool(maxWorkers, p)
	return pool.processFeeds(ctx, feedURLs)
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

// stripHTML removes HTML tags and decodes HTML entities from text
func stripHTML(input string) string {
	if input == "" {
		return ""
	}

	// Remove HTML tags using regex
	re := regexp.MustCompile(`<[^>]*>`)
	text := re.ReplaceAllString(input, "")

	// Decode common HTML entities
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&quot;", "\"")
	text = strings.ReplaceAll(text, "&#39;", "'")
	text = strings.ReplaceAll(text, "&nbsp;", " ")

	// Clean up multiple spaces and trim
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")
	return strings.TrimSpace(text)
}
