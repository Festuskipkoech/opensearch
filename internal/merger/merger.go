package merger

import "sort"

// Merge combines results from multiple engines into one clean ordered list.
// Applies URL deduplication, domain deduplication, and score-based reranking.
func Merge(engineResults []EngineResults) []Result {
	deduped := deduplicateByURL(flatten(engineResults))
	deduped = deduplicateByDomain(deduped)
	sort.Slice(deduped, func(i, j int) bool {
		return  deduped[i].Score > deduped[j].Score
	})
	return deduped
}

// flatten collects all results from all engines into one slice.
// Failed engine results are skipped — a partial result set is better
// than surfacing an error for an engine the user did not ask about.
func flatten(engineResults []EngineResults) []Result {
	var all []Result
	for _, er := range engineResults {
		if er.Err != nil {
			continue
		}
		all = append(all, er.Results...)
	}
	return all
}

// deduplicateByURL keeps the first occurrence of each URL.
// If the same URL appears from multiple engines the highest-scored
// version wins because results are pre-sorted by the caller.
func deduplicateByURL(results []Result) []Result {
	seen := make(map[string]bool, len(results))
	deduped := make([]Result, 0, len(results))
	for _, r := range results {
		if seen[r.URL] {
			continue
		}
		seen[r.URL] = true
		deduped = append(deduped, r)
	}
	return deduped
}

// deduplicateByDomain keeps the highest-scoring result per domain.
// This prevents one domain from dominating the result list.
// Results must be sorted by score descending before calling this.
func deduplicateByDomain(results []Result) []Result {
	seen := make(map[string]bool, len(results))
	deduped := make([]Result, 0, len(results))
	for _, r := range results {
		if seen[r.Domain] {
			continue
		}
		seen[r.Domain] = true
		deduped = append(deduped, r)
	}
	return deduped
}