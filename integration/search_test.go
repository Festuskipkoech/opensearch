//go:build integration

package integration
 
import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"
 
	"opensearch/internal/merger"
)
// baseURL reads the server address from env or falls back to localhost.
// In CI this is set to the Docker Compose service address.
func baseURL() string {
	if addr := os.Getenv("OPENSEARCH_ADDR"); addr != "" {
		return "http://" + addr
	}
	return "http://localhost:8080"
}

// searchRequest mirrors the API request body.
type searchRequest struct {
	Query string `json:"query"`
	Intent string `json:"intent,omitempty"`
	MaxResults int `json:"max_results,omitempty"`
}

// searchResponse mirrors the API response body.
type searchResponse struct {
	Query string `json:"query"`
	Intent string `json:"intent"`
	Uncertain bool `json:"uncertain"`
	Results []merger.Result `json:"results"`
	Cached bool `json:"cached"`
	LatencyMS int64 `json:"latency_ms"`
}

func doSearch(t *testing.T, req searchRequest) searchResponse {
	t.Helper()

	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	resp, err := http.Post(
		baseURL()+"/search",
		"application/json",
		bytes.NewReader(body),
	)
	if err !=nil {
		t.Fatalf("POST /search: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var sr searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	return sr
}

func TestHealthEndpointReturnsOk(t *testing.T) {
	resp, err := http.Get(baseURL() + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestSearchReturnsResults(t *testing.T) {
    resp := doSearch(t, searchRequest{
        Query: "golang goroutine scheduling",
        MaxResults: 5,
    })

    if len(resp.Results) == 0 {
        t.Error("expected at least one result, got none")
    }
    if resp.Query != "golang goroutine scheduling" {
        t.Errorf("response query mismatch: got %q", resp.Query)
    }
    if resp.Intent == "" {
        t.Error("expected intent to be set in response")
    }
}

func TestSearchResponseNotCachedOnFirstCall(t *testing.T) {
	// use a unique query so we are certain it is not in cache
	query := fmt.Sprintf("unique integration test query %d", time.Now().UnixNano())

	resp := doSearch(t, searchRequest{Query: query, MaxResults: 3})
	if resp.Cached {
		t.Error("first call must not be marked as cached")
	}
}

func TestSearchCacheHitOnSecondIdenticalCall(t *testing.T) {
	query := fmt.Sprintf("cache test query %d", time.Now().UnixNano())

	// first call — populates cache
	doSearch(t, searchRequest{Query: query, MaxResults: 3})
	
	// give fire-and-forget cache write time to complete
	time.Sleep(100 * time.Millisecond)
 
	// second call — must be a cache hit
	resp := doSearch(t, searchRequest{Query: query, MaxResults: 3})
	if !resp.Cached {
		t.Error("second identical call must be served from cache")
	}
}

func TestSearchCacheHitFasterThanMiss(t *testing.T) {
	query := fmt.Sprintf("latency test query %d", time.Now().UnixNano())
 
	// first call — cache miss
	start := time.Now()
	doSearch(t, searchRequest{Query: query, MaxResults: 3})
	missLatency := time.Since(start)
 
	time.Sleep(100 * time.Millisecond)
 
	// second call — cache hit
	start = time.Now()
	doSearch(t, searchRequest{Query: query, MaxResults: 3})
	hitLatency := time.Since(start)
 
	if hitLatency >= missLatency {
		t.Errorf("cache hit (%v) must be faster than cache miss (%v)", hitLatency, missLatency)
	}
}
 
func TestSearchWithAgentProvidedIntent(t *testing.T) {
	resp := doSearch(t, searchRequest{
		Query: "latest news today",
		Intent: "news",
		MaxResults: 5,
	})
 
	if resp.Intent != "news" {
		t.Errorf("expected intent %q from agent, got %q", "news", resp.Intent)
	}
}
 
func TestSearchWithInvalidAgentIntentReturns400(t *testing.T) {
	body, _ := json.Marshal(searchRequest{
		Query: "some query",
		Intent: "invalid_class",
	})
 
	resp, err := http.Post(
		baseURL()+"/search",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("POST /search: %v", err)
	}
	defer resp.Body.Close()
 
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected error status for invalid intent, got %d", resp.StatusCode)
	}
}
 
func TestSearchEmptyQueryReturns400(t *testing.T) {
	body, _ := json.Marshal(searchRequest{Query: ""})
 
	resp, err := http.Post(
		baseURL()+"/search",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("POST /search: %v", err)
	}
	defer resp.Body.Close()
 
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for empty query, got %d", resp.StatusCode)
	}
}
 
func TestSearchResultsHaveRequiredFields(t *testing.T) {
	resp := doSearch(t, searchRequest{
		Query: "what is a goroutine",
		MaxResults: 3,
	})
 
	for i, r := range resp.Results {
		if r.URL == "" {
			t.Errorf("result %d has empty URL", i)
		}
		if r.Title == "" {
			t.Errorf("result %d has empty Title", i)
		}
		if r.Domain == "" {
			t.Errorf("result %d has empty Domain", i)
		}
	}
}
 
func TestSearchSnippetQueryUnder800ms(t *testing.T) {
	start := time.Now()
	doSearch(t, searchRequest{
		Query: "capital of kenya",
		MaxResults: 5,
	})
	elapsed := time.Since(start)
 
	if elapsed > 800*time.Millisecond {
		t.Errorf("snippet query took %v, must be under 800ms", elapsed)
	}
}
 
