# Ransomware Bot

A high-performance Discord bot written in **Go** that delivers threat intelligence data via Discord webhooks. The bot fetches data from the ransomware.live API and multiple RSS feeds, providing regular cybersecurity alerts to your Discord channels.
The original bot from vx-underground no longer works, so I built a new one (https://github.com/vxunderground/ThreatIntelligenceDiscordBot).

⚠️ Disclaimer
This bot is 100% vibe-coded - I provide no guarantee for 100% security.

<img width="524" height="377" alt="grafik" src="https://github.com/user-attachments/assets/699b63de-043e-40cc-9fb8-e396cf55ce79" />

## 🔧 Prerequisites

### API Key
The ransomware.live API allows **3,000 calls per day** (API Pro). You need to request an API key at: https://www.ransomware.live/api

### Discord Webhooks
To create Discord webhooks on your own Discord server:
1. Go to your Discord server settings
2. Navigate to "Integrations" → "Webhooks"
3. Click "Create Webhook"
4. Choose the channel and copy the webhook URL

⚠️ **Security Warning**: Never share your webhook URLs with others - they provide direct access to post messages in your Discord channels.

## ⚙️ Configuration

### 1. General Configuration (`configs/config_general.json`)

```json
{
  "log_level": "INFO",
  "log_rotation": {
    "max_size_mb": 10,
    "max_backups": 5,
    "max_age_days": 7,
    "compress": true
  },
  "api_key": "YOUR_RANSOMWARE_LIVE_API_KEY",
  "api_poll_interval": "1h",
  "rss_poll_interval": "30m",
  "api_start_time": "",
  "rss_retry_count": 3,
  "rss_retry_delay": "2s",
  "discord_delay": "2s",
  "webhooks": {
    "ransomware": {
      "enabled": true,
      "url": "https://discord.com/api/webhooks/YOUR_WEBHOOK_ID/YOUR_WEBHOOK_TOKEN"
    },
    "rss": {
      "enabled": true,
      "url": "https://discord.com/api/webhooks/YOUR_WEBHOOK_ID/YOUR_WEBHOOK_TOKEN"
    },
    "government": {
      "enabled": false,
      "url": "https://discord.com/api/webhooks/YOUR_WEBHOOK_ID/YOUR_WEBHOOK_TOKEN"
    }
  }
}
```

**Configuration Options:**

| Field | Description | Example |
|-------|-------------|---------|
| `log_level` | Logging verbosity | `"DEBUG"`, `"INFO"`, `"WARNING"`, `"ERROR"` |
| `api_key` | Ransomware.live API key | `"your-api-key-here"` |
| `api_poll_interval` | API checking frequency | `"1h"`, `"30m"`, `"15m"` |
| `rss_poll_interval` | RSS checking frequency | `"30m"`, `"15m"`, `"5m"` |
| `discord_delay` | Delay before Discord push | `"2s"`, `"1s"`, `"500ms"` |
| `rss_retry_count` | Failed feed retry attempts | `3`, `5`, `10` |
| `rss_retry_delay` | Delay between retries | `"2s"`, `"5s"`, `"10s"` |


### 2. RSS Feeds Configuration (`configs/config_feeds.json`)

```json
{
  "ransomware_feeds": [],
  "government_feeds": [
    "https://www.cisa.gov/uscert/ncas/alerts.xml",
    "https://www.ncsc.gov.uk/api/1/services/v1/report-rss-feed.xml",
    "https://www.cisecurity.org/feed/advisories"
  ],
  "general_feeds": [
    "https://grahamcluley.com/feed/",
    "https://krebsonsecurity.com/feed/",
    "https://www.darkreading.com/rss.xml",
    "https://www.bleepingcomputer.com/feed/",
    "https://www.schneier.com/feed/atom/",
    "https://securelist.com/feed/",
    "https://research.checkpoint.com/feed/",
    "https://www.proofpoint.com/us/rss.xml",
    "https://redcanary.com/feed/",
    "https://www.sentinelone.com/feed/"
  ]
}
```

**Feed Categories:**
- `ransomware_feeds` - RSS feeds specific to ransomware threats (routed to ransomware webhook)
- `government_feeds` - Government cybersecurity alerts (routed to government webhook)  
- `general_feeds` - General cybersecurity news and research (routed to RSS webhook)

💡 **Tip**: You can easily add, remove, or modify feed URLs in any category to customize your threat intelligence sources.

### 3. Message Formatting (`configs/config_format.json`)

```json
{
  "show_unicode_flags": true,
  "field_order": [
    "victim",
    "activity", 
    "country",
    "website",
    "group",
    "discovered",
    "attackdate",
    "description"
  ]
}
```

**Formatting Options:**
- `show_unicode_flags` - Display country flag emojis (🇺🇸, 🇩🇪, etc.)
- `field_order` - Customizable order of fields in Discord messages

💡 **Tip**: You can reorder fields or remove unwanted ones from the `field_order` array to customize your Discord message layout.

**Available Fields (if available):**
- `id` - Unique entry identifier
- `victim` - Target organization
- `group` - Ransomware group name
- `country` - Country (with flag emoji if enabled)
- `activity` - Attack classification
- `attackdate` - When attack occurred
- `discovered` - When attack was discovered
- `post_url` - Link to ransom post/leak page
- `website` - Victim's website
- `description` - Attack details
- `screenshot` - Ransom note screenshot
- `published` - Publication timestamp

## 📦 Docker Compatibility

✅ **Tested on Unraid**: This container has been successfully tested on Unraid without any issues.  
✅ **Lightweight**: Docker image size is approximately ~36MB

### Building the Container

```bash
# Clone the repository
git clone <repository-url>
cd <repository-name>

# Build the Docker image
docker build -t ransomware-bot .

# Save the image as a tar file (optional)
docker save -o ransomware-bot.tar ransomware-bot:latest

# Or use Docker Compose to build and run
docker-compose up -d --build
```

### Required Volume Mounts
* **Configuration**: `/app/config` - Mount your config directory here
* **Logs**: `/app/logs` - Application logs with rotation  
* **Data**: `/app/data` - **Critical for persistence** (status tracking/deduplication)

⚠️ **Important**: Without the `/app/data` volume, all processed items tracking will be lost on container restart, causing duplicate Discord messages.

## Acknowledgments

- [ransomware.live](https://ransomware.live) for providing the threat intelligence API
- [discordgo](https://github.com/bwmarrin/discordgo) for Discord integration
- [gofeed](https://github.com/mmcdole/gofeed) for RSS parsing capabilities
- The cybersecurity community for RSS feed sources
