package resilience

import (
	"context"
	"sync"
	"time"

	"github.com/jcronq/frontend_router_mcp/internal/logger"
	appErrors "github.com/jcronq/frontend_router_mcp/internal/errors"
)

// State represents the circuit breaker state
type State int

const (
	// StateClosed means the circuit breaker is closed (normal operation)
	StateClosed State = iota
	// StateOpen means the circuit breaker is open (failing fast)
	StateOpen
	// StateHalfOpen means the circuit breaker is half-open (testing)
	StateHalfOpen
)

// String returns the string representation of the state
func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	name             string
	maxFailures      int
	resetTimeout     time.Duration
	halfOpenMaxCalls int
	
	mu             sync.RWMutex
	state          State
	failureCount   int
	lastFailureTime time.Time
	halfOpenCalls  int
	
	logger *logger.Logger
}

// Config holds circuit breaker configuration
type Config struct {
	Name             string
	MaxFailures      int
	ResetTimeout     time.Duration
	HalfOpenMaxCalls int
}

// DefaultConfig returns default circuit breaker configuration
func DefaultConfig(name string) Config {
	return Config{
		Name:             name,
		MaxFailures:      5,
		ResetTimeout:     30 * time.Second,
		HalfOpenMaxCalls: 3,
	}
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(config Config) *CircuitBreaker {
	return &CircuitBreaker{
		name:             config.Name,
		maxFailures:      config.MaxFailures,
		resetTimeout:     config.ResetTimeout,
		halfOpenMaxCalls: config.HalfOpenMaxCalls,
		state:            StateClosed,
		logger:           logger.WithComponent("circuit-breaker"),
	}
}

// Execute executes the given function with circuit breaker protection
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func() error) error {
	// Check if we should allow the call
	if !cb.allowCall(ctx) {
		return appErrors.New(appErrors.ErrorTypeNetwork, "circuit breaker is open").
			WithContext("circuit_breaker", cb.name).
			WithContext("state", cb.state.String())
	}
	
	// Execute the function
	err := fn()
	
	// Record the result
	cb.recordResult(ctx, err)
	
	return err
}

// allowCall checks if the call should be allowed based on circuit breaker state
func (cb *CircuitBreaker) allowCall(ctx context.Context) bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	
	switch cb.state {
	case StateClosed:
		return true
	case StateOpen:
		// Check if we should transition to half-open
		if time.Since(cb.lastFailureTime) > cb.resetTimeout {
			cb.logger.WithContext(ctx).Info("Circuit breaker transitioning to half-open",
				"name", cb.name,
				"failure_count", cb.failureCount)
			cb.state = StateHalfOpen
			cb.halfOpenCalls = 0
			return true
		}
		return false
	case StateHalfOpen:
		// Allow limited calls in half-open state
		if cb.halfOpenCalls < cb.halfOpenMaxCalls {
			cb.halfOpenCalls++
			return true
		}
		return false
	default:
		return false
	}
}

// recordResult records the result of a function call
func (cb *CircuitBreaker) recordResult(ctx context.Context, err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	
	if err != nil {
		cb.recordFailure(ctx)
	} else {
		cb.recordSuccess(ctx)
	}
}

// recordFailure records a failure
func (cb *CircuitBreaker) recordFailure(ctx context.Context) {
	cb.failureCount++
	cb.lastFailureTime = time.Now()
	
	switch cb.state {
	case StateClosed:
		if cb.failureCount >= cb.maxFailures {
			cb.logger.WithContext(ctx).Warn("Circuit breaker opening due to failures",
				"name", cb.name,
				"failure_count", cb.failureCount,
				"max_failures", cb.maxFailures)
			cb.state = StateOpen
		}
	case StateHalfOpen:
		cb.logger.WithContext(ctx).Warn("Circuit breaker opening from half-open due to failure",
			"name", cb.name,
			"failure_count", cb.failureCount)
		cb.state = StateOpen
	}
}

// recordSuccess records a success
func (cb *CircuitBreaker) recordSuccess(ctx context.Context) {
	switch cb.state {
	case StateClosed:
		cb.failureCount = 0
	case StateHalfOpen:
		// If we've completed all half-open calls successfully, close the circuit
		if cb.halfOpenCalls >= cb.halfOpenMaxCalls {
			cb.logger.WithContext(ctx).Info("Circuit breaker closing from half-open",
				"name", cb.name,
				"successful_calls", cb.halfOpenCalls)
			cb.state = StateClosed
			cb.failureCount = 0
		}
	}
}

// GetState returns the current state of the circuit breaker
func (cb *CircuitBreaker) GetState() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// GetFailureCount returns the current failure count
func (cb *CircuitBreaker) GetFailureCount() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.failureCount
}

// IsOpen returns true if the circuit breaker is open
func (cb *CircuitBreaker) IsOpen() bool {
	return cb.GetState() == StateOpen
}

// Reset resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset(ctx context.Context) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	
	cb.logger.WithContext(ctx).Info("Circuit breaker manually reset",
		"name", cb.name,
		"previous_state", cb.state.String())
	
	cb.state = StateClosed
	cb.failureCount = 0
	cb.halfOpenCalls = 0
}

// Manager manages multiple circuit breakers
type Manager struct {
	breakers map[string]*CircuitBreaker
	mu       sync.RWMutex
}

// NewManager creates a new circuit breaker manager
func NewManager() *Manager {
	return &Manager{
		breakers: make(map[string]*CircuitBreaker),
	}
}

// GetOrCreate gets an existing circuit breaker or creates a new one
func (m *Manager) GetOrCreate(name string, config Config) *CircuitBreaker {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if cb, exists := m.breakers[name]; exists {
		return cb
	}
	
	config.Name = name
	cb := NewCircuitBreaker(config)
	m.breakers[name] = cb
	return cb
}

// Get gets an existing circuit breaker
func (m *Manager) Get(name string) (*CircuitBreaker, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	cb, exists := m.breakers[name]
	return cb, exists
}

// List returns all circuit breakers
func (m *Manager) List() map[string]*CircuitBreaker {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	result := make(map[string]*CircuitBreaker)
	for name, cb := range m.breakers {
		result[name] = cb
	}
	return result
}

// WithCircuitBreaker is a convenience function for executing with circuit breaker protection
func WithCircuitBreaker(ctx context.Context, name string, fn func() error) error {
	// Use default circuit breaker manager
	manager := NewManager()
	cb := manager.GetOrCreate(name, DefaultConfig(name))
	return cb.Execute(ctx, fn)
}

// Retry executes a function with retry logic and circuit breaker protection
func Retry(ctx context.Context, name string, maxRetries int, backoff time.Duration, fn func() error) error {
	manager := NewManager()
	cb := manager.GetOrCreate(name, DefaultConfig(name))
	
	var lastErr error
	for i := 0; i <= maxRetries; i++ {
		err := cb.Execute(ctx, fn)
		if err == nil {
			return nil
		}
		
		lastErr = err
		
		// Don't retry if circuit breaker is open
		if cb.IsOpen() {
			return err
		}
		
		// Don't retry on the last attempt
		if i == maxRetries {
			break
		}
		
		// Wait before retry
		select {
		case <-time.After(backoff):
			backoff *= 2 // Exponential backoff
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	
	return lastErr
}