package openbd_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tamnd/openbd-cli/openbd"
)

func newTestClient(ts *httptest.Server) *openbd.Client {
	cfg := openbd.DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.Rate = 0
	return openbd.NewClient(cfg)
}

// TestGetBookSingle checks that a single ISBN returns one Book.
func TestGetBookSingle(t *testing.T) {
	fixture := []any{
		map[string]any{
			"summary": map[string]any{
				"isbn":      "9784873115658",
				"title":     "Go言語プログラミング",
				"author":    "Alan A. A. Donovan",
				"publisher": "オライリー・ジャパン",
				"pubdate":   "20160120",
				"cover":     "https://cover.openbd.jp/9784873115658.jpg",
				"series":    "",
				"volume":    "",
			},
			"onix": map[string]any{},
		},
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := json.Marshal(fixture)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	books, err := c.GetBooks(context.Background(), "9784873115658")
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 1 {
		t.Fatalf("got %d books, want 1", len(books))
	}
	b := books[0]
	if b.ISBN != "9784873115658" {
		t.Errorf("ISBN = %q, want 9784873115658", b.ISBN)
	}
	if b.Title != "Go言語プログラミング" {
		t.Errorf("Title = %q", b.Title)
	}
	if b.Author != "Alan A. A. Donovan" {
		t.Errorf("Author = %q", b.Author)
	}
	if b.Publisher != "オライリー・ジャパン" {
		t.Errorf("Publisher = %q", b.Publisher)
	}
}

// TestGetBookMultiple checks that a comma-separated list returns multiple books.
func TestGetBookMultiple(t *testing.T) {
	fixture := []any{
		map[string]any{
			"summary": map[string]any{
				"isbn":  "9784873115658",
				"title": "Go言語プログラミング",
			},
			"onix": map[string]any{},
		},
		map[string]any{
			"summary": map[string]any{
				"isbn":  "9784873117835",
				"title": "Goプログラミング実践入門",
			},
			"onix": map[string]any{},
		},
	}
	var gotQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		b, _ := json.Marshal(fixture)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	books, err := c.GetBooks(context.Background(), "9784873115658,9784873117835")
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 2 {
		t.Fatalf("got %d books, want 2", len(books))
	}
	if !strings.Contains(gotQuery, "9784873115658") {
		t.Errorf("query %q should contain first ISBN", gotQuery)
	}
}

// TestGetBookNullSkipped checks that null entries from the API are skipped.
func TestGetBookNullSkipped(t *testing.T) {
	// API returns array with one real book and one null.
	raw := `[{"summary":{"isbn":"9784873115658","title":"Go言語プログラミング","author":"","publisher":"","pubdate":"","cover":"","series":"","volume":""},"onix":{}},null]`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(raw))
	}))
	defer ts.Close()

	c := newTestClient(ts)
	books, err := c.GetBooks(context.Background(), "9784873115658,0000000000000")
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 1 {
		t.Fatalf("got %d books, want 1 (null entry should be skipped)", len(books))
	}
	if books[0].ISBN != "9784873115658" {
		t.Errorf("ISBN = %q, want 9784873115658", books[0].ISBN)
	}
}

// TestGetCoverage checks that coverage returns a count.
func TestGetCoverage(t *testing.T) {
	// Return a small array of ISBN strings.
	fixture := []string{"9784873115658", "9784873117835", "9784798142418"}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := json.Marshal(fixture)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	}))
	defer ts.Close()

	c := newTestClient(ts)
	cov, err := c.GetCoverage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cov.Count != 3 {
		t.Errorf("Count = %d, want 3", cov.Count)
	}
}

// TestRetryOn503 checks that the client retries on 503 and succeeds.
func TestRetryOn503(t *testing.T) {
	var hits int
	fixture := []string{"9784873115658", "9784873117835"}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		b, _ := json.Marshal(fixture)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	}))
	defer ts.Close()

	cfg := openbd.DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.Rate = 0
	cfg.Retries = 5
	c := openbd.NewClient(cfg)

	start := time.Now()
	cov, err := c.GetCoverage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cov.Count != 2 {
		t.Errorf("Count = %d, want 2", cov.Count)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}
