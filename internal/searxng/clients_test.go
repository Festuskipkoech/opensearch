package searxng

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// mockSearXNG is a test HTTP server returning controlled search responses.
// requests uses atomic.Int64 because the HTTP server handles concurrent
// requests in separate goroutines — a plain int would be a data race.
type mockSearXNG struct {
	results  []searchResult
	requests atomic.Int64
	delay    time.Duration
}

func (m *mockSearXNG) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m.requests.Add(1)
		if m.delay > 0 {
			time.Sleep(m.delay)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(searchResponse{Results: m.results})
	}
}

func newTestClient(t *testing.T, mock *mockSearXNG, minResults int) *Client {
	t.Helper()
	srv := httptest.NewServer(mock.handler())
	t.Cleanup(srv.Close)
	return New(srv.URL, minResults)
}

func TestSearchFiresOneRequestPerEngine(t *testing.T) {
	mock := &mockSearXNG{
		results: []searchResult{
			{URL: "https://example.com", Title: "Example", Content: "snippet", Engine: "brave", Score: 0.9},
		},
	}
	client := newTestClient(t, mock, 1)

	engines := []string{"brave", "ddg", "mojeek"}
	client.Search(context.Background(), "goroutine scheduling", engines)

	// early exit may fire after 2 responses — at minimum 2 requests must be made
	if mock.requests.Load() < 2 {
		t.Errorf("expected at least 2 requests for 3 engines, got %d", mock.requests.Load())
	}
}

func TestSearchReturnsResults(t *testing.T) {
	mock := &mockSearXNG{
		results: []searchResult{
			{URL: "https://go.dev", Title: "Go", Content: "snippet one", Engine: "brave", Score: 0.9},
			{URL: "https://pkg.go.dev", Title: "Pkg", Content: "snippet two", Engine: "brave", Score: 0.8},
		},
	}
	client := newTestClient(t, mock, 1)

	results := client.Search(context.Background(), "golang", []string{"brave"})
	if len(results) == 0 {
		t.Error("expected results, got none")
	}
}

func TestSearchSkipsEmptyURLs(t *testing.T) {
	mock := &mockSearXNG{
		results: []searchResult{
			{URL: "", Title: "No URL", Content: "snippet", Engine: "brave", Score: 0.9},
			{URL: "https://go.dev", Title: "Go", Content: "snippet", Engine: "brave", Score: 0.8},
		},
	}
	client := newTestClient(t, mock, 1)

	results := client.Search(context.Background(), "golang", []string{"brave"})
	for _, r := range results {
		if r.URL == "" {
			t.Error("result with empty URL must not appear in output")
		}
	}
}

func TestSearchExtractsDomain(t *testing.T) {
	mock := &mockSearXNG{
		results: []searchResult{
			{URL: "https://www.example.com/page", Title: "Page", Content: "snippet", Engine: "brave", Score: 0.9},
		},
	}
	client := newTestClient(t, mock, 1)

	results := client.Search(context.Background(), "test", []string{"brave"})
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}
	if results[0].Domain != "example.com" {
		t.Errorf("expected domain %q, got %q", "example.com", results[0].Domain)
	}
}

func TestSearchRespectsContextCancellation(t *testing.T) {
	mock := &mockSearXNG{
		results: []searchResult{
			{URL: "https://example.com", Title: "Example", Content: "snippet", Engine: "brave", Score: 0.9},
		},
		delay: 500 * time.Millisecond,
	}
	client := newTestClient(t, mock, 100)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	client.Search(ctx, "test", []string{"brave", "ddg", "mojeek"})
	elapsed := time.Since(start)

	if elapsed > 300*time.Millisecond {
		t.Errorf("search did not respect context cancellation, took %v", elapsed)
	}
}

func TestExtractDomain(t *testing.T) {
	cases := []struct {
		rawURL   string
		expected string
	}{
		{"https://www.example.com/page", "example.com"},
		{"https://go.dev/doc", "go.dev"},
		{"https://pkg.go.dev/net/http", "pkg.go.dev"},
		{"not-a-url", "not-a-url"},
	}

	for _, tc := range cases {
		t.Run(tc.rawURL, func(t *testing.T) {
			got := extractDomain(tc.rawURL)
			if got != tc.expected {
				t.Errorf("extractDomain(%q) = %q, want %q", tc.rawURL, got, tc.expected)
			}
		})
	}
}