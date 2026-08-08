package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"opensearch/internal/cache"
	"opensearch/internal/classifier"
	"opensearch/internal/crawler"
	"opensearch/internal/merger"
	"opensearch/internal/types"
)

// Request is what the API layer passes in after parsing the HTTP body.
type Request struct {
	Query string
	AgentIntent string
	MaxResults  int
}

// classifierClient is the interface the orchestrator needs from the classifier.
// *classifier.Classifier satisfies this interface.
type classifierClient interface {
	Classify(ctx context.Context, query string) (classifier.Intent, error)
	AgentIntent(class string) (classifier.Intent, error)
}

// searxngClient is the interface the orchestrator needs from the searxng client.
type searxngClient interface {
	Search(ctx context.Context, query string, engines []string) []merger.Result
}

// engineRouter is the interface the orchestrator needs from the router.
type engineRouter interface {
	Select(intent string) []string
	SelectHedge(intentA, intentB string) []string
}

// cacheStore is the interface the orchestrator needs from the cache.
type cacheStore interface {
	Get(ctx context.Context, key string) (types.Response, error)
	Set(ctx context.Context, key string, value types.Response, ttl time.Duration) error
}

// Orchestrator coordinates the full search pipeline.
type Orchestrator struct {
	cache   cacheStore
	clf     classifierClient
	router  engineRouter
	searxng searxngClient
	merger  func([]merger.EngineResults) []merger.Result
	crawler func(crawler.Request) crawler.Decision
	ttlFor  func(intent string) int
}

// New wires all dependencies into the Orchestrator.
func New(
	c cacheStore,
	clf classifierClient,
	r engineRouter,
	s searxngClient,
	ttlFor func(string) int,
) *Orchestrator {
	return &Orchestrator{
		cache: c,
		clf: clf,
		router:  r,
		searxng: s,
		merger: merger.Merge,
		crawler: crawler.Decide,
		ttlFor: ttlFor,
	}
}

// Search runs the full pipeline for one request.
func (o *Orchestrator) Search(ctx context.Context, req Request) (types.Response, error) {
	start := time.Now()

	if req.Query == "" {
		return types.Response{}, fmt.Errorf("query must not be empty")
	}

	// cache check — return immediately on hit
	cacheKey := cache.Key(req.Query, req.AgentIntent)
	if cached, err := o.cache.Get(ctx, cacheKey); err == nil {
		cached.Cached = true
		cached.LatencyMS = time.Since(start).Milliseconds()
		return cached, nil
	}

	// classify intent
	intent, err := o.resolveIntent(ctx, req)
	if err != nil {
		return types.Response{}, fmt.Errorf("classify: %w", err)
	}

	// select engines
	var engines []string
	if intent.Uncertain {
		engines = o.router.SelectHedge(intent.Class, intent.RunnerUp)
	} else {
		engines = o.router.Select(intent.Class)
	}

	// fan out to SearXNG
	raw := o.searxng.Search(ctx, req.Query, engines)

	// merge results
	results := o.merger(toEngineResults(raw))

	// crawl decision — stub returns sufficient=true in Phase 1
	decision := o.crawler(crawler.Request{
		Query:   req.Query,
		Intent:  intent.Class,
		Results: results,
	})
	_ = decision // Phase 2 uses this to invoke Spider-rs

	// cap results
	if req.MaxResults > 0 && len(results) > req.MaxResults {
		results = results[:req.MaxResults]
	}

	resp := types.Response{
		Query: req.Query,
		Intent: intent.Class,
		Uncertain: intent.Uncertain,
		Results:  results,
		Cached: false,
		LatencyMS: time.Since(start).Milliseconds(),
	}

	// cache write is fire-and-forget — agent does not wait for it
	ttl := time.Duration(o.ttlFor(intent.Class)) * time.Second
	go func() {
		if err := o.cache.Set(context.Background(), cacheKey, resp, ttl); err != nil {
			slog.Error("cache write failed", "key", cacheKey, "error", err)
		}
	}()

	return resp, nil
}

func (o *Orchestrator) resolveIntent(ctx context.Context, req Request) (classifier.Intent, error) {
	if req.AgentIntent != "" {
		return o.clf.AgentIntent(req.AgentIntent)
	}
	return o.clf.Classify(ctx, req.Query)
}

func toEngineResults(results []merger.Result) []merger.EngineResults {
	byEngine := make(map[string][]merger.Result)
	for _, r := range results {
		byEngine[r.Engine] = append(byEngine[r.Engine], r)
	}
	out := make([]merger.EngineResults, 0, len(byEngine))
	for engine, res := range byEngine {
		out = append(out, merger.EngineResults{Engine: engine, Results: res})
	}
	return out
}