package utils

import (
	"errors"
	"sync"
	"time"
)

// CircuitState represents the state of the circuit breaker
type CircuitState string

const (
	// StateClosed allows requests to pass through
	StateClosed CircuitState = "closed"
	// StateOpen blocks requests
	StateOpen CircuitState = "open"
	// StateHalfOpen allows limited requests to test if service recovered
	StateHalfOpen CircuitState = "half-open"
)

var (
	// ErrCircuitOpen is returned when circuit breaker is open
	ErrCircuitOpen = errors.New("circuit breaker is open")
	// ErrTooManyRequests is returned when too many requests in half-open state
	ErrTooManyRequests = errors.New("too many requests in half-open state")
)

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	maxFailures  int
	resetTimeout time.Duration
	halfOpenMax  int

	mutex       sync.Mutex
	state       CircuitState
	failures    int
	lastFailure time.Time
	halfOpenReq int
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(maxFailures int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		maxFailures:  maxFailures,
		resetTimeout: resetTimeout,
		halfOpenMax:  1, // Allow 1 request in half-open state
		state:        StateClosed,
	}
}

// Call executes the given function with circuit breaker protection
func (cb *CircuitBreaker) Call(fn func() error) error {
	cb.mutex.Lock()

	// Check if circuit should transition from open to half-open
	if cb.state == StateOpen {
		if time.Since(cb.lastFailure) > cb.resetTimeout {
			cb.state = StateHalfOpen
			cb.halfOpenReq = 0
		} else {
			cb.mutex.Unlock()
			return ErrCircuitOpen
		}
	}

	// Limit concurrent requests in half-open state
	if cb.state == StateHalfOpen {
		if cb.halfOpenReq >= cb.halfOpenMax {
			cb.mutex.Unlock()
			return ErrTooManyRequests
		}
		cb.halfOpenReq++
	}

	cb.mutex.Unlock()

	// Execute the function
	err := fn()

	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	if err != nil {
		cb.onFailure()
		return err
	}

	cb.onSuccess()
	return nil
}

// onFailure handles a failed request
func (cb *CircuitBreaker) onFailure() {
	cb.failures++
	cb.lastFailure = time.Now()

	if cb.state == StateHalfOpen {
		// Go back to open state if half-open request fails
		cb.state = StateOpen
		cb.failures = cb.maxFailures // Ensure it stays open
	} else if cb.failures >= cb.maxFailures {
		cb.state = StateOpen
	}
}

// onSuccess handles a successful request
func (cb *CircuitBreaker) onSuccess() {
	if cb.state == StateHalfOpen {
		// Transition to closed if half-open request succeeds
		cb.state = StateClosed
		cb.failures = 0
		cb.halfOpenReq = 0
	} else if cb.state == StateClosed {
		// Reset failure count on success
		cb.failures = 0
	}
}

// GetState returns the current state of the circuit breaker
func (cb *CircuitBreaker) GetState() CircuitState {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()
	return cb.state
}

// Reset resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()
	cb.state = StateClosed
	cb.failures = 0
	cb.halfOpenReq = 0
}
