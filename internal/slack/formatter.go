package slack

import (
	"fmt"
	"strings"

	"Ransomware-Bot/internal/api"
	"Ransomware-Bot/internal/config"
	"Ransomware-Bot/internal/rss"
)

// SlackMessage represents a Slack message with Block Kit formatting
type SlackMessage struct {
	Text   string  `json:"text"`            // Fallback text
	Blocks []Block `json:"blocks,omitempty"` // Block Kit blocks
}

// Block represents a Slack Block Kit block
type Block struct {
	Type      string        `json:"type"`
	Text      *TextObject   `json:"text,omitempty"`
	Fields    []TextObject  `json:"fields,omitempty"`
	Elements  []TextObject  `json:"elements,omitempty"` // For context blocks
	Accessory interface{}   `json:"accessory,omitempty"`
}

// TextObject represents a text object in Slack Block Kit
type TextObject struct {
	Type string `json:"type"` // plain_text or mrkdwn
	Text string `json:"text"`
}

// formatRansomwareMessage formats a ransomware entry as a Slack Block Kit message
func formatRansomwareMessage(entry api.RansomwareEntry, formatConfig *config.FormatConfig) SlackMessage {
	// Create fallback text
	fallbackText := fmt.Sprintf("Ransomware Alert: %s -> %s", entry.Group, entry.Victim)

	blocks := []Block{}

	// Get Slack-specific title or use default
	titleText := "Ransomware Alert"
	if formatConfig != nil && formatConfig.Slack.TitleText != "" {
		titleText = formatConfig.Slack.TitleText
	}

	// Header block (without emoji for Slack)
	blocks = append(blocks, Block{
		Type: "header",
		Text: &TextObject{
			Type: "plain_text",
			Text: titleText,
		},
	})

	// Main section with key information
	fields := []TextObject{}

	// Use Slack-specific field order if available, otherwise use default
	var fieldOrder []string
	if formatConfig != nil {
		fieldOrder = formatConfig.FieldOrder
		if len(formatConfig.Slack.FieldOrder) > 0 {
			fieldOrder = formatConfig.Slack.FieldOrder
		}
	}
	// Use default field order if formatConfig is nil
	if len(fieldOrder) == 0 {
		fieldOrder = []string{"group", "victim", "country", "activity", "attackdate", "discovered", "screenshot", "post_url", "website", "description"}
	}

	// Build fields based on format config order
	for _, fieldName := range fieldOrder {
		var fieldText string

		switch fieldName {
		case "group":
			if entry.Group != "" {
				fieldText = fmt.Sprintf("*Group:*\n%s", entry.Group)
			}
		case "victim":
			if entry.Victim != "" {
				fieldText = fmt.Sprintf("*Victim:*\n%s", entry.Victim)
			}
		case "country":
			if entry.Country != "" {
				countryText := entry.Country
				if formatConfig != nil && formatConfig.ShowUnicodeFlags {
					countryText = getCountryFlag(entry.Country) + " " + entry.Country
				}
				fieldText = fmt.Sprintf("*Country:*\n%s", countryText)
			}
		case "activity":
			if entry.Activity != "" {
				fieldText = fmt.Sprintf("*Activity:*\n%s", entry.Activity)
			}
		case "attackdate":
			if entry.AttackDate != "" {
				fieldText = fmt.Sprintf("*Attack Date:*\n%s", entry.AttackDate)
			}
		case "discovered":
			if !entry.Discovered.Time.IsZero() {
				fieldText = fmt.Sprintf("*Discovered:*\n%s", entry.Discovered.Time.Format("2006-01-02 15:04:05"))
			}
		}

		if fieldText != "" {
			fields = append(fields, TextObject{
				Type: "mrkdwn",
				Text: fieldText,
			})
		}
	}

	// Add section with fields (Slack limits to 10 fields per section)
	if len(fields) > 0 {
		// Split fields into chunks of max 10 fields
		const maxFieldsPerSection = 10
		for i := 0; i < len(fields); i += maxFieldsPerSection {
			end := i + maxFieldsPerSection
			if end > len(fields) {
				end = len(fields)
			}
			blocks = append(blocks, Block{
				Type:   "section",
				Fields: fields[i:end],
			})
		}
	}

	// Add divider
	blocks = append(blocks, Block{
		Type: "divider",
	})

	// Add URLs section - each URL in its own section with dividers between them
	urlsSectionAdded := false
	for _, fieldName := range fieldOrder {
		var urlBlock *Block

		switch fieldName {
		case "screenshot":
			if entry.Screenshot != "" {
				urlBlock = &Block{
					Type: "section",
					Fields: []TextObject{
						{
							Type: "mrkdwn",
							Text: fmt.Sprintf("*Screenshot:*\n%s", entry.Screenshot),
						},
					},
				}
			}
		case "post_url":
			if entry.ClaimURL != "" {
				// Add divider before Ransom URL if screenshot was added
				if urlsSectionAdded {
					blocks = append(blocks, Block{Type: "divider"})
				}
				urlBlock = &Block{
					Type: "section",
					Fields: []TextObject{
						{
							Type: "mrkdwn",
							Text: fmt.Sprintf("*Ransom URL:*\n`%s`", defangURL(entry.ClaimURL)),
						},
					},
				}
			}
		case "url", "website":
			if entry.URL != "" {
				urlBlock = &Block{
					Type: "section",
					Fields: []TextObject{
						{
							Type: "mrkdwn",
							Text: fmt.Sprintf("*Website:*\n<%s|View Website>", entry.URL),
						},
					},
				}
			}
		}

		if urlBlock != nil {
			blocks = append(blocks, *urlBlock)
			urlsSectionAdded = true
		}
	}

	// Add description if present
	for _, fieldName := range fieldOrder {
		if fieldName == "description" && entry.Description != "" {
			blocks = append(blocks, Block{
				Type: "section",
				Text: &TextObject{
					Type: "mrkdwn",
					Text: fmt.Sprintf("*Description:*\n%s", truncateText(entry.Description, 500)),
				},
			})
			break
		}
	}

	// Add context footer
	blocks = append(blocks, Block{
		Type: "context",
		Elements: []TextObject{
			{
				Type: "mrkdwn",
				Text: fmt.Sprintf("Published: %s | Source: ransomware.live", entry.Published.Time.Format("2006-01-02 15:04:05")),
			},
		},
	})

	return SlackMessage{
		Text:   fallbackText,
		Blocks: blocks,
	}
}

