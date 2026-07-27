package router

import (
	"time"
)
 
const (
	defaultEngineCount = 3
	circuitBreakerThreshold = 3
	circuitBreakerCooldown = 30 * time.Second
)

type engine struct {
	name string
	circuit *circuitBreaker
	latencySamples []time.Duration
}

// Router selects which engines to query for a given intent class.
// The affinity matrix, circuit breakers, and latency samples all live here.
type Router struct {
	engines map[string]*engine
}

// New builds a Router with one engine entry per engine in the affinity matrix.
// No embedding, no gRPC, no external calls — pure in-memory initialisation.
func New() *Router {
	engines := make(map[string]*engine)

	for name := range affinityMatrix["general"] {
		engines[name]= &engine{
			name: name,
			circuit: newCircuitBreaker(circuitBreakerThreshold, circuitBreakerCooldown),
		}
	}
	return &Router{engines: engines}
}

// Select returns the names of the best engines to query for this intent.
func (r *Router) Select(intent string) []string{
	return selectEngines(r.engines, intent, defaultEngineCount)
}

// SelectHedge returns engines suited to both candidate intents combined.
// Called by the orchestrator when classifier confidence is below threshold.
func (r *Router) SelectHedge(intentA, intentB string) []string{
	setA := selectEngines(r.engines, intentA, 2)
	setB := selectEngines(r.engines, intentB, 2)

	seen := make(map[string]bool, len(setA)+len(setB))
	merged := make([]string, 0, len(setA)+len(setB))

	for _, e := range append(setA, setB...) {
		if !seen[e] {
			seen[e] = true
			merged= append(merged, e)
		}
	}
	return merged
}

// RecordSuccess updates the circuit breaker and latency sample after a
// successful SearXNG response.
func (r *Router) RecordSuccess(engineName string, latency time.Duration) {
	e, ok := r.engines[engineName]
	if !ok{
		return
	}
	e.circuit.recordSuccess()
	e.latencySamples = appendLatency(e.latencySamples, latency)
}

// RecordFailure nudges or opens the circuit breaker for a failing engine.
func (r *Router) RecordFailure(engineName string) {
	e, ok := r.engines[engineName]
	if !ok{
		return
	}
	e.circuit.recordFailure()
}

// RecordOutcome updates the affinity matrix from real query outcome signals.
// Stubbed in Phase 1 — activated in Phase 3.
func (r *Router) RecordOutcome(intent, engineName string, outcomeSignal float64) {
	updateAffinity(intent, engineName, outcomeSignal)
}

// appendLatency adds a sample to the rolling window capped at 50 entries.
func appendLatency(samples []time.Duration, d time.Duration) []time.Duration {
	samples = append(samples, d)
	if len(samples) > 50 {
		samples = samples[1:]
	}
	return samples	
}