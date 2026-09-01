// Package ebird is a client for the eBird API 2.0.
package ebird

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const baseURL = "https://api.ebird.org/v2"

// Client calls the eBird API, authenticating every request with an API key.
type Client struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
}

// NewClient returns a Client authenticated with apiKey. httpClient may be
// nil, in which case a client with a 30s timeout is used.
func NewClient(apiKey string, httpClient *http.Client) (*Client, error) {
	if apiKey == "" {
		return nil, errors.New("ebird: API key is required")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{httpClient: httpClient, baseURL: baseURL, apiKey: apiKey}, nil
}

// get issues an authenticated GET against path and decodes the JSON response
// body into out.
func (c *Client) get(ctx context.Context, path string, query url.Values, out any) error {
	reqURL := c.baseURL + path
	if query != nil {
		reqURL += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("X-eBirdApiToken", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("requesting %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("requesting %s: unexpected status %s", path, resp.Status)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decoding %s response: %w", path, err)
	}
	return nil
}
