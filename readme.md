# Discord Threat Intelligence Bot

A high-performance Discord bot written in **Go** that delivers threat intelligence data via Discord webhooks. The bot fetches data from the ransomware.live API and multiple RSS feeds, providing real-time cybersecurity alerts to your Discord channels.

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

| Field | Description | Example | Default |
|-------|-------------|---------|---------|
| `log_level` | Logging verbosity | `"DEBUG"`, `"INFO"`, `"WARNING"`, `"ERROR"` | `"INFO"` |
| `api_key` | Ransomware.live API key | `"your-api-key-here"` | `""` |
| `api_poll_interval` | API checking frequency | `"1h"`, `"30m"`, `"15m"` | `"1h"` |
| `rss_poll_interval` | RSS checking frequency | `"30m"`, `"15m"`, `"5m"` | `"30m"` |
| `discord_delay` | Delay before Discord push | `"2s"`, `"1s"`, `"500ms"` | `"2s"` |
| `rss_retry_count` | Failed feed retry attempts | `3`, `5`, `10` | `3` |
| `rss_retry_delay` | Delay between retries | `"2s"`, `"5s"`, `"10s"` | `"2s"` |

### 2. RSS Feeds Configuration (`configs/config_feeds.json`)

```json
{
  "ransomware_feeds": [
    "https://example.com/ransomware.xml"
  ],
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
    "https://cybersecurity.att.com/site/blog-all-rss",
    "https://redcanary.com/feed/",
    "https://www.sentinelone.com/feed/"
  ]
}
```

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

## Acknowledgments

- [ransomware.live](https://ransomware.live) for providing the threat intelligence API
- [discordgo](https://github.com/bwmarrin/discordgo) for Discord integration
- [gofeed](https://github.com/mmcdole/gofeed) for RSS parsing capabilities
- The cybersecurity community for RSS feed sources