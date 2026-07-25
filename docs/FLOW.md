# Request Flow

Every search request follows this path from agent to response. Understanding
this flow is the fastest way to orient yourself in the codebase.

---

## Happy Path — Snippet Response (Cache Miss)

```
Agent
  |
  | POST /search {"query": "...", "intent": "code"}
  v
api.Handler
  validates request shape
  generates request ID
  attaches to context
  |
  v
orchestrator.Search(ctx, request)
  |
  v
cache.Get(key)
  miss — continue
  hit  — return immediately, skip everything below
  |
  v
classifier.Classify(ctx, query)
  agent provided intent -> use it, skip classification
  no intent provided    -> embed query, compare to prototypes, return intent
  returns: Intent{class, confidence, vector}
  |
  v
router.Select(ctx, intent, queryVector)
  scores all engines on profile similarity + latency + health
  applies diversity constraint
  returns: []Engine (top 2-3)
  |
  v
searxng.Search(ctx, query, engines)
  fires one goroutine per engine
  collects results via channel as they arrive
  early exit if top two responded and result count sufficient
  cancels remaining via context
  returns: []RawResult from each engine
  |
  v
merger.Merge(results)
  deduplicates by URL
  deduplicates by domain
  reranks combined list by relevance signal
  returns: []RankedResult
  |
  v
crawler.Decide(ctx, query, results)
  embeds each snippet
  scores sufficiency: query-snippet similarity + density + authority
  above threshold -> sufficient = true
  returns: sufficient bool, []URL (top N if not sufficient)
  |
  | sufficient = true
  v
cache.Set(key, response, ttl)    fired in goroutine, non-blocking
  |
  v
Agent receives SearchResponse
  query, intent, results[], cached=false, latency_ms
```

Total latency target: under 800ms for snippet responses.

---

## Happy Path — Content Response (Crawl Needed)

Same as above until crawler.Decide returns sufficient = false.

```
  ...merger.Merge...
  |
  v
crawler.Decide returns sufficient = false, urls = [top 3 URLs]
  |
  v
crawler.Fetch(ctx, urls)
  calls Spider-rs via gRPC StreamCrawl
  Spider-rs fetches each URL concurrently
  results stream back as each URL completes
  each result attached to the corresponding RankedResult
  returns: []EnrichedResult
  |
  v
cache.Set(key, response, ttl)    goroutine, non-blocking
  |
  v
Agent receives SearchResponse
  same shape as snippet response
  results[].content populated with markdown where crawled
  results[].token_count populated
```

Total latency target: under 3 seconds for content responses.

---

## Cache Hit Path

```
Agent
  |
  v
api.Handler
  |
  v
orchestrator
  |
  v
cache.Get(key)
  hit
  |
  v
Agent receives SearchResponse
  cached = true
  latency: under 50ms
```

Cache key is a normalised query hash — lowercased, whitespace-normalised. The
intent class is included in the key so the same query with different intents
gets separate cache entries.

---

## Engine Failure Path

```
  ...router.Select returns [brave, bing, ddg]...
  |
  v
searxng.Search fires goroutines for all three
  brave responds normally
  bing times out after 2s
  ddg responds normally
  |
  v
results channel receives brave + ddg results
  bing timeout increments bing circuit breaker failure count
  if failure count exceeds threshold: bing circuit breaker opens
  bing excluded from routing for cooldown period
  |
  v
merger.Merge proceeds with brave + ddg results
  pipeline continues normally
```

A single engine failure degrades result quality slightly but does not fail the
request. Two engine failures still return results from the surviving engine.
All three failing returns an error to the agent.

---

## Classification Uncertainty Path

```
  ...classifier.Classify runs...
  top similarity score = 0.54 (below confidence threshold of 0.65)
  second score = 0.49
  |
  v
orchestrator detects low confidence
  takes top two candidate intents: code + research
  queries engines suited to both
  merges results from both engine sets
  intent in response marked as "uncertain"
```

---

## Streaming Response Path (Phase 2)

When stream=true in the request:

```
  ...searxng.Search returns snippet results...
  |
  v
agent receives first SSE event: snippet results
  agent can begin reasoning immediately
  |
  v
crawler.Fetch streams results back from Spider-rs
  each URL that completes sends another SSE event
  agent receives enriched results as they resolve
  |
  v
final SSE event: done signal with total latency
```

---

## Context Cancellation

Every operation in the pipeline receives a context. If the agent disconnects
mid-request the context cancels. Goroutines doing SearXNG fan-out stop
immediately. The gRPC stream to Spider-rs is cancelled. No orphaned work
continues after the client disconnects.

The request ID attached to the context appears in every log line for the
lifetime of that request, making any request fully traceable in logs.