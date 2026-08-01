package searxng

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"opensearch/internal/merger"
)

const (
	// earlyExitMinResults is the minimum result count that triggers early exit
	// when the top two engines have both responded
	earlyExitMinResults = 5
	requestTimeout = 5 *time.Second
)

// Client fires parallel search queries to our self-hosted SearXNG instance.
type Client struct {
	baseURL string
	httpClient *http.Client
	minResults int
}

// New creates a Client pointed at the SearXNG instance URL.
func New(baseURL string, minResults int) *Client {
	return  &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: requestTimeout},
		minResults: minResults,
	}
}

// Search fires one goroutine per engine, collects results as they arrive,
// and applies early exit when conditions are met.
// The context controls cancellation — if the caller cancels, all in-flight
// requests stop immediately.
func (c *Client) Search(ctx context.Context, query string, engines []string) []merger.Result {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	results := make(chan merger.EngineResults, len(engines))

	var wg sync.WaitGroup
	for _, engine := range engines {
		wg.Add(1)
		go func(eng string)  {
			defer wg.Done()			
			res, err := c.queryEngine(ctx, query, eng)
			results <- merger.EngineResults{Engine: eng, Results: res, Err: err}
		}(engine)
	}
	// close results channel when all goroutines finish
	go func ()  {
		wg.Wait()		
		close(results)
	}()
	return c.collect(results, cancel, len(engines))
}

// collect reads from the results channel and applies early exit logic.
// Early exit fires when two engines have responded and combined result
// count exceeds the minimum threshold — remaining requests are cancelled.
func (c *Client) collect(
	results <-chan merger.EngineResults,
	cancel context.CancelFunc,
	total int,
) []merger.Result {
	var combined []merger.Result
	responded := 0

	for r := range results {
		responded++
		if r.Err == nil {
			combined = append(combined, r.Results...)
		}
		// early exit — two engines responded and we have enough results
		if responded >= 2 && len(combined) >= c.minResults {
			cancel()
			// early exit — two engines responded and we have enough results
			for remaining := range results {
				if remaining.Err == nil {
					combined = append(combined, remaining.Results...)
				}
			}
			break
		}
		if responded == total {
			break
		}
	}
	return combined
}

// queryEngine sends one search request to SearXNG for a specific engine.
func (c *Client) queryEngine(ctx context.Context, query, engine string) ([]merger.Result, error) {
	endpoint := fmt.Sprintf("%s/search", c.baseURL)

	params := url.Values{}
	params.Set("q", query)
	params.Set("engines", engine)
	params.Set("format", "json")
	
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return  nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("searxng request engine=%s: %w", engine, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("searxng engine=%s status=%d", engine, resp.StatusCode)
	}

	var sr searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, fmt.Errorf("decode response engine=%s: %w", engine, err)
	}
	
	return toResults(sr.Results, engine), nil
}

// toResults converts SearXNG response items to pipeline Result types.
func toResults(items []searchResult, engine string) []merger.Result {
	results := make([]merger.Result, 0, len(items))
	for _, item := range items {
		if item.URL == "" {
			continue
		}

		results = append(results, merger.Result{
			URL: item.URL,
			Title: item.Title,
			Snippet: item.Content,
			Domain: extractDomain(item.URL),
			Engine: engine,
			Score: item.Score,
		})
	}
	return results
}

// extractDomain pulls the hostname from a URL for domain-level deduplication.
func extractDomain(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return rawURL
	}
	// strip www. prefix so www.example.com and example.com are the same domain
	return strings.TrimPrefix(u.Host, "www.")
}
