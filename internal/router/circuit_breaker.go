package router
 
import (
	"sync"
	"time"
)

type circuitState int

const (
	stateClosed circuitState = iota // healthy — all requests pass through
	stateOpen // failing — engine excluded from routing
	stateHalfOpened // probing — one request allowed to test recovery
)
// circuitBreaker tracks the health of one engine.
// State lives in memory. Resets to closed on server restart.
type circuitBreaker struct {
	mu sync.Mutex
	state circuitState
	failures int
	lastFailure time.Time
	threshold int // failures before opening
	cooldown time.Duration // how long to stay open before probing
}

func newCircuitBreaker(threshold int, cooldown time.Duration) *circuitBreaker{
	return  &circuitBreaker{
		state: stateClosed,
		threshold: threshold,
		cooldown: cooldown,
	}
}

// healthScore returns a routing weight based on current state.
// CLOSED = 1.0, HALF-OPEN = 0.5, OPEN = 0.0
func (cb *circuitBreaker) healthScore() float32 {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == stateOpen && time.Since(cb.lastFailure) >= cb.cooldown {
		cb.state= stateHalfOpened
	}

	switch cb.state {
	case stateClosed:
		return 1.0
	case stateHalfOpened:
		return  0.5
	default:
		return 0.0
	}
}

// recordSuccess resets the circuit to closed.
func (cb *circuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = stateClosed
	cb.failures = 0
}

// recordFailure increments the failure count and opens the circuit
// if the threshold is exceeded.
func (cb *circuitBreaker) recordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures++
	cb.lastFailure = time.Now()
	if cb.failures >= cb.threshold {
		cb.state = stateOpen
	}
}

// isOpen returns true when the engine must not be used.
func (cb *circuitBreaker) isOpen() bool{
	return cb.healthScore() == 0.0
}