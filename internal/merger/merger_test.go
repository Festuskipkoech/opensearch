package merger

import (
	"testing"
	"fmt"
)

func results(items ...Result) []EngineResults {
	return []EngineResults{{Engine: "brave", Results: items}}
}

func TestMergeDeduplicatesByURL(t *testing.T) {
	input := []EngineResults{
		{Engine: "brave", Results: []Result{
			{URL: "https://go.dev", Domain: "go.dev", Score: 0.9},
			{URL: "https://go.dev", Domain: "go.dev", Score: 0.8},
		}},
	}

	merged := Merge(input)
	if len(merged) != 1 {
		t.Errorf("expected 1 result after URL deduplication, got %d", len(merged))
	}
}

func TestMergeDeduplicatesByDomain(t *testing.T) {
	input := []EngineResults{
		{Engine: "brave", Results: []Result{
			{URL: "https://go.dev/doc", Domain: "go.dev", Score: 0.9},
			{URL: "https://go.dev/tour", Domain: "go.dev", Score: 0.8},
		}},
	}

	merged := Merge(input)
	if len(merged) != 1 {
		t.Errorf("expected 1 result after domain deduplication, got %d", len(merged))
	}
}

func TestMergeDomainDeduplicationKeepsHighestScore(t *testing.T) {
	input := []EngineResults{
		{Engine: "brave", Results: []Result{
			{URL: "https://go.dev/doc", Domain: "go.dev", Score: 0.9},
			{URL: "https://go.dev/tour", Domain: "go.dev", Score: 0.5},
		}},
	}

	merged := Merge(input)
	if len(merged) != 1 {
		t.Fatalf("expected 1 result, got %d", len(merged))
	}
	if merged[0].Score != 0.9 {
		t.Errorf("expected highest scoring result (0.9) kept, got score %.1f", merged[0].Score)
	}
}

func TestMergeRanksByScoreDescending(t *testing.T) {
	input := results(
		Result{URL: "https://a.com", Domain: "a.com", Score: 0.5},
		Result{URL: "https://b.com", Domain: "b.com", Score: 0.9},
		Result{URL: "https://c.com", Domain: "c.com", Score: 0.7},
	)

	merged := Merge(input)
	if len(merged) < 2 {
		t.Fatal("expected at least 2 results")
	}
	for i := 1; i < len(merged); i++ {
		if merged[i].Score > merged[i-1].Score {
			t.Errorf("results not sorted: position %d score %.1f > position %d score %.1f",
				i, merged[i].Score, i-1, merged[i-1].Score)
		}
	}
}

func TestMergeSkipsFailedEngines(t *testing.T) {
	input := []EngineResults{
		{Engine: "brave", Results: []Result{
			{URL: "https://go.dev", Domain: "go.dev", Score: 0.9},
		}},
		{Engine: "ddg", Err: fmt.Errorf("engine timeout"), Results: nil},
	}

	merged := Merge(input)
	if len(merged) == 0 {
		t.Error("expected results from healthy engine, got none")
	}
}

func TestMergeEmptyInputReturnsEmpty(t *testing.T) {
	merged := Merge(nil)
	if len(merged) != 0 {
		t.Errorf("expected empty result for nil input, got %d results", len(merged))
	}
}

func TestMergeDistinctDomainsAllKept(t *testing.T) {
	input := results(
		Result{URL: "https://a.com", Domain: "a.com", Score: 0.9},
		Result{URL: "https://b.com", Domain: "b.com", Score: 0.8},
		Result{URL: "https://c.com", Domain: "c.com", Score: 0.7},
	)

	merged := Merge(input)
	if len(merged) != 3 {
		t.Errorf("expected 3 results for 3 distinct domains, got %d", len(merged))
	}
}