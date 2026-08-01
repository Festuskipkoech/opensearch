package types

import "opensearch/internal/merger"

// Response is the structured result returned to the agent.
// Lives in its own package to avoid circular imports between
// cache, orchestrator, and api packages.
type Response struct {
	Query string `json:"query"`
	Intent string `json:"intent"`
	Uncertain bool `json:"uncertain,omitempty"`
	Results []merger.Result `json:"results"`
	Cached bool `json:"cached"`
	LatencyMS int64 `json:"latency_ms"`
}