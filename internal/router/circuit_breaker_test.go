package router

import (
	"testing"
	"time"
)

func newTestBreaker() *circuitBreaker {
	return newCircuitBreaker(3, 100*time.Millisecond)
}

func TestCircuitBreakerStartsClosed(t *testing.T) {
	cb := newTestBreaker()
	if cb.healthScore() != 1.0 {
		t.Errorf("expected health 1.0 when closed, got %.1f", cb.healthScore())
	}
}

func TestCircuitBreakerOpensAfterThreshold(t *testing.T) {
	cb := newTestBreaker()
	cb.recordFailure()
	cb.recordFailure()
	cb.recordFailure()

	if !cb.isOpen() {
		t.Error("expected circuit to be open after 3 failures")
	}
	if cb.healthScore() != 0.0 {
		t.Errorf("expected health 0.0 when open, got %.1f", cb.healthScore())
	}
}

func TestCircuitBreakerDoesNotOpenBeforeThreshold(t *testing.T) {
	cb := newTestBreaker()
	cb.recordFailure()
	cb.recordFailure()

	if cb.isOpen() {
		t.Error("circuit must not open before threshold is reached")
	}
}

func TestCircuitBreakerTransitionsToHalfOpenAfterCooldown(t *testing.T) {
	cb := newTestBreaker()
	cb.recordFailure()
	cb.recordFailure()
	cb.recordFailure()

	time.Sleep(150 * time.Millisecond)

	score := cb.healthScore()
	if score != 0.5 {
		t.Errorf("expected health 0.5 in half-open state, got %.1f", score)
	}
}

func TestCircuitBreakerClosesOnSuccessFromHalfOpen(t *testing.T) {
	cb := newTestBreaker()
	cb.recordFailure()
	cb.recordFailure()
	cb.recordFailure()

	time.Sleep(150 * time.Millisecond)
	cb.healthScore() // triggers transition to half-open

	cb.recordSuccess()

	if cb.healthScore() != 1.0 {
		t.Errorf("expected health 1.0 after success from half-open, got %.1f", cb.healthScore())
	}
}

func TestCircuitBreakerSuccessResetsFailureCount(t *testing.T) {
	cb := newTestBreaker()
	cb.recordFailure()
	cb.recordFailure()
	cb.recordSuccess()
	cb.recordFailure()
	cb.recordFailure()

	// two failures after reset — should still be closed
	if cb.isOpen() {
		t.Error("circuit must not open when failure count was reset by a success")
	}
}