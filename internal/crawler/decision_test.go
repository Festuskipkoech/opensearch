package crawler

import (
	"testing"
	"opensearch/internal/merger"
)

func TestDecideAlwaysSufficientInPhase1(t *testing.T) {
	req := Request{
		Query:  "goroutine scheduling",
		Intent: "code",
		Results: []merger.Result{
			{URL: "https://go.dev", Snippet: "goroutines are lightweight"},
		},
	}

	decision := Decide(req)
	if !decision.Sufficient {
		t.Error("Phase 1 stub must always return sufficient=true")
	}
}

func TestDecideSufficientForEmptyResults(t *testing.T) {
	req := Request{
		Query:   "anything",
		Intent:  "general",
		Results: nil,
	}

	decision := Decide(req)
	if !decision.Sufficient {
		t.Error("Phase 1 stub must return sufficient=true even for empty results")
	}
}

func TestTopURLsReturnsMaxThree(t *testing.T) {
	req := Request{
		Results: []merger.Result{
			{URL: "https://a.com"},
			{URL: "https://b.com"},
			{URL: "https://c.com"},
			{URL: "https://d.com"},
			{URL: "https://e.com"},
		},
	}

	urls := topURLs(req)
	if len(urls) != maxCrawlURLs {
		t.Errorf("expected %d URLs, got %d", maxCrawlURLs, len(urls))
	}
}

func TestTopURLsReturnsAllWhenFewerThanMax(t *testing.T) {
	req := Request{
		Results: []merger.Result{
			{URL: "https://a.com"},
			{URL: "https://b.com"},
		},
	}

	urls := topURLs(req)
	if len(urls) != 2 {
		t.Errorf("expected 2 URLs when only 2 results exist, got %d", len(urls))
	}
}

func TestTopURLsEmptyResultsReturnsEmpty(t *testing.T) {
	req := Request{Results: nil}
	urls := topURLs(req)
	if len(urls) != 0 {
		t.Errorf("expected empty URL list for empty results, got %d", len(urls))
	}
}

func TestTopURLsPreservesOrder(t *testing.T) {
	req := Request{
		Results: []merger.Result{
			{URL: "https://first.com"},
			{URL: "https://second.com"},
			{URL: "https://third.com"},
		},
	}

	urls := topURLs(req)
	expected := []string{"https://first.com", "https://second.com", "https://third.com"}
	for i, u := range urls {
		if u != expected[i] {
			t.Errorf("position %d: expected %q, got %q", i, expected[i], u)
		}
	}
}