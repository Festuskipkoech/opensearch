# OpenSearch Engine

A fast, lightweight, self-hosted search engine built for AI agents.

Eliminates per-query costs of commercial search APIs by combining a self-hosted
metasearch layer with intelligent content extraction — returning structured,
LLM-ready output that agents can consume directly.

---

## The Problem

AI agents need search. Every agent workflow that reasons over the real world fires
search queries — sometimes dozens per task. Commercial APIs like Tavily, Serper,
and Brave Search charge per query. At agent scale those costs compound fast.

The open source ecosystem has no production-ready answer. SearXNG solves
discovery. Spider-rs solves content extraction. Nothing combines them into a
single agent-native engine a team can self-host and trust at scale.

This project fills that gap.

---

## What It Does

Takes a query from an agent. Classifies its intent. Routes to the best search
engines. Fetches results in parallel. Decides whether snippets are sufficient or
full page content is needed. Returns structured JSON with URLs, titles, snippets,
full markdown content where needed, token counts, and relevance scores.

Zero per-query cost. Fully self-hosted. One command to run.

---

## Tech Stack

- **Go** — core engine, API, classifier, router, merger, cache interface
- **Rust / Spider-rs** — content extraction layer, wrapped in a gRPC server
- **SearXNG** — self-hosted metasearch, aggregates 70+ search engines
- **Redis** — result caching with intent-aware TTL

---

## Running Locally

Requirements: Docker and Docker Compose.

```bash
git clone https://github.com/Festuskipkoech/opensearch
cd opensearch
docker compose up
```

The engine is available at `http://localhost:8080`.

Send a search request:

```bash
curl -X POST http://localhost:8080/search \
  -H "Content-Type: application/json" \
  -d '{"query": "goroutine scheduling internals"}'
```

---

## Project Structure

```
cmd/server/          entry point
internal/api/        HTTP handlers and middleware
internal/classifier/ intent classification via prototype vectors
internal/router/     engine selection and circuit breaking
internal/searxng/    SearXNG HTTP client
internal/merger/     result deduplication and reranking
internal/crawler/    crawl decision and Spider-rs gRPC client
internal/cache/      Redis interface
internal/config/     environment and configuration loading
internal/orchestrator/ request lifecycle coordination
proto/               Protocol Buffer definitions for Spider-rs boundary
integration/         integration tests requiring live infrastructure
loadtest/            load testing scripts
docs/                architecture, flow, phase, and decision documents
```

---

## Documentation

- `docs/ARCHITECTURE.md` — system design and module boundaries
- `docs/FLOW.md` — request lifecycle from agent to response
- `docs/PHASES.md` — what each phase builds and its acceptance criteria
- `docs/TESTING.md` — testing strategy and how to run the suite
- `docs/DECISIONS.md` — why key architectural decisions were made

---

## API

```
POST /search

{
  "query":       "string",
  "intent":      "news | code | research | factual | commercial | general",
  "max_results": 10,
  "crawl":       "auto | always | never",
  "stream":      false
}
```

`intent` is optional. When provided by the agent the classifier is skipped.
`crawl` defaults to auto. The crawl decision layer determines whether full
content extraction is needed based on query intent and snippet quality.

---

## License

MIT
