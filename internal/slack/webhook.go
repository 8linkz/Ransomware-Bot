package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
func (w *WebhookSender) executeWebhook(ctx context.Context, webhookURL string, payload any) error {
	// Marshal payload to JSON
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Log the JSON payload at debug level
	log.WithField("payload_size", len(jsonData)).Debug("Sending Slack webhook request")
	log.WithField("payload", string(jsonData)).Trace("Slack webhook payload")

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

	// Read response body for detailed error information
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		log.WithError(readErr).Warn("Failed to read Slack response body")
	}

	// Check response status
	if resp.StatusCode != http.StatusOK {
		// Log the full error details from Slack
		bodyStr := string(body)

		// Log to console with full details for debugging
		fmt.Printf("\n=== SLACK WEBHOOK ERROR ===\n")
		fmt.Printf("Status Code: %d\n", resp.StatusCode)
		fmt.Printf("Response Body: %s\n", bodyStr)
		fmt.Printf("Payload Size: %d bytes\n", len(jsonData))
		fmt.Printf("Payload JSON:\n%s\n", string(jsonData))
		fmt.Printf("===========================\n\n")

		log.WithFields(log.Fields{
			"status_code":   resp.StatusCode,
			"response_body": bodyStr,
			"payload_size":  len(jsonData),
		}).Error("Slack webhook request failed")

		return fmt.Errorf("slack webhook returned status %d: %s - response: %s", resp.StatusCode, resp.Status, bodyStr)
	}

	// Log successful response at debug level
	log.WithField("response", string(body)).Debug("Slack webhook response")

	return nil
}
