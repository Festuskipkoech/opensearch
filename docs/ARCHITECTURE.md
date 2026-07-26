# Architecture

## Shape

OpenSearch is a modular monolith. One Go binary with clean internal module
boundaries. The binary coordinates four self-hosted infrastructure pieces —
SearXNG, Spider-rs, the model service, and Redis — all running in the same
Docker Compose stack.

---

## Deployment Topology

```
Docker Compose stack

  Go binary (opensearch)
         |
         |-- HTTP/JSON ---------> SearXNG container (our instance)
         |-- gRPC ---------------> Spider-rs container (our Rust binary)
         |-- gRPC ---------------> Model service container (our Python service)
         |-- Redis protocol -----> Redis container
```

All containers share one Docker network. No external API calls in the critical
path. Zero per-query cost.

---

## Communication Protocols

```
Agent          -> Go engine       HTTP/JSON    external facing, agent-native
Go engine      -> SearXNG         HTTP/JSON    SearXNG exposes HTTP, we adapt
Go engine      -> Spider-rs       gRPC         we own both sides, we define the proto
Go engine      -> Model service   gRPC         we own both sides, we define the proto
Go engine      -> Redis           Redis proto  handled by Go Redis library
```

gRPC is used at both the Spider-rs and model service boundaries because we wrote
both the server and the client in each case and defined the proto contract.
SearXNG is an existing Python application whose HTTP interface we cannot change.

---

## The Model Service

A separate Python gRPC service. No FastAPI, no HTTP — pure gRPC server.
Two models load from disk at startup and stay loaded for the server lifetime.
No HuggingFace calls at runtime. No external network calls after startup.

```
rpc Classify   distilbert-base-uncased fine-tuned on our query dataset
               input:  query string
               output: intent class + confidence score 0.0 to 1.0

rpc Relevance  cross-encoder/ms-marco-MiniLM-L6-v2 pinned from workbench
               input:  query string + snippet string
               output: relevance score 0.0 to 1.0
                       sigmoid normalisation applied internally before returning
                       the Go crawler module always receives a 0-1 value
```

Both model weight directories come from the opensearch-models workbench.
Neither model downloads anything at container startup.

```
opensearch/models/classifier/   fine-tuned distilbert — ~253MB
opensearch/models/relevance/    pinned cross-encoder  — 91MB
```

### Model Service Structure

```
model_service/
├── service.py          gRPC server entry point, loads both models at startup
├── classifier.py       distilbert inference — returns intent class + confidence
├── relevance.py        cross-encoder inference — applies sigmoid, returns 0-1 score
├── Dockerfile
└── requirements.txt
```

`service.py` initialises both models before the gRPC server binds to its port.
If either model fails to load from disk the service exits — it does not start
in a degraded state. The Go engine health check will detect the failure and
the server will not accept traffic until the model service is healthy.

---

## Module Boundaries

**api**
Receives HTTP requests. Validates input. Calls the orchestrator. Writes HTTP
responses. Contains no business logic.

**orchestrator**
The only module that knows the full request flow. Coordinates classifier client,
router, searxng, merger, crawler, and cache in sequence. Thin — coordinates,
does not compute.

**classifier**
gRPC client to the model service rpc Classify. Returns intent class, confidence,
and flags uncertainty when confidence is below threshold.
Knows nothing about engines or crawling.

**router**
Takes the intent class from the classifier. Scores engines using the affinity
matrix combined with live latency and circuit breaker health. Returns ordered
engine list. Maintains circuit breaker state and latency samples in memory.

**searxng**
Receives a query and a list of engines. Fires parallel HTTP requests to our
SearXNG instance. Returns raw results. Knows nothing about classification.

**merger**
Receives multiple result lists from parallel engine queries. Returns one
deduplicated reranked list. Knows nothing about where results came from.

**crawler**
gRPC client to the model service rpc Relevance for sufficiency scoring.
gRPC client to Spider-rs for content extraction. Returns enriched results.
Receives relevance scores already normalised to 0-1 — applies sufficiency
formula directly without any sigmoid conversion.

**cache**
Receives a cache key and a typed result. Stores and retrieves from Redis.
TTL determined by intent class, passed in by the orchestrator.

