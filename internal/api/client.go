package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
)

// Client represents the HTTP client for external APIs
type Client struct {
	httpClient *http.Client
	apiKey     string
}

// NewClient creates a new API client instance
func NewClient(apiKey string) (*Client, error) {
	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		apiKey: apiKey,
	}, nil
}

// makeRequest performs an HTTP request with proper error handling and retries
func (c *Client) makeRequest(ctx context.Context, url string, target interface{}) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Add API key if provided
	if c.apiKey != "" {
		req.Header.Set("X-API-KEY", c.apiKey)
	}

	// Set common headers
	req.Header.Set("User-Agent", "Discord-Threat-Intel-Bot/1.0")
	req.Header.Set("Accept", "application/json")

	// Perform request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API returned status %d: %s", resp.StatusCode, resp.Status)
	}

	// Restrict response size to prevent excessive memory usage
	const maxResponseSize = 10 * 1024 * 1024 // 10MB
	limitedReader := io.LimitReader(resp.Body, maxResponseSize)

	// Decode JSON response
	if err := json.NewDecoder(limitedReader).Decode(target); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	return nil
}

// logRequest logs API request details
func (c *Client) logRequest(url string, duration time.Duration, err error) {
	fields := log.Fields{
		"url":      url,
		"duration": duration,
	}

	if err != nil {
		log.WithFields(fields).WithError(err).Error("API request failed")
	} else {
		log.WithFields(fields).Debug("API request successful")
	}
}
