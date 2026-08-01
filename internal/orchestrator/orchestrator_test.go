package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"opensearch/internal/cache"
	"opensearch/internal/classifier"
	"opensearch/internal/merger"
	"opensearch/internal/types"
)

// --- fakes ---
// fakeCache controls cache hits and misses and records writes.
type fakeCache struct {
	stored map[string]types.Response
	writes int
	done chan struct{}
}

func newFakeCache() *fakeCache {
    return &fakeCache{
        stored: make(map[string]types.Response),
        done: make(chan struct{}),
    }
}
func (f *fakeCache) Get(_ context.Context, key string) (types.Response, error) {
	if r, ok := f.stored[key]; ok {
		return r, nil
	}
	return types.Response{}, cache.ErrCacheMiss
}

func (f *fakeCache) Set(_ context.Context, key string, r types.Response, _ time.Duration) error {
    f.stored[key] = r
    f.writes++
    close(f.done)
    return nil
}

// fakeClassifier returns controlled Intent values without gRPC.
type fakeClassifier struct {
	intent classifier.Intent
	err    error
}

func (f *fakeClassifier) Classify(_ context.Context, _ string) (classifier.Intent, error) {
	return f.intent, f.err
}

func (f *fakeClassifier) AgentIntent(class string) (classifier.Intent, error) {
	if class == "invalid" {
		return classifier.Intent{}, errors.New("unknown intent class")
	}
	return classifier.Intent{Class: class, Confidence: 1.0, Uncertain: false}, nil
}

// fakeSearXNG returns controlled results without HTTP calls.
type fakeSearXNG struct {
	results []merger.Result
}

func (f *fakeSearXNG) Search(_ context.Context, _ string, _ []string) []merger.Result {
	return f.results
}

// fakeRouter returns fixed engine lists without affinity scoring.
type fakeRouter struct{}

func (f *fakeRouter) Select(_ string) []string        { return []string{"brave", "ddg"} }
func (f *fakeRouter) SelectHedge(_, _ string) []string { return []string{"brave", "ddg", "mojeek"} }

// --- builder ---
func buildOrchestrator(fc *fakeCache, clf *fakeClassifier, results []merger.Result) *Orchestrator {
	return New(fc, clf, &fakeRouter{}, &fakeSearXNG{results: results}, func(_ string) int { return 3600 })
}

// --- tests ---
func TestSearchEmptyQueryReturnsError(t *testing.T) {
	o := buildOrchestrator(newFakeCache(), &fakeClassifier{}, nil)

	_, err := o.Search(context.Background(), Request{Query: ""})
	if err == nil {
		t.Error("expected error for empty query, got nil")
	}
}

