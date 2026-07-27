package router
 
import (
	"sort"
	"time"
)

const (
	weightAffinity = 0.40
	weightLatency = 0.30
	weightHealth = 0.20
	weightDiversity = 0.10

	maxAcceptableLatencyMS = 3000.0
)

// candidate holds a scored engine ready for selection.
type candidate struct {
	name string
	score float64
}

// score computes the weighted routing score for one engine.
func score(
	name string,
	intent string,
	cb *circuitBreaker,
	latencySamples []time.Duration,
	diversityPenalty float64,
) float64{
	affinity :=affinityFor(intent, name)
	latency := latencyScore(latencySamples)
	health := float64(cb.healthScore())

	return  affinity*weightAffinity + latency*weightLatency + health*weightHealth + diversityPenalty*weightDiversity
}

// selectEngines scores all engines for the given intent and returns the top n.
// Applies the diversity constraint and anchor enforcement.
func selectEngines(
	engines map[string]*engine,
	intent string,
	n int,
) []string{
	candidates := scoreCandidates(engines, intent)

	selected := make([]string, 0, n)
	for _, c := range candidates {
		if len(selected) >= n {
			break
		}
		if engines[c.name].circuit.isOpen() {
			continue
		}
		if overlapsSelected(c.name, selected) {
			continue
		}
		selected = append(selected, c.name)
	}
	selected = ensureAnchor(selected, candidates, engines, n)
	return selected
}

// scoreCandidates scores all healthy engines and returns them sorted
// highest score first
func scoreCandidates(engines map[string]*engine, intent string) []candidate {
	candidates := make([]candidate, 0, len(engines))

	for name, e := range engines{
		if e.circuit.isOpen() {
			continue
		}
		diversity := diversityBonus(name, engines)
		s := score(name, intent, e.circuit, e.latencySamples, diversity)
		candidates = append(candidates, candidate{name: name, score: s})
	}
	sort.Slice(candidates, func(i, j int) bool{
		return  candidates[i].score > candidates[j].score
	})
	return candidates
}

// overlapsSelected returns true if the candidate shares an index with any
// already-selected engine.
func overlapsSelected(candidate string, selected []string)bool {
	for _, pair := range indexOverlap {
		for _, s := range selected {
			if (pair[0] == candidate && pair[1] == s) || (pair[1] == candidate && pair[0] == s) {
				return true
			}
		}
	}
	return false
}

// diversityBonus returns a positive score contribution when an engine has
// an independent index. Returns 0 when the engine overlaps with a healthy peer.
func diversityBonus(name string, engines map[string]*engine) float64 {
	for _, pair := range indexOverlap {
		var peer string
		if pair[0] == name {
			peer = pair[1]
		} else if pair[1] == name {
			peer = pair[0]
		} else {
			continue
		}
		if e, ok := engines[peer]; ok && !e.circuit.isOpen() {
			return 0.0
		}
	}
	return 1.0
}

// ensureAnchor guarantees at least one anchor engine appears in the selection.
// If none are present the highest-scoring available anchor replaces the
// lowest-scoring selected engine.
func ensureAnchor(
	selected []string,
	ranked []candidate,
	engines map[string]*engine,
	n int,
) []string{
	for _, s := range selected{
		for _, a := range anchors {
			if s == a {
				return selected
			}
		}
	}

	for _, c := range ranked {
		for _, a :=range anchors {
			if c.name != a{
				continue
			}
			if engines[a].circuit.isOpen() {
				continue
			}
			if len(selected) >= n {
				selected[len(selected)-1] = c.name
			} else {
				selected = append(selected, c.name)
			}
			return selected
		}
	}
	return selected
}

// latencyScore normalises rolling average response time to 0.0-1.0.
// Lower latency produces a higher score. Returns 1.0 when no samples exist.
func latencyScore(samples []time.Duration) float64 {
	if len(samples) == 0 {
		return 1.0
	}

	var total time.Duration
	for _, d := range samples {
		total += d
	}
	avg := float64(total/time.Millisecond) / float64(len(samples))
	s := 1.0 - (avg / maxAcceptableLatencyMS)
	if s < 0 {
		return 0.0
	}
	return s
}