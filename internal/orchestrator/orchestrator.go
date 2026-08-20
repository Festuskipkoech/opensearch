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

type Request struct {
	Query string
	AgentIntent string
	MaxResults int
}

type classifierClient interface {
	Classify(ctx context.Context, query string) (classifier.Intent, error)
	AgentIntent(class string) (classifier.Intent, error)
}

type searxngClient interface {
	Search(ctx context.Context, query string, engines []string) []merger.Result
}

type engineRouter interface {
	Select(intent string) []string
	SelectHedge(intentA, intentB string) []string
}

type cacheStore interface {
	Get(ctx context.Context, key string) (types.Response, error)
	Set(ctx context.Context, key string, value types.Response, ttl time.Duration) error
}

type crawlerDecider interface {
	Decide(ctx context.Context, req crawler.Request) crawler.Decision
}

type Orchestrator struct {
	cache cacheStore
	clf classifierClient
	router engineRouter
	searxng searxngClient
	merger func([]merger.EngineResults) []merger.Result
	crawler crawlerDecider
	ttlFor func(intent string) int
}

func New(
	c cacheStore,
	clf classifierClient,
	r engineRouter,
	s searxngClient,
	cr crawlerDecider,
	ttlFor func(string) int,
) *Orchestrator {
	return &Orchestrator{
		cache: c,
		clf: clf,
		router: r,
		searxng: s,
		merger: merger.Merge,
		crawler: cr,
		ttlFor: ttlFor,
	}
}

func (o *Orchestrator) Search(ctx context.Context, req Request) (types.Response, error) {
	start := time.Now()

	if req.Query == "" {
		return types.Response{}, fmt.Errorf("query must not be empty")
	}

	cacheKey := cache.Key(req.Query, req.AgentIntent)
	if cached, err := o.cache.Get(ctx, cacheKey); err == nil {
		cached.Cached = true
		cached.LatencyMS = time.Since(start).Milliseconds()
		return cached, nil
	}

	intent, err := o.resolveIntent(ctx, req)
	if err != nil {
		return types.Response{}, fmt.Errorf("classify: %w", err)
	}

	var engines []string
	if intent.Uncertain {
		engines = o.router.SelectHedge(intent.Class, intent.RunnerUp)
	} else {
		engines = o.router.Select(intent.Class)
	}

	raw := o.searxng.Search(ctx, req.Query, engines)
	results := o.merger(toEngineResults(raw))

	decision := o.crawler.Decide(ctx, crawler.Request{
		Query: req.Query,
		Intent: intent.Class,
		Results: results,
	})

	if !decision.Sufficient && len(decision.EnrichedResults) > 0 {
		results = decision.EnrichedResults
	}

	if req.MaxResults > 0 && len(results) > req.MaxResults {
		results = results[:req.MaxResults]
	}

	resp := types.Response{
		Query: req.Query,
		Intent: intent.Class,
		Uncertain: intent.Uncertain,
		Results: results,
		Cached: false,
		LatencyMS: time.Since(start).Milliseconds(),
	}

	ttl := time.Duration(o.ttlFor(intent.Class)) * time.Second
	go func() {
		if err := o.cache.Set(context.Background(), cacheKey, resp, ttl); err != nil {
			slog.Error("cache write failed", "key", cacheKey, "error", err)
		}
	}()

	return resp, nil
}

var ErrInvalidIntent = fmt.Errorf("invalid intent class")

func (o *Orchestrator) resolveIntent(ctx context.Context, req Request) (classifier.Intent, error) {
	if req.AgentIntent != "" {
		intent, err := o.clf.AgentIntent(req.AgentIntent)
		if err != nil {
			return classifier.Intent{}, fmt.Errorf("%w: %w", ErrInvalidIntent, err)
		}
		return intent, nil
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