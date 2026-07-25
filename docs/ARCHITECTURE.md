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

A separate Python service exposing two gRPC endpoints. One service, two models
loaded into memory at startup.

```
POST /classify    distilbert-base-uncased fine-tuned on our query dataset
                  input: query string
                  output: intent class + confidence score

POST /relevance   cross-encoder/ms-marco-MiniLM-L6-v2
                  input: query string + snippet string
                  output: relevance score 0.0 to 1.0
```

Model weights live in opensearch/models/. The fine-tuned distilbert weights
come from the opensearch-models workbench project. The cross-encoder weights
download from HuggingFace at container startup.

The model service is a separate project from the fine-tuning workbench.
opensearch-models handles all dirty experimentation and exports final weights.
opensearch wraps those weights in a production gRPC service.

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
gRPC client to the model service /classify endpoint. Returns intent class,
confidence, and flags uncertainty when confidence is below threshold.
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
gRPC client to the model service /relevance endpoint for sufficiency scoring.
gRPC client to Spider-rs for content extraction. Returns enriched results.

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
class — a floating point number representing how well that engine historically
performs for that intent. These scores start as expert-seeded values and update
from real query outcomes over time using an exponential moving average.

```
              brave   ddg    bing   mojeek  yandex
news           0.90   0.60   0.72   0.40    0.65
code           0.75   0.85   0.80   0.35    0.20
factual        0.65   0.90   0.60   0.50    0.40
research       0.85   0.55   0.70   0.80    0.30
commercial     0.70   0.60   0.85   0.30    0.25
general        0.80   0.75   0.65   0.75    0.55
```

Engine score formula:

```
score = (intent_affinity * 0.40)
      + (latency_score   * 0.30)
      + (health_score    * 0.20)
      + (diversity_bonus * 0.10)
```

Affinity update after each query outcome:

```
new_affinity = old_affinity * 0.95 + outcome_signal * 0.05
```

Where outcome_signal comes from cross-encoder relevance scores on the returned
results. High relevance nudges affinity up. Low relevance nudges it down.
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
│   ├── classifier/     fine-tuned distilbert weights from opensearch-models
│   └── relevance/      cross-encoder downloaded at container startup
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