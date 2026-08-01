package merger

// Result represents one search result moving through the pipeline.
// This is the shared type — searxng, merger, crawler, and orchestrator
// all work with this struct.
type Result struct {
	URL string
	Title string
	Snippet string
	Domain string
	Engine string
	Score float64
	Content string // populated by crawler in Phase 2, empty in Phase 1
	Tokens int // populated by crawler in Phase 2, empty in Phase 1
}

// EngineResults holds the raw results from one engine query.
type EngineResults struct {
	Engine string
	Results []Result
	Err error
}