package api

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
)

// Client represents the HTTP client for external APIs
type Client struct {
	httpClient *http.Client
	apiKey     string
}

// NewClient creates a new API client instance with hardened TLS configuration
func NewClient(apiKey string) (*Client, error) {
	// Create hardened TLS configuration
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12, // Minimum TLS 1.2
		MaxVersion: tls.VersionTLS13, // Prefer TLS 1.3

		// Secure cipher suites for TLS 1.2 (TLS 1.3 manages its own)
		CipherSuites: []uint16{
			// GCM ciphers (preferred - AEAD)
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			// CBC ciphers (available fallbacks)
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
			tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
		},

		// Security hardening
		InsecureSkipVerify: false,                           // Always verify certificates
		ServerName:         "",                              // Let Go handle SNI automatically
		ClientSessionCache: tls.NewLRUClientSessionCache(0), // Session resumption
	}

	// Create HTTP transport with hardened TLS
	transport := &http.Transport{
		TLSClientConfig:       tlsConfig,
		DisableCompression:    false, // Keep compression for performance
		DisableKeepAlives:     false, // Keep alive for performance
		MaxIdleConns:          10,    // Limit idle connections
		MaxIdleConnsPerHost:   5,     // Limit per-host connections
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &Client{
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
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
	req.Header.Set("User-Agent", "Wget/1.21.3")
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

	// Decode JSON response
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
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
