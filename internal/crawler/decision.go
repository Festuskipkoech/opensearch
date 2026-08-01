package crawler
 
// Decision layer is stubbed in Phase 1.
// Always returns sufficient=true so the pipeline returns snippets immediately.
// Phase 2 replaces this with real sufficiency scoring via the model service
// relevance endpoint and snippet density analysis.

const maxCrawlURLs = 3

// Decide determines whether snippets are sufficient or Spider-rs is needed.
// Phase 1 stub — always sufficient.
func Decide(_ Request) Decision {
	return Decision{Sufficient: true}
}

// topURLs extracts the top N URLs from a result list.
// Used by the real implementation in Phase 2.
// Kept here so the signature is established and Phase 2 can slot in cleanly.
func topURLs(req Request) []string {
	n := maxCrawlURLs
	if len(req.Results) < n {
		n = len(req.Results)
	}
	urls := make([]string, n)
	for i :=0; i < n; i++ {
		urls[i] = req.Results[i].URL
	}
	return urls
}
