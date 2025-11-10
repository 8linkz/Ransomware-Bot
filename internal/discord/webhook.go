package discord

import (
	"fmt"
	"strings"

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

	return &WebhookSender{
		session: session,
	}
}

// SendRansomwareEntry sends a ransomware entry to a Discord webhook
func (w *WebhookSender) SendRansomwareEntry(webhookURL string, entry api.RansomwareEntry, formatConfig *config.FormatConfig) error {
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
	if err := w.executeWebhook(webhookID, webhookToken, params); err != nil {
		return fmt.Errorf("failed to send ransomware entry: %w", err)
	}

	log.WithFields(log.Fields{
		"group":  entry.Group,
		"victim": entry.Victim,
	}).Info("Sent ransomware entry to Discord")

	return nil
}

// SendRSSEntry sends an RSS entry to a Discord webhook
func (w *WebhookSender) SendRSSEntry(webhookURL string, entry rss.Entry) error {
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
	if err := w.executeWebhook(webhookID, webhookToken, params); err != nil {
		return fmt.Errorf("failed to send RSS entry: %w", err)
	}

	log.WithFields(log.Fields{
		"title":      entry.Title,
		"feed_title": entry.FeedTitle,
	}).Info("Sent RSS entry to Discord")

	return nil
}

// executeWebhook sends a webhook message to Discord
func (w *WebhookSender) executeWebhook(webhookID, webhookToken string, params *discordgo.WebhookParams) error {
	if w.session == nil {
		return fmt.Errorf("discord session not available")
	}

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
