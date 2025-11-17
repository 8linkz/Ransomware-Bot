package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"Ransomware-Bot/internal/api"
	"Ransomware-Bot/internal/config"
	"Ransomware-Bot/internal/rss"

	log "github.com/sirupsen/logrus"
)

// WebhookSender handles sending messages to Slack webhooks
type WebhookSender struct {
	client *http.Client
}

// NewWebhookSender creates a new Slack webhook sender instance
func NewWebhookSender() *WebhookSender {
	return &WebhookSender{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SendRansomwareEntry sends a ransomware entry to a Slack webhook
func (w *WebhookSender) SendRansomwareEntry(ctx context.Context, webhookURL string, entry api.RansomwareEntry, formatConfig *config.FormatConfig) error {
	// Check if context is cancelled before proceeding
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Format the entry as a Slack Block Kit message
	payload := formatRansomwareMessage(entry, formatConfig)

	// Send the webhook
	if err := w.executeWebhook(ctx, webhookURL, payload); err != nil {
		return fmt.Errorf("failed to send ransomware entry: %w", err)
	}

	log.WithFields(log.Fields{
		"group":  entry.Group,
		"victim": entry.Victim,
	}).Info("Sent ransomware entry to Slack")

	return nil
}

// SendRansomwareEntriesBatch sends multiple ransomware entries as separate messages in sequence
// Note: Slack doesn't support multiple blocks in one message like Discord's embeds,
// so we send them as individual messages with rate limiting
func (w *WebhookSender) SendRansomwareEntriesBatch(ctx context.Context, webhookURL string, entries []api.RansomwareEntry, formatConfig *config.FormatConfig) error {
	if len(entries) == 0 {
		return nil
	}

	// Group entries into batches of 10 for logging/tracking purposes
	const batchSize = 10
	totalBatches := (len(entries) + batchSize - 1) / batchSize

	for batchNum := 0; batchNum < totalBatches; batchNum++ {
		start := batchNum * batchSize
		end := start + batchSize
		if end > len(entries) {
			end = len(entries)
		}

		batch := entries[start:end]

		// Send each entry in the batch
		for _, entry := range batch {
			// Check context before each send
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			payload := formatRansomwareMessage(entry, formatConfig)
			if err := w.executeWebhook(ctx, webhookURL, payload); err != nil {
				return fmt.Errorf("failed to send entry in batch %d: %w", batchNum+1, err)
			}

			// Small delay between messages to avoid rate limiting (handled by scheduler)
		}

		log.WithFields(log.Fields{
			"batch":       batchNum + 1,
			"total_batch": totalBatches,
			"batch_size":  len(batch),
			"total":       len(entries),
		}).Info("Sent ransomware entries batch to Slack")
	}

	return nil
}

// SendRSSEntry sends an RSS entry to a Slack webhook
func (w *WebhookSender) SendRSSEntry(ctx context.Context, webhookURL string, entry rss.Entry, formatConfig *config.FormatConfig) error {
	// Check if context is cancelled before proceeding
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Format the entry as a Slack Block Kit message
	payload := formatRSSMessage(entry, formatConfig)

	// Send the webhook
	if err := w.executeWebhook(ctx, webhookURL, payload); err != nil {
		return fmt.Errorf("failed to send RSS entry: %w", err)
	}

	log.WithFields(log.Fields{
		"title":      entry.Title,
		"feed_title": entry.FeedTitle,
	}).Info("Sent RSS entry to Slack")

	return nil
}

// executeWebhook sends a webhook message to Slack
func (w *WebhookSender) executeWebhook(ctx context.Context, webhookURL string, payload interface{}) error {
	// Marshal payload to JSON
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Create HTTP request with context
	req, err := http.NewRequestWithContext(ctx, "POST", webhookURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack webhook returned status %d: %s", resp.StatusCode, resp.Status)
	}

	return nil
}
