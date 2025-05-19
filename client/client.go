package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// BackoffStrategy computes a wait duration based on the retry attempt.
type BackoffStrategy func(attempt int) time.Duration

// Client wraps an http.Client with retry logic and JSON helpers.
type Client struct {
	httpClient *http.Client
	retryCount int
	backoff    BackoffStrategy
}

// ClientOption configures the Client.
type ClientOption func(*Client)

// WithTimeout sets the base HTTP client timeout.
func WithTimeout(d time.Duration) ClientOption {
	return func(c *Client) {
		c.httpClient.Timeout = d
	}
}

// WithRetries sets how many times to retry on server errors.
func WithRetries(count int) ClientOption {
	return func(c *Client) {
		c.retryCount = count
	}
}

// WithBackoffStrategy allows customizing the retry backoff.
func WithBackoffStrategy(bs BackoffStrategy) ClientOption {
	return func(c *Client) {
		c.backoff = bs
	}
}

// NewClient constructs a Client with sensible defaults.
// Defaults: timeout=30s, retries=3, backoff=exponential (attempt * 500ms).
func NewClient(opts ...ClientOption) *Client {
	c := &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		retryCount: 3,
		backoff: func(attempt int) time.Duration {
			return time.Duration(attempt) * 500 * time.Millisecond
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// GetJSON performs an HTTP GET and decodes the JSON response into dest.
func (c *Client) GetJSON(ctx context.Context, url string, dest any) error {
	return c.doJSON(ctx, http.MethodGet, url, nil, dest)
}

// PostJSON performs an HTTP POST with a JSON-encoded body and decodes the JSON response into dest.
func (c *Client) PostJSON(ctx context.Context, url string, body any, dest any) error {
	return c.doJSON(ctx, http.MethodPost, url, body, dest)
}

// doJSON handles the core request logic: marshaling, retries, and decoding.
func (c *Client) doJSON(ctx context.Context, method, url string, body any, dest any) error {

	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		reader = bytes.NewBuffer(b)
	}

	// Build request
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Attempt request with retries on server errors
	var resp *http.Response
	for i := 0; i <= c.retryCount; i++ {
		resp, err = c.httpClient.Do(req)
		if err == nil && resp.StatusCode < 500 {
			break
		}
		wait := c.backoff(i)
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Handle non-2xx
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return errors.New(fmt.Sprintf("unexpected status %d: %s", resp.StatusCode, string(b)))
	}

	// Decode JSON into dest
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(dest); err != nil {
		return fmt.Errorf("decode JSON: %w", err)
	}
	return nil
}
