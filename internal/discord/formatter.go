package discord

import (
	"fmt"
	"strings"
	"time"

	"Ransomware-Bot/internal/api"
	"Ransomware-Bot/internal/config"
	"Ransomware-Bot/internal/country"
	"Ransomware-Bot/internal/rss"

	"github.com/bwmarrin/discordgo"
)

const (
	// Color constants for Discord embeds
	ColorRansomware = 0xff0000 // Red for ransomware alerts
	ColorRSS        = 0x0099ff // Blue for RSS feeds
	ColorGovernment = 0xffa500 // Orange for government feeds
)

// formatRansomwareEmbed creates a Discord embed for a ransomware entry
func (w *WebhookSender) formatRansomwareEmbed(entry api.RansomwareEntry, formatConfig *config.FormatConfig) *discordgo.MessageEmbed {
	embed := &discordgo.MessageEmbed{
		Title:     fmt.Sprintf("🚨 Ransomware Alert: %s", entry.Group),
		Color:     ColorRansomware,
		Timestamp: entry.Discovered.Time.Format(time.RFC3339),
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Ransomware.live API",
		},
	}

	// Add fields based on the format configuration
	for _, fieldName := range formatConfig.FieldOrder {
		// Add spacing before description
		if fieldName == "description" {
			// Add empty field for spacing
			spacer := &discordgo.MessageEmbedField{
				Name:   "\u200B", // Invisible character
				Value:  "\u200B", // Invisible character
				Inline: false,
			}
			embed.Fields = append(embed.Fields, spacer)
		}

		field := w.createRansomwareField(fieldName, entry, formatConfig.ShowUnicodeFlags)
		if field != nil {
			embed.Fields = append(embed.Fields, field)
		}
	}

	return embed
}

// createRansomwareField creates a Discord embed field for a specific ransomware entry field
func (w *WebhookSender) createRansomwareField(fieldName string, entry api.RansomwareEntry, showFlags bool) *discordgo.MessageEmbedField {
	switch strings.ToLower(fieldName) {
	case "country":
		if entry.Country != "" {
			value := country.FormatCountryDisplay(entry.Country, showFlags)
			return &discordgo.MessageEmbedField{
				Name:   "🌍 Country",
				Value:  value,
				Inline: true,
			}
		}

	case "victim":
		if entry.Victim != "" {
			return &discordgo.MessageEmbedField{
				Name:   "🎯 Victim",
				Value:  entry.Victim,
				Inline: true,
			}
		}

	case "group":
		if entry.Group != "" {
			return &discordgo.MessageEmbedField{
				Name:   "💀 Group",
				Value:  entry.Group,
				Inline: true,
			}
		}

	case "activity":
		if entry.Activity != "" {
			return &discordgo.MessageEmbedField{
				Name:   "📊 Activity",
				Value:  entry.Activity,
				Inline: true,
			}
		}

	case "attackdate":
		if entry.AttackDate != "" {
			// Parse the date and reformat it
			if t, err := time.Parse("2006-01-02 15:04:05.999999", entry.AttackDate); err == nil {
				// Format without microseconds
				formattedDate := t.Format("2006-01-02 15:04:05")
				return &discordgo.MessageEmbedField{
					Name:   "⚔️ Attack Date",
					Value:  formattedDate,
					Inline: true,
				}
			} else if _, err := time.Parse("2006-01-02 15:04:05", entry.AttackDate); err == nil {
				// Already in correct format
				return &discordgo.MessageEmbedField{
					Name:   "⚔️ Attack Date",
					Value:  entry.AttackDate,
					Inline: true,
				}
			} else {
				// Fallback: use as-is
				return &discordgo.MessageEmbedField{
					Name:   "⚔️ Attack Date",
					Value:  entry.AttackDate,
					Inline: true,
				}
			}
		}

	case "discovered":
		return &discordgo.MessageEmbedField{
			Name:   "🔍 Discovered",
			Value:  entry.Discovered.Format("2006-01-02 15:04:05"),
			Inline: true,
		}

	case "post_url":
		if entry.ClaimURL != "" {
			return &discordgo.MessageEmbedField{
				Name:   "🔗 Ransom URL",
				Value:  entry.ClaimURL,
				Inline: false,
			}
		}

	case "website":
		if entry.URL != "" {
			return &discordgo.MessageEmbedField{
				Name:   "🌐 Website",
				Value:  entry.URL,
				Inline: false,
			}
		}

	case "description":
		if entry.Description != "" && len(entry.Description) <= 1024 {
			return &discordgo.MessageEmbedField{
				Name:   "📝 Description",
				Value:  entry.Description,
				Inline: false,
			}
		}

	case "screenshot":
		if entry.Screenshot != "" {
			return &discordgo.MessageEmbedField{
				Name:   "📸 Screenshot",
				Value:  fmt.Sprintf("[View Screenshot](%s)", entry.Screenshot),
				Inline: false,
			}
		}
	}

	return nil
}

