package crawler

import (
	"testing"

	"opensearch/internal/merger"
)

func TestThresholdForKnownIntents(t *testing.T) {
	cases := []struct {
		intent    string
		wantAbove float64
		wantBelow float64
	}{
		{"research", 0.44, 0.46},
		{"factual", 0.69, 0.71},
		{"code", 0.64, 0.66},
		{"news", 0.59, 0.61},
		{"commercial", 0.59, 0.61},
		{"general", 0.64, 0.66},
	}
	for _, tc := range cases {
		t.Run(tc.intent, func(t *testing.T) {
			got := thresholdFor(tc.intent)
			if got <= tc.wantAbove || got >= tc.wantBelow {
				t.Errorf("intent %q: threshold %.2f not in range (%.2f, %.2f)", tc.intent, got, tc.wantAbove, tc.wantBelow)
			}
		})
	}
}

func TestThresholdForUnknownIntentUsesDefault(t *testing.T) {
	got := thresholdFor("unknown")
	if got != 0.65 {
		t.Errorf("expected default threshold 0.65, got %.2f", got)
	}
}

func TestSnippetDensityBands(t *testing.T) {
	cases := []struct {
		snippet string
		want    float64
	}{
		{"", 0.1},
		{"short", 0.1},
		{"this is a medium length snippet that has some content in it yes", 0.4},
		{"this is a longer snippet with more than one hundred and fifty characters in it to test the medium band threshold properly here", 0.7},
		{"this is a long snippet with more than three hundred characters in it to test the high density band threshold properly here we go adding more text to make sure we exceed the three hundred character mark for this test case", 1.0},
	}
	for _, tc := range cases {
		got := snippetDensity(tc.snippet)
		if got != tc.want {
			t.Errorf("snippet len %d: expected %.1f, got %.1f", len(tc.snippet), tc.want, got)
		}
	}
}

func TestSourceAuthorityKnownDomains(t *testing.T) {
	authoritative := []string{
		"wikipedia.org",
		"github.com",
		"stackoverflow.com",
		"pkg.go.dev",
		"arxiv.org",
	}
	for _, domain := range authoritative {
		got := sourceAuthority(domain)
		if got != 1.0 {
			t.Errorf("domain %q: expected authority 1.0, got %.1f", domain, got)
		}
	}
}

func TestSourceAuthorityUnknownDomain(t *testing.T) {
	got := sourceAuthority("random-blog.com")
	if got != 0.5 {
		t.Errorf("expected 0.5 for unknown domain, got %.1f", got)
	}
}

func TestTopResultsReturnsMaxThree(t *testing.T) {
	req := Request{
		Results: []merger.Result{
			{URL: "https://a.com"},
			{URL: "https://b.com"},
			{URL: "https://c.com"},
			{URL: "https://d.com"},
			{URL: "https://e.com"},
		},
	}
	top := topResults(req)
	if len(top) != maxCrawlURLs {
		t.Errorf("expected %d results, got %d", maxCrawlURLs, len(top))
	}
}

func TestTopResultsReturnsAllWhenFewerThanMax(t *testing.T) {
	req := Request{
		Results: []merger.Result{
			{URL: "https://a.com"},
			{URL: "https://b.com"},
		},
	}
	top := topResults(req)
	if len(top) != 2 {
		t.Errorf("expected 2 results, got %d", len(top))
	}
}

func TestTopResultsEmptyReturnsEmpty(t *testing.T) {
	req := Request{Results: nil}
	top := topResults(req)
	if len(top) != 0 {
		t.Errorf("expected empty, got %d", len(top))
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