// formatRSSMessage formats an RSS entry as a Slack Block Kit message
func formatRSSMessage(entry rss.Entry, formatConfig *config.FormatConfig) SlackMessage {
	// Create fallback text
	fallbackText := fmt.Sprintf("RSS Update: %s", entry.Title)

	blocks := []Block{}

	// Get Slack-specific RSS title or use default
	rssText := "RSS Feed Update"
	if formatConfig != nil && formatConfig.Slack.RSSText != "" {
		rssText = formatConfig.Slack.RSSText
	}

	// Header block (without emoji for Slack)
	blocks = append(blocks, Block{
		Type: "header",
		Text: &TextObject{
			Type: "plain_text",
			Text: rssText,
		},
	})

	// Title section with larger text
	blocks = append(blocks, Block{
		Type: "section",
		Text: &TextObject{
			Type: "mrkdwn",
			Text: fmt.Sprintf("*%s*", entry.Title),
		},
	})

	// Add divider after title
	blocks = append(blocks, Block{
		Type: "divider",
	})

	// Description if present
	if entry.Description != "" {
		cleanDescription := stripHTML(entry.Description)
		blocks = append(blocks, Block{
			Type: "section",
			Text: &TextObject{
				Type: "mrkdwn",
				Text: truncateText(cleanDescription, 500),
			},
		})
	}

	// Link section (prominent)
	if entry.Link != "" {
		blocks = append(blocks, Block{
			Type: "section",
			Text: &TextObject{
				Type: "mrkdwn",
				Text: fmt.Sprintf("*Link:*\n%s", entry.Link),
			},
		})
	}

	// Metadata section
	fields := []TextObject{}

	if entry.Author != "" {
		fields = append(fields, TextObject{
			Type: "mrkdwn",
			Text: fmt.Sprintf("*Author:*\n%s", entry.Author),
		})
	}

	// Categories
	if len(entry.Categories) > 0 {
		fields = append(fields, TextObject{
			Type: "mrkdwn",
			Text: fmt.Sprintf("*Categories:*\n%s", strings.Join(entry.Categories, ", ")),
		})
	}

	if len(fields) > 0 {
		blocks = append(blocks, Block{
			Type:   "section",
			Fields: fields,
		})
	}

	// Context footer
	publishedTime := "Unknown"
	if !entry.Published.IsZero() {
		publishedTime = entry.Published.Format("2006-01-02 15:04:05")
	}

	feedInfo := entry.FeedTitle
	if feedInfo == "" {
		feedInfo = "RSS Feed"
	}

	blocks = append(blocks, Block{
		Type: "context",
		Elements: []TextObject{
			{
				Type: "mrkdwn",
				Text: fmt.Sprintf("Published: %s | Source: %s", publishedTime, feedInfo),
			},
		},
	})

	return SlackMessage{
		Text:   fallbackText,
		Blocks: blocks,
	}
}

// getCountryFlag returns the flag emoji for a country code
func getCountryFlag(countryCode string) string {
	if len(countryCode) != 2 {
		return ""
	}

	// Convert country code to flag emoji
	// Each letter becomes its regional indicator symbol
	countryCode = strings.ToUpper(countryCode)
	flag := ""
	for _, ch := range countryCode {
		if ch >= 'A' && ch <= 'Z' {
			flag += string(rune(0x1F1E6 + (ch - 'A')))
		}
	}
	return flag
}

// stripHTML removes HTML tags from text (simple implementation)
func stripHTML(text string) string {
	// Remove HTML tags using strings.Builder for efficiency
	var result strings.Builder
	result.Grow(len(text)) // Pre-allocate capacity
	inTag := false
	for _, ch := range text {
		if ch == '<' {
			inTag = true
			continue
		}
		if ch == '>' {
			inTag = false
			continue
		}
		if !inTag {
			result.WriteRune(ch)
		}
	}
	return strings.TrimSpace(result.String())
}

// truncateText truncates text to a maximum length
func truncateText(text string, maxLength int) string {
	if len(text) <= maxLength {
		return text
	}
	return text[:maxLength-3] + "..."
}

// defangURL defangs URLs to prevent accidental clicks
// http://example.com -> hxxp://example[.]com
func defangURL(url string) string {
	defanged := strings.ReplaceAll(url, "http://", "hxxp://")
	defanged = strings.ReplaceAll(defanged, "https://", "hxxps://")
	defanged = strings.ReplaceAll(defanged, ".", "[.]")
	return defanged
}
