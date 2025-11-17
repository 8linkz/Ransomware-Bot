package discord

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"Ransomware-Bot/internal/api"
	"Ransomware-Bot/internal/config"
	"Ransomware-Bot/internal/rss"

	"github.com/bwmarrin/discordgo"
	log "github.com/sirupsen/logrus"
)

// WebhookSender handles sending messages to Discord webhooks
type WebhookSender struct {
	session *discordgo.Session
}

// NewWebhookSender creates a new webhook sender instance
func NewWebhookSender() *WebhookSender {
	// For webhook-only operations, we create a minimal Discord session
	// The empty token is fine since we're only using webhook functionality
	session, err := discordgo.New("")
	if err != nil {
		log.WithError(err).Fatal("Failed to create Discord session, webhook operations may be limited")
	}

	// Configure HTTP client with timeout to prevent hanging requests
	session.Client = &http.Client{
		Timeout: 10 * time.Second,
	}

	return &WebhookSender{
		session: session,
	}
}

// SendRansomwareEntry sends a ransomware entry to a Discord webhook
func (w *WebhookSender) SendRansomwareEntry(ctx context.Context, webhookURL string, entry api.RansomwareEntry, formatConfig *config.FormatConfig) error {
	// Check if context is cancelled before proceeding
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Format the entry as a Discord embed
	embed := w.formatRansomwareEmbed(entry, formatConfig)

	// Create webhook parameters
	params := &discordgo.WebhookParams{
		Embeds: []*discordgo.MessageEmbed{embed},
	}

	// Extract webhook ID and token from URL
	webhookID, webhookToken, err := parseWebhookURL(webhookURL)
	if err != nil {
		return fmt.Errorf("invalid webhook URL: %w", err)
	}

	// Send the webhook
	if err := w.executeWebhook(ctx, webhookID, webhookToken, params); err != nil {
		return fmt.Errorf("failed to send ransomware entry: %w", err)
	}

	log.WithFields(log.Fields{
		"group":  entry.Group,
		"victim": entry.Victim,
	}).Info("Sent ransomware entry to Discord")

	return nil
}

// SendRansomwareEntriesBatch sends multiple ransomware entries in batches (max 10 per message)
func (w *WebhookSender) SendRansomwareEntriesBatch(ctx context.Context, webhookURL string, entries []api.RansomwareEntry, formatConfig *config.FormatConfig) error {
	if len(entries) == 0 {
		return nil
	}

	// Check if context is cancelled before proceeding
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	webhookID, webhookToken, err := parseWebhookURL(webhookURL)
	if err != nil {
		return fmt.Errorf("invalid webhook URL: %w", err)
	}

	// Discord allows max 10 embeds per message
	const maxEmbedsPerMessage = 10

	// Process entries in batches
	for i := 0; i < len(entries); i += maxEmbedsPerMessage {
		// Check context before each batch
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Calculate batch size
		end := i + maxEmbedsPerMessage
		if end > len(entries) {
			end = len(entries)
		}

		batch := entries[i:end]
		embeds := make([]*discordgo.MessageEmbed, len(batch))

		// Format each entry in the batch
		for j, entry := range batch {
			embeds[j] = w.formatRansomwareEmbed(entry, formatConfig)
		}

		params := &discordgo.WebhookParams{
			Embeds: embeds,
		}

		// Send the batch
		if err := w.executeWebhook(ctx, webhookID, webhookToken, params); err != nil {
			return fmt.Errorf("failed to send batch %d-%d: %w", i, end, err)
		}

		log.WithFields(log.Fields{
			"batch_start": i + 1,
			"batch_end":   end,
			"batch_size":  len(batch),
			"total":       len(entries),
		}).Info("Sent ransomware entries batch to Discord")
	}

	return nil
}

// SendRSSEntry sends an RSS entry to a Discord webhook
func (w *WebhookSender) SendRSSEntry(ctx context.Context, webhookURL string, entry rss.Entry) error {
	// Check if context is cancelled before proceeding
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Format the entry as a Discord embed
	embed := w.formatRSSEmbed(entry)

	// Create webhook parameters
	params := &discordgo.WebhookParams{
		Embeds: []*discordgo.MessageEmbed{embed},
	}

	// Extract webhook ID and token from URL
	webhookID, webhookToken, err := parseWebhookURL(webhookURL)
	if err != nil {
		return fmt.Errorf("invalid webhook URL: %w", err)
	}

	// Send the webhook
	if err := w.executeWebhook(ctx, webhookID, webhookToken, params); err != nil {
		return fmt.Errorf("failed to send RSS entry: %w", err)
	}

	log.WithFields(log.Fields{
		"title":      entry.Title,
		"feed_title": entry.FeedTitle,
	}).Info("Sent RSS entry to Discord")

	return nil
}

// executeWebhook sends a webhook message to Discord
func (w *WebhookSender) executeWebhook(ctx context.Context, webhookID, webhookToken string, params *discordgo.WebhookParams) error {
	if w.session == nil {
		return fmt.Errorf("discord session not available")
	}

	// Check context before executing
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Note: discordgo doesn't support context natively, but we check before sending
	_, err := w.session.WebhookExecute(webhookID, webhookToken, false, params)
	return err
}

// parseWebhookURL extracts the webhook ID and token from a Discord webhook URL
func parseWebhookURL(webhookURL string) (string, string, error) {
	// Discord webhook URLs have the format:
	// https://discord.com/api/webhooks/{webhook.id}/{webhook.token}

	const prefix = "https://discord.com/api/webhooks/"
	if !strings.HasPrefix(webhookURL, prefix) {
		return "", "", fmt.Errorf("invalid webhook URL format")
	}

	// Remove the prefix
	remainder := webhookURL[len(prefix):]

	// Split by '/' to get ID and token
	parts := strings.Split(remainder, "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("webhook URL missing ID or token")
	}

	webhookID := parts[0]
	webhookToken := parts[1]

	if webhookID == "" || webhookToken == "" {
		return "", "", fmt.Errorf("webhook ID or token is empty")
	}

	return webhookID, webhookToken, nil
}
