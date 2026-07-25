# Phases

Each phase delivers a working, testable system. No phase ends with half-built
infrastructure. Acceptance criteria define what done means.

---

## Dependency Before Phase 1

The fine-tuned distilbert weights must exist in opensearch/models/classifier/
before Group 2 code can be written or tested. These come from the opensearch-models
workbench project. The cross-encoder downloads from HuggingFace at container
startup — no manual step needed for that model.

---

## Phase 1 — Core Search Pipeline

**What it builds**

The complete Go engine returning snippet-only search results. No Rust yet.
A working search engine end to end.

- HTTP API endpoint accepting search requests
- gRPC client to model service for intent classification
- Engine router with affinity matrix scoring and diversity constraint
- Circuit breaker per engine — CLOSED, OPEN, HALF-OPEN state machine
- SearXNG HTTP client with parallel fan-out and early exit
- Result merger with URL and domain deduplication
- gRPC client to model service for snippet relevance scoring
- Redis cache with intent-aware TTL
- Crawl decision layer present but stubbed — always returns sufficient
- Graceful shutdown — drains in-flight requests on SIGTERM
- Docker Compose stack — Go binary, model service, SearXNG, Redis
- Python model service with distilbert and cross-encoder over gRPC

**Active engines in Phase 1**

```
brave, ddg, bing, mojeek, yandex
```

Google is not included. It blocks SearXNG instances after 5 consecutive
requests via TLS fingerprinting. It enters the pool in Phase 3.

**What it defers**

Content extraction, Spider-rs, streaming responses, router affinity updates,
Google engine.

**Acceptance criteria**

- POST /search returns structured JSON results in under 800ms for common queries
- Cache hit returns the same result in under 50ms
- Classifier correctly identifies intent for 15 known test queries
- Circuit breaker opens after 3 failures and recovers after cooldown
- Parallel fan-out to two engines completes faster than sequential
- docker compose up brings the full stack with one command

---

## Phase 2 — Content Extraction

**What it builds**

The Rust Spider-rs binary wrapped in a gRPC server, the Go gRPC client,
the live crawl decision layer, and streaming responses.

- Spider-rs Rust binary with gRPC server implementing the crawler proto
- Go gRPC client for Spider-rs in internal/crawler
- Live crawl decision using relevance scores from model service
- StreamCrawl integration — results stream back as each URL completes
- Streaming HTTP response — agent receives snippets immediately, content follows
- Token counting per result chunk
- Docker Compose updated with Spider-rs container

**What it defers**

Router affinity updates, Google engine, load testing, observability.

**Acceptance criteria**

- Research queries return full markdown content from top 3 URLs
- Factual queries with clear snippet answers do not invoke Spider-rs
- Streaming response delivers first snippet result in under 500ms
- Spider-rs container starts cleanly and accepts gRPC connections
- Token count present and accurate on every crawled result

---

## Phase 3 — Resilience and Intelligence

**What it builds**

The systems that make the engine production-hardened and self-improving.
Google enters the engine pool in this phase.

- Router affinity matrix updates from real query outcome signals
- Google engine added with affinity 0.90 across all intents
- TLS fingerprint rotation per request for Google outbound calls
- Residential proxy pool integration for Google routing
- Google circuit breaker released from forced OPEN to CLOSED
  only after proxy infrastructure is confirmed working
- pprof profiling endpoints
- Structured request tracing — request ID in every log line
- Load test at 100 concurrent queries measuring p50, p95, p99 latency

**What it defers**

Multi-region SearXNG, Kubernetes manifests.

**Acceptance criteria**

- Affinity scores demonstrably shift after sustained traffic on known intents
- Google engine returns results without triggering a block under normal load
- Manually blocking an engine causes OPEN state within 3 failures
- Blocked engine recovers after cooldown without operator action
- p95 latency under 100 concurrent queries stays under 1500ms for snippets
- Cache hit rate above 40% during load test with repeated query patterns
- No goroutine leaks under sustained load

---

## Phase 4 — Deployment and Operations

**What it builds**

Everything needed to deploy and operate the engine in production.

- Dockerfile for Go binary — multi-stage build, minimal final image
- Dockerfile for Spider-rs binary — multi-stage Rust build
- Dockerfile for model service — Python with both models
- Production Docker Compose with health checks on all services
- Health check endpoint returning status of all dependencies
- Environment-based configuration — no hardcoded values anywhere
- Graceful startup — engine does not accept traffic until all dependencies healthy

**Acceptance criteria**

- docker compose up starts all services cleanly with no manual steps
- Health check correctly reports degraded state when any dependency is down
- Configuration fully driven by environment variables with documented defaults
- Multi-stage Docker builds produce images under 100MB for Go binary
- Restarting the Go container under load causes no request loss to clients
  that retry on disconnect