func TestSearchCacheHitReturnsCachedResponse(t *testing.T) {
	fc := newFakeCache()
	cached := types.Response{Query: "goroutine", Intent: "code", Cached: false}

	// pre-populate cache with the key the orchestrator will look up
	key := cache.Key("goroutine", "")
	fc.stored[key] = cached

	clf := &fakeClassifier{
		intent: classifier.Intent{Class: "code", Confidence: 0.95},
	}
	o := buildOrchestrator(fc, clf, nil)

	resp, err := o.Search(context.Background(), Request{Query: "goroutine"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Cached {
		t.Error("expected Cached=true on cache hit")
	}
}

func TestSearchCacheHitSkipsClassifier(t *testing.T) {
	fc := newFakeCache()
	key := cache.Key("goroutine", "")
	fc.stored[key] = types.Response{Query: "goroutine", Intent: "code"}

	// classifier returns an error — if called the test will fail
	clf := &fakeClassifier{err: errors.New("classifier must not be called on cache hit")}
	o := buildOrchestrator(fc, clf, nil)

	_, err := o.Search(context.Background(), Request{Query: "goroutine"})
	if err != nil {
		t.Errorf("cache hit must not call classifier, got error: %v", err)
	}
}

func TestSearchCacheMissCallsClassifier(t *testing.T) {
	called := false
	clf := &fakeClassifier{
		intent: classifier.Intent{Class: "code", Confidence: 0.95},
	}

	// wrap to detect call
	original := clf.intent
	clf.intent = original
	_ = called // suppress unused warning

	o := buildOrchestrator(newFakeCache(), clf, []merger.Result{
		{URL: "https://go.dev", Domain: "go.dev", Engine: "brave", Score: 0.9},
	})

	resp, err := o.Search(context.Background(), Request{Query: "goroutine scheduling"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Intent != "code" {
		t.Errorf("expected intent %q, got %q", "code", resp.Intent)
	}
}

func TestSearchAgentIntentSkipsClassifier(t *testing.T) {
	// classifier returns error — if called the test will surface it
	clf := &fakeClassifier{err: errors.New("classifier must not be called when agent provides intent")}
	o := buildOrchestrator(newFakeCache(), clf, []merger.Result{
		{URL: "https://go.dev", Domain: "go.dev", Engine: "brave", Score: 0.9},
	})

	resp, err := o.Search(context.Background(), Request{
		Query:       "goroutine scheduling",
		AgentIntent: "code",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Intent != "code" {
		t.Errorf("expected intent %q from agent, got %q", "code", resp.Intent)
	}
}

func TestSearchAgentIntentInvalidClassReturnsError(t *testing.T) {
	o := buildOrchestrator(newFakeCache(), &fakeClassifier{}, nil)

	_, err := o.Search(context.Background(), Request{
		Query:       "some query",
		AgentIntent: "invalid",
	})
	if err == nil {
		t.Error("expected error for invalid agent intent class, got nil")
	}
}

func TestSearchUncertainIntentUsesHedgeRouting(t *testing.T) {
	clf := &fakeClassifier{
		intent: classifier.Intent{
			Class:      "code",
			RunnerUp:   "research",
			Confidence: 0.55,
			Uncertain:  true,
		},
	}
	o := buildOrchestrator(newFakeCache(), clf, []merger.Result{
		{URL: "https://go.dev", Domain: "go.dev", Engine: "brave", Score: 0.9},
	})

	resp, err := o.Search(context.Background(), Request{Query: "ambiguous query"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Uncertain {
		t.Error("expected Uncertain=true in response when classifier confidence is low")
	}
}

func TestSearchMaxResultsCapsOutput(t *testing.T) {
	results := []merger.Result{
		{URL: "https://a.com", Domain: "a.com", Engine: "brave", Score: 0.9},
		{URL: "https://b.com", Domain: "b.com", Engine: "brave", Score: 0.8},
		{URL: "https://c.com", Domain: "c.com", Engine: "brave", Score: 0.7},
		{URL: "https://d.com", Domain: "d.com", Engine: "brave", Score: 0.6},
		{URL: "https://e.com", Domain: "e.com", Engine: "brave", Score: 0.5},
	}

	clf := &fakeClassifier{
		intent: classifier.Intent{Class: "general", Confidence: 0.90},
	}
	o := buildOrchestrator(newFakeCache(), clf, results)

	resp, err := o.Search(context.Background(), Request{
		Query:      "something",
		MaxResults: 3,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Results) > 3 {
		t.Errorf("expected at most 3 results, got %d", len(resp.Results))
	}
}

func TestSearchWritesToCacheAfterMiss(t *testing.T) {
	fc := newFakeCache()
	clf := &fakeClassifier{
		intent: classifier.Intent{Class: "factual", Confidence: 0.92},
	}
	o := buildOrchestrator(fc, clf, []merger.Result{
		{URL: "https://go.dev", Domain: "go.dev", Engine: "brave", Score: 0.9},
	})

	_, err := o.Search(context.Background(), Request{Query: "capital of france"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// give the fire-and-forget goroutine time to complete
	<-fc.done

	if fc.writes == 0 {
		t.Error("expected cache write after cache miss, got none")
	}
}

func TestSearchResponseNotCachedOnFirstCall(t *testing.T) {
	clf := &fakeClassifier{
		intent: classifier.Intent{Class: "code", Confidence: 0.91},
	}
	o := buildOrchestrator(newFakeCache(), clf, []merger.Result{
		{URL: "https://go.dev", Domain: "go.dev", Engine: "brave", Score: 0.9},
	})

	resp, err := o.Search(context.Background(), Request{Query: "goroutine panic"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Cached {
		t.Error("first call must not be marked as cached")
	}
}