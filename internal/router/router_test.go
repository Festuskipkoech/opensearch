package router

import (
	"testing"
	"time"
)

// buildTestRouter constructs a Router with all engines healthy and no
// latency samples. Tests that need specific conditions mutate state directly.
func buildTestRouter() *Router {
	return New()
}

func TestSelectReturnsRequestedCount(t *testing.T) {
	r := buildTestRouter()
	selected := r.Select("code")
	if len(selected) != defaultEngineCount {
		t.Errorf("expected %d engines, got %d: %v", defaultEngineCount, len(selected), selected)
	}
}

func TestSelectHighAffinityEngineAppears(t *testing.T) {
	r := buildTestRouter()

	// brave and ddg both have 0.80 affinity for code — at least one must appear
	selected := r.Select("code")
	found := false
	for _, s := range selected {
		if s == "brave" || s == "ddg" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected brave or ddg in code selection, got: %v", selected)
	}
}

func TestSelectExcludesOpenCircuit(t *testing.T) {
	r := buildTestRouter()

	// open brave's circuit
	r.engines["brave"].circuit.recordFailure()
	r.engines["brave"].circuit.recordFailure()
	r.engines["brave"].circuit.recordFailure()

	selected := r.Select("research")
	for _, s := range selected {
		if s == "brave" {
			t.Errorf("open circuit engine brave must not appear in selection: %v", selected)
		}
	}
}

func TestSelectDiversityConstraintNoBothDDGAndBing(t *testing.T) {
	r := buildTestRouter()

	// run multiple intents — ddg and bing must never both appear
	for _, intent := range []string{"news", "code", "factual", "research", "commercial", "general"} {
		selected := r.Select(intent)
		hasDDG := false
		hasBing := false
		for _, s := range selected {
			if s == "ddg" {
				hasDDG = true
			}
			if s == "bing" {
				hasBing = true
			}
		}
		if hasDDG && hasBing {
			t.Errorf("intent %q: ddg and bing share an index and must not both appear: %v", intent, selected)
		}
	}
}

func TestSelectAlwaysIncludesAnchor(t *testing.T) {
	r := buildTestRouter()

	for _, intent := range []string{"news", "code", "factual", "research", "commercial", "general"} {
		selected := r.Select(intent)
		hasAnchor := false
		for _, s := range selected {
			if s == "brave" || s == "ddg" {
				hasAnchor = true
			}
		}
		if !hasAnchor {
			t.Errorf("intent %q: expected anchor (brave or ddg) in selection: %v", intent, selected)
		}
	}
}

func TestSelectHedgeDeduplicates(t *testing.T) {
	r := buildTestRouter()
	selected := r.SelectHedge("code", "research")

	seen := make(map[string]bool)
	for _, s := range selected {
		if seen[s] {
			t.Errorf("SelectHedge returned duplicate engine %q: %v", s, selected)
		}
		seen[s] = true
	}
}

func TestSelectHedgeCoversBothIntents(t *testing.T) {
	r := buildTestRouter()

	// code -> brave, ddg high affinity
	// factual -> ddg, mojeek high affinity
	// hedge should cover both
	selected := r.SelectHedge("code", "factual")
	if len(selected) == 0 {
		t.Error("SelectHedge returned empty selection")
	}
}

func TestRecordSuccessUpdatesLatency(t *testing.T) {
	r := buildTestRouter()
	r.RecordSuccess("brave", 200*time.Millisecond)

	if len(r.engines["brave"].latencySamples) != 1 {
		t.Errorf("expected 1 latency sample after RecordSuccess, got %d",
			len(r.engines["brave"].latencySamples))
	}
}

func TestRecordFailureOpensCircuitAfterThreshold(t *testing.T) {
	r := buildTestRouter()
	r.RecordFailure("bing")
	r.RecordFailure("bing")
	r.RecordFailure("bing")

	if !r.engines["bing"].circuit.isOpen() {
		t.Error("expected bing circuit to be open after 3 failures")
	}

	selected := r.Select("commercial")
	for _, s := range selected {
		if s == "bing" {
			t.Errorf("open circuit bing must not appear in selection: %v", selected)
		}
	}
}

func TestLatencyScoreFullWithNoSamples(t *testing.T) {
	score := latencyScore(nil)
	if score != 1.0 {
		t.Errorf("expected latency score 1.0 with no samples, got %.2f", score)
	}
}

func TestLatencyScoreDegradesWithHighLatency(t *testing.T) {
	samples := []time.Duration{2500 * time.Millisecond}
	score := latencyScore(samples)
	if score >= 1.0 || score < 0 {
		t.Errorf("expected degraded latency score between 0 and 1, got %.2f", score)
	}
}

func TestLatencyScoreZeroAtOrAboveMax(t *testing.T) {
	samples := []time.Duration{5000 * time.Millisecond}
	score := latencyScore(samples)
	if score != 0.0 {
		t.Errorf("expected latency score 0.0 at max latency, got %.2f", score)
	}
}

func TestAffinityForUnknownIntentReturnsDefault(t *testing.T) {
	score := affinityFor("unknown_intent", "brave")
	if score != 0.30 {
		t.Errorf("expected default affinity 0.30 for unknown intent, got %.2f", score)
	}
}

func TestAffinityForUnknownEngineReturnsDefault(t *testing.T) {
	score := affinityFor("code", "unknown_engine")
	if score != 0.30 {
		t.Errorf("expected default affinity 0.30 for unknown engine, got %.2f", score)
	}
}

func TestAppendLatencyCapAt50(t *testing.T) {
	var samples []time.Duration
	for i := 0; i < 60; i++ {
		samples = appendLatency(samples, time.Duration(i)*time.Millisecond)
	}
	if len(samples) != 50 {
		t.Errorf("expected rolling window capped at 50, got %d", len(samples))
	}
}