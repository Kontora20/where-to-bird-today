// Package ebird is a client for the eBird API 2.0.
package ebird

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"sync/atomic"
	"time"
)

const baseURL = "https://api.ebird.org/v2"

// ErrNotFound is returned when eBird answers 404, which it does for a
// checklist that was deleted after it was listed.
var ErrNotFound = errors.New("ebird: not found")

// ErrRateLimited is returned when eBird answers 429.
var ErrRateLimited = errors.New("ebird: rate limited")

// Client calls the eBird API, authenticating every request with an API key.
type Client struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
	// quotaLeft is the requests left in eBird's hourly limit as of the last
	// response, or -1 before eBird has said.
	quotaLeft atomic.Int64
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
	client := &Client{httpClient: httpClient, baseURL: baseURL, apiKey: apiKey}
	client.quotaLeft.Store(-1)
	return client, nil
}

// QuotaLeft returns the requests left in eBird's hourly limit as of the last
// response, and false before eBird has said.
func (c *Client) QuotaLeft() (int, bool) {
	left := c.quotaLeft.Load()
	return int(left), left >= 0
}

// policyLeft finds the hourly figure in eBird's RateLimit header, which reads
// like `"policy";r=412,"burst";r=24`.
var policyLeft = regexp.MustCompile(`"policy";r=(\d+)`)

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

	if match := policyLeft.FindStringSubmatch(resp.Header.Get("RateLimit")); match != nil {
		if left, err := strconv.Atoi(match[1]); err == nil {
			c.quotaLeft.Store(int64(left))
		}
	}

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("requesting %s: %w", path, ErrNotFound)
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return fmt.Errorf("requesting %s: %w", path, ErrRateLimited)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("requesting %s: unexpected status %s", path, resp.Status)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decoding %s response: %w", path, err)
	}
	return nil
}
