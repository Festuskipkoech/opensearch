# Testing

## Philosophy

Tests in this project follow one rule: test behaviour, not implementation.
A test that breaks when you rename a variable is not a useful test. A test that
breaks when the classifier returns the wrong intent for a known query is.

Each module is tested in isolation. Real infrastructure is only involved in
integration tests. Load tests measure system behaviour under concurrent pressure.

---

## Test Types

### Unit Tests

Live next to the code they test in the same package using the _test.go suffix.
No network. No Redis. No SearXNG running. Dependencies replaced with interfaces
and simple in-memory fakes.

Fast — the entire unit suite must complete in under 10 seconds.

```
internal/classifier/classifier_test.go
internal/router/router_test.go
internal/router/circuit_breaker_test.go
internal/merger/merger_test.go
internal/crawler/decision_test.go
internal/cache/keys_test.go
```

What each covers:

**classifier** — known queries return correct intent class. Low similarity scores
below confidence threshold trigger uncertainty handling. Agent-provided intent
bypasses embedding.

**router** — engine scoring ranks correctly given known profile similarity,
latency, and health inputs. Diversity constraint excludes overlapping indexes.
Open circuit breaker removes engine from selection.

**circuit breaker** — state transitions CLOSED to OPEN after threshold failures.
OPEN to HALF-OPEN after cooldown. HALF-OPEN to CLOSED on probe success.
HALF-OPEN back to OPEN on probe failure.

**merger** — duplicate URLs removed. Duplicate domains collapsed to one result.
Reranking produces stable ordering given fixed relevance inputs.

**crawl decision** — high similarity score returns sufficient=true. Low score
returns sufficient=false with correct URL list. Source authority score
correctly influences threshold.

**cache keys** — normalisation produces identical keys for semantically
equivalent queries. Intent class included in key. Different intents produce
different keys for the same query.

---

### Integration Tests

Live in integration/ at the project root. Require Docker Compose running before
execution. Test the boundary between Go code and real infrastructure.

Run with:
```
docker compose up -d
go test ./integration/... -tags integration
```

The -tags integration flag ensures integration tests never run accidentally
during normal go test ./...

What integration tests cover:

**cache** — write a result to real Redis, read it back, verify TTL set correctly,
verify expired key returns miss.

**searxng client** — send a known query to our running SearXNG instance, verify
results returned have expected shape, verify parallel requests complete faster
than sequential.

**crawler gRPC** — send a URL to the running Spider-rs service, verify markdown
returned is non-empty, verify StreamCrawl delivers results in order.

**full pipeline** — send a fixed query through the complete search pipeline
end to end, verify response shape, verify cache populated after first request,
verify second identical request is a cache hit.

---

### Load Tests

Live in loadtest/. Use k6 which sends HTTP requests against the running stack.
Measure latency distribution, error rate, and cache hit rate under concurrent
pressure.

Run with:
```
docker compose up
k6 run loadtest/search.js
```

What load tests measure:

- p50, p95, p99 latency under 100 concurrent virtual users
- Error rate under sustained load
- Cache hit rate with repeated query patterns
- Latency degradation when one SearXNG engine is artificially slowed
- Goroutine count stability over time — no goroutine leaks

Pass criteria defined in Phase 3 acceptance criteria.

---

## Running Tests

Unit tests only, fast:
```
go test ./internal/...
```

Unit tests with race detector — always run this before committing:
```
go test -race ./internal/...
```

Unit tests with coverage report:
```
go test -cover ./internal/...
```

Integration tests — requires infrastructure running:
```
docker compose up -d
go test ./integration/... -tags integration
```

All tests including integration:
```
docker compose up -d
go test -race ./... -tags integration
```

---

## Writing a New Test

Unit test structure — arrange, act, assert:

```
func TestClassifierReturnsCodeForGoQuery(t *testing.T) {
    // arrange — build the thing under test with fakes
    // act     — call the function being tested
    // assert  — check the result using t.Errorf or t.Fatalf
}
```

Table-driven tests for functions with many input cases:

```
func TestMergerDeduplication(t *testing.T) {
    cases := []struct {
        name     string
        input    []RawResult
        expected int
    }{
        {"no duplicates", ...},
        {"duplicate URLs", ...},
        {"duplicate domains", ...},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            // test each case
        })
    }
}
```

Table-driven tests are the Go convention for exhaustive case coverage without
repeating test boilerplate.

---

## What Not To Test

- That a struct field can be set and read back
- That the standard library works correctly
- Internal implementation details that are not observable from outside the module
- Exact log message strings — test that an error was logged, not what it said