**config**
Loads all configuration from environment variables at startup. Every other
module receives configuration as a struct. Nothing reads environment variables
directly except this package.

---

## Routing — Affinity Matrix

Routing is not a lookup table. Each engine has a base affinity score per intent
class representing how well that engine historically performs for that intent.
Scores are seeded in broad honest tiers grounded in publicly available engine
quality research. Precise calibration comes from real traffic via EMA update.

```
              brave   ddg    bing   mojeek  yandex
news           0.80   0.60   0.80   0.30    0.60
code           0.80   0.80   0.80   0.30    0.20
factual        0.60   0.80   0.60   0.60    0.30
research       0.80   0.60   0.60   0.60    0.30
commercial     0.60   0.60   0.80   0.30    0.30
general        0.80   0.80   0.60   0.60    0.60
```

Tier basis:
- brave    independent index, high quality, high blocking tolerance
- ddg      Bing-proxied, reliable, low-JS endpoints, high blocking tolerance
- bing     independent index, strong commercial and code coverage
- mojeek   independent index, solid general queries, very high blocking tolerance
- yandex   strong non-English and local queries, weak English technical content

Google is excluded from Phase 1 and 2. It blocks SearXNG instances aggressively
via TLS fingerprinting — a fresh instance gets blocked after 5 requests. It
enters the engine pool in Phase 3 behind TLS fingerprint rotation and a
residential proxy, added to the affinity matrix at that point.

Engine score formula:

```
score = (intent_affinity * 0.40)
      + (latency_score   * 0.30)
      + (health_score    * 0.20)
      + (diversity_bonus * 0.10)
```

Affinity update after each query outcome (Phase 3):

```
new_affinity = old_affinity * 0.95 + outcome_signal * 0.05
```

outcome_signal derives from cross-encoder relevance scores on returned results.
Phase 1 seeds expert values and does not update. Phase 3 activates updates.

---

## Dependency Injection

All dependencies constructed once in main.go and injected explicitly.
Nothing creates its own dependencies. Nothing uses globals.

```
startup order:

  config          loaded first
  redis           created once, lives for server lifetime
  cache           receives redis client
  model client    gRPC connection to Python model service
  classifier      receives model client
  router          affinity matrix + circuit breakers live here in memory
  searxng         receives config for instance URL
  merger          no external dependencies
  crawler         receives model client (relevance) + gRPC conn to Spider-rs
  orchestrator    receives all of the above
  api             receives orchestrator
```

---

## Concurrency Model

**SearXNG fan-out**
Selected engines queried in parallel via goroutines. Results flow into a
buffered channel. Early exit when top two engines respond and result count
exceeds minimum threshold. Remaining requests cancelled via context.

**Spider-rs streaming**
Go client calls StreamCrawl. Results stream back as each URL completes via
gRPC server-side streaming. Forwarded to agent as they arrive.

**Cache write**
Fires in a goroutine after response is built. Agent does not wait for it.

---

## Circuit Breaker

Each engine has an independent circuit breaker with three states.

```
CLOSED      healthy, requests pass through
OPEN        failing or blocked, engine excluded from routing
HALF-OPEN   cooldown expired, one probe request allowed
```

State lives in memory inside the router module. Resets to CLOSED on restart.

---

## File Organisation

```
opensearch/
├── cmd/server/main.go
├── internal/
│   ├── api/
│   ├── orchestrator/
│   ├── classifier/
│   ├── router/
│   ├── searxng/
│   ├── merger/
│   ├── crawler/
│   ├── cache/
│   └── config/
├── model_service/
│   ├── service.py
│   ├── classifier.py
│   ├── relevance.py
│   ├── Dockerfile
│   └── requirements.txt
├── models/
│   ├── classifier/     fine-tuned distilbert weights (~253MB)
│   └── relevance/      pinned cross-encoder weights (91MB)
├── proto/
│   ├── crawler/
│   └── models/
├── gen/
│   ├── crawler/
│   └── models/
├── integration/
├── loadtest/
├── config/searxng/
└── docs/
```

Test files live next to the code they test using the _test.go suffix.
Integration tests requiring live infrastructure live in integration/.