// formatRSSEmbed creates a Discord embed for an RSS entry
func (w *WebhookSender) formatRSSEmbed(entry rss.Entry) *discordgo.MessageEmbed {
	embed := &discordgo.MessageEmbed{
		Title:       entry.Title,
		Description: w.truncateDescription(entry.Description, 2048),
		Color:       ColorRSS,
		Timestamp:   entry.Published.Format(time.RFC3339),
		Footer: &discordgo.MessageEmbedFooter{
			Text: entry.FeedTitle,
		},
	}

	// Add URL if available
	if entry.Link != "" {
		embed.URL = entry.Link
	}

	// Add author if available
	if entry.Author != "" {
		embed.Author = &discordgo.MessageEmbedAuthor{
			Name: entry.Author,
		}
	}

	// Add categories as a field if available
	if len(entry.Categories) > 0 {
		categoriesStr := strings.Join(entry.Categories, ", ")
		if len(categoriesStr) <= 1024 {
			embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
				Name:   "📂 Categories",
				Value:  categoriesStr,
				Inline: false,
			})
		}
	}

	// Add publication date field
	embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
		Name:   "📅 Published",
		Value:  entry.Published.Format("2006-01-02 15:04:05"),
		Inline: true,
	})

	return embed
}

// truncateDescription truncates a description to fit Discord's limits
func (w *WebhookSender) truncateDescription(description string, maxLength int) string {
	if len(description) <= maxLength {
		return description
	}

	// Find a good place to cut off (preferably at a sentence boundary)
	truncated := description[:maxLength-3]

	// Look for the last period, exclamation mark, or question mark
	lastSentenceEnd := -1
	for i := len(truncated) - 1; i >= 0; i-- {
		if truncated[i] == '.' || truncated[i] == '!' || truncated[i] == '?' {
			lastSentenceEnd = i
			break
		}
	}

	// If we found a sentence boundary and it's not too early, use it
	if lastSentenceEnd > maxLength/2 {
		return truncated[:lastSentenceEnd+1]
	}

	// Otherwise, just truncate and add ellipsis
	return truncated + "..."
}

// CreateStatusEmbed creates a status/info embed
func (w *WebhookSender) CreateStatusEmbed(title, message string, color int) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title:       title,
		Description: message,
		Color:       color,
		Timestamp:   time.Now().Format(time.RFC3339),
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Discord Threat Intel Bot",
		},
	}
}

// SendStatusMessage sends a status message to a webhook
func (w *WebhookSender) SendStatusMessage(webhookURL, title, message string) error {
	embed := w.CreateStatusEmbed(title, message, 0x00ff00) // Green color for status

	params := &discordgo.WebhookParams{
		Embeds: []*discordgo.MessageEmbed{embed},
	}

	webhookID, webhookToken, err := parseWebhookURL(webhookURL)
	if err != nil {
		return fmt.Errorf("invalid webhook URL: %w", err)
	}

	return w.executeWebhook(webhookID, webhookToken, params)
}
