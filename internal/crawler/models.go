package crawler

import "opensearch/internal/merger"

// Decision is the output of the crawl decision layer.
type Decision struct {
	Sufficient bool
	URLs []string // top N URLs to crawl when not sufficient
}

// Request carries everything the crawler needs to make a sufficiency decision.
type Request struct {
	Query string
	Intent string
	Results []merger.Result
}