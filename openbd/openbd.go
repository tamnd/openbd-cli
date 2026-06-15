// Package openbd is the library behind the openbd command line:
// the HTTP client, request shaping, and typed data models for the OpenBD API
// (https://api.openbd.jp), the Japanese book database with 1.9M ISBNs.
//
// No API key is required. The Client paces requests, sets a real User-Agent,
// and retries transient failures (429 and 5xx).
package openbd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Host is the site this client talks to.
const Host = "api.openbd.jp"

// Config holds all tunable parameters for the Client.
type Config struct {
	BaseURL   string
	UserAgent string
	Rate      time.Duration
	Timeout   time.Duration
	Retries   int
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		BaseURL:   "https://api.openbd.jp",
		UserAgent: "openbd-cli/0.1 (+https://github.com/tamnd/openbd-cli)",
		Rate:      200 * time.Millisecond,
		Timeout:   15 * time.Second,
		Retries:   3,
	}
}

// Client talks to the OpenBD API.
type Client struct {
	cfg  Config
	http *http.Client
	mu   sync.Mutex
	last time.Time
}

// NewClient returns a Client with the given configuration.
func NewClient(cfg Config) *Client {
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: cfg.Timeout},
	}
}

// Book holds the public data about a single book.
type Book struct {
	ISBN      string `kit:"id" json:"isbn"`
	Title     string `json:"title"`
	Author    string `json:"author"`
	Publisher string `json:"publisher"`
	PubDate   string `json:"pubdate"`
	Series    string `json:"series,omitempty"`
	Volume    string `json:"volume,omitempty"`
	Cover     string `json:"cover,omitempty"`
}

// Coverage holds the count of ISBNs covered by OpenBD.
type Coverage struct {
	Count int `json:"count"`
}

// --- wire types ---

type wireTextContent struct {
	TextType struct {
		Content string `json:"content"`
	} `json:"TextType"`
	Text string `json:"Text"`
}

type wireTitleElement struct {
	TitleText struct {
		Content string `json:"content"`
	} `json:"TitleText"`
}

type wireBook struct {
	Summary struct {
		ISBN      string `json:"isbn"`
		Title     string `json:"title"`
		Volume    string `json:"volume"`
		Series    string `json:"series"`
		Publisher string `json:"publisher"`
		PubDate   string `json:"pubdate"`
		Cover     string `json:"cover"`
		Author    string `json:"author"`
	} `json:"summary"`
	Onix struct {
		DescriptiveDetail struct {
			TitleDetail struct {
				TitleElement []wireTitleElement `json:"TitleElement"`
			} `json:"TitleDetail"`
		} `json:"DescriptiveDetail"`
		CollateralDetail struct {
			TextContent []wireTextContent `json:"TextContent"`
		} `json:"CollateralDetail"`
	} `json:"onix"`
}

// GetBooks fetches books by one or more ISBNs (comma-separated).
// The API returns null for ISBNs not found; those are skipped.
func (c *Client) GetBooks(ctx context.Context, isbns string) ([]Book, error) {
	rawURL := c.cfg.BaseURL + "/v1/get?isbn=" + isbns
	body, err := c.get(ctx, rawURL)
	if err != nil {
		return nil, err
	}

	// The API returns a JSON array of objects or nulls.
	var raw []json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse books: %w", err)
	}

	var out []Book
	for _, r := range raw {
		if string(r) == "null" {
			continue
		}
		var w wireBook
		if err := json.Unmarshal(r, &w); err != nil {
			continue
		}
		out = append(out, flattenBook(w))
	}
	return out, nil
}

// GetCoverage returns the count of ISBNs covered by OpenBD.
func (c *Client) GetCoverage(ctx context.Context) (*Coverage, error) {
	rawURL := c.cfg.BaseURL + "/v1/coverage"
	body, err := c.get(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	var isbns []string
	if err := json.Unmarshal(body, &isbns); err != nil {
		return nil, fmt.Errorf("parse coverage: %w", err)
	}
	return &Coverage{Count: len(isbns)}, nil
}

// get fetches a URL and returns the body, pacing and retrying as configured.
func (c *Client) get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.cfg.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, rawURL)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", rawURL, lastErr)
}

func (c *Client) do(ctx context.Context, rawURL string) (body []byte, retry bool, err error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

func (c *Client) pace() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cfg.Rate <= 0 {
		return
	}
	if wait := c.cfg.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

// --- flatten helpers ---

func flattenBook(w wireBook) Book {
	title := w.Summary.Title
	// Use the long title from ONIX if available.
	if elems := w.Onix.DescriptiveDetail.TitleDetail.TitleElement; len(elems) > 0 {
		if t := strings.TrimSpace(elems[0].TitleText.Content); t != "" {
			title = t
		}
	}
	return Book{
		ISBN:      w.Summary.ISBN,
		Title:     title,
		Author:    w.Summary.Author,
		Publisher: w.Summary.Publisher,
		PubDate:   w.Summary.PubDate,
		Series:    w.Summary.Series,
		Volume:    w.Summary.Volume,
		Cover:     w.Summary.Cover,
	}
}
