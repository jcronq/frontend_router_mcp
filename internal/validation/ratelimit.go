package validation

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimiter provides rate limiting functionality for WebSocket connections and HTTP requests
type RateLimiter struct {
	mu      sync.RWMutex
	clients map[string]*ClientRateLimit
	global  *rate.Limiter
	config  RateLimitConfig
}

// RateLimitConfig configures rate limiting parameters
type RateLimitConfig struct {
	// Global rate limits
	GlobalRPS   int           // Global requests per second
	GlobalBurst int           // Global burst capacity
	
	// Per-client rate limits
	ClientRPS     int           // Per-client requests per second
	ClientBurst   int           // Per-client burst capacity
	ClientWindow  time.Duration // Window for tracking client requests
	
	// Connection limits
	MaxConnections    int           // Maximum concurrent connections
	MaxConnectionsPerIP int         // Maximum connections per IP
	
	// Message limits
	MaxMessageRate    int           // Maximum messages per second per client
	MaxMessageBurst   int           // Maximum message burst per client
	
	// Cleanup settings
	CleanupInterval   time.Duration // How often to clean up old entries
	ClientExpiry      time.Duration // How long to keep client entries
	
	// Blocked IP settings
	BlockedIPTTL      time.Duration // How long to keep blocked IPs
	AutoBlockThreshold int          // Requests to trigger auto-block
	AutoBlockDuration  time.Duration // Duration of auto-block
}

// DefaultRateLimitConfig provides secure default rate limiting configuration
var DefaultRateLimitConfig = RateLimitConfig{
	GlobalRPS:           1000,
	GlobalBurst:         2000,
	ClientRPS:           100,
	ClientBurst:         200,
	ClientWindow:        time.Minute,
	MaxConnections:      500,
	MaxConnectionsPerIP: 10,
	MaxMessageRate:      50,
	MaxMessageBurst:     100,
	CleanupInterval:     5 * time.Minute,
	ClientExpiry:        30 * time.Minute,
	BlockedIPTTL:        time.Hour,
	AutoBlockThreshold:  1000,
	AutoBlockDuration:   10 * time.Minute,
}

// ClientRateLimit tracks rate limiting for individual clients
type ClientRateLimit struct {
	ID              string
	IP              string
	RequestLimiter  *rate.Limiter
	MessageLimiter  *rate.Limiter
	ConnectionCount int
	LastSeen        time.Time
	TotalRequests   int64
	TotalMessages   int64
	ViolationCount  int
	IsBlocked       bool
	BlockedUntil    time.Time
	CreatedAt       time.Time
}

// BlockedIP represents a blocked IP address
type BlockedIP struct {
	IP        string
	BlockedAt time.Time
	ExpiresAt time.Time
	Reason    string
	Count     int
}

// NewRateLimiter creates a new rate limiter with the specified configuration
func NewRateLimiter(config RateLimitConfig) *RateLimiter {
	rl := &RateLimiter{
		clients: make(map[string]*ClientRateLimit),
		global:  rate.NewLimiter(rate.Limit(config.GlobalRPS), config.GlobalBurst),
		config:  config,
	}
	
	// Start cleanup goroutine
	go rl.cleanupLoop()
	
	return rl
}

// AllowRequest checks if a request should be allowed based on rate limiting
func (rl *RateLimiter) AllowRequest(clientID, clientIP string) (bool, error) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	// Check global rate limit first
	if !rl.global.Allow() {
		return false, fmt.Errorf("global rate limit exceeded")
	}
	
	// Get or create client rate limit
	client, exists := rl.clients[clientID]
	if !exists {
		client = &ClientRateLimit{
			ID:             clientID,
			IP:             clientIP,
			RequestLimiter: rate.NewLimiter(rate.Limit(rl.config.ClientRPS), rl.config.ClientBurst),
			MessageLimiter: rate.NewLimiter(rate.Limit(rl.config.MaxMessageRate), rl.config.MaxMessageBurst),
			LastSeen:       time.Now(),
			CreatedAt:      time.Now(),
		}
		rl.clients[clientID] = client
	}
	
	// Update last seen
	client.LastSeen = time.Now()
	
	// Check if client is blocked
	if client.IsBlocked && time.Now().Before(client.BlockedUntil) {
		return false, fmt.Errorf("client %s is blocked until %v", clientID, client.BlockedUntil)
	} else if client.IsBlocked && time.Now().After(client.BlockedUntil) {
		// Unblock client
		client.IsBlocked = false
		client.BlockedUntil = time.Time{}
	}
	
	// Check client rate limit
	if !client.RequestLimiter.Allow() {
		client.ViolationCount++
		
		// Auto-block if too many violations
		if client.ViolationCount >= rl.config.AutoBlockThreshold {
			client.IsBlocked = true
			client.BlockedUntil = time.Now().Add(rl.config.AutoBlockDuration)
			return false, fmt.Errorf("client %s auto-blocked due to rate limit violations", clientID)
		}
		
		return false, fmt.Errorf("client rate limit exceeded for %s", clientID)
	}
	
	// Check connections per IP
	if rl.countConnectionsForIP(clientIP) >= rl.config.MaxConnectionsPerIP {
		return false, fmt.Errorf("too many connections from IP %s", clientIP)
	}
	
	client.TotalRequests++
	return true, nil
}

// AllowMessage checks if a message should be allowed based on rate limiting
func (rl *RateLimiter) AllowMessage(clientID string) (bool, error) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	client, exists := rl.clients[clientID]
	if !exists {
		return false, fmt.Errorf("client %s not found", clientID)
	}
	
	// Check if client is blocked
	if client.IsBlocked && time.Now().Before(client.BlockedUntil) {
		return false, fmt.Errorf("client %s is blocked", clientID)
	}
	
	// Check message rate limit
	if !client.MessageLimiter.Allow() {
		client.ViolationCount++
		return false, fmt.Errorf("message rate limit exceeded for client %s", clientID)
	}
	
	client.TotalMessages++
	return true, nil
}

// AddConnection adds a connection for a client
func (rl *RateLimiter) AddConnection(clientID, clientIP string) error {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	// Check global connection limit
	if rl.getTotalConnections() >= rl.config.MaxConnections {
		return fmt.Errorf("maximum connections exceeded")
	}
	
	// Check connections per IP
	if rl.countConnectionsForIP(clientIP) >= rl.config.MaxConnectionsPerIP {
		return fmt.Errorf("too many connections from IP %s", clientIP)
	}
	
	client, exists := rl.clients[clientID]
	if !exists {
		client = &ClientRateLimit{
			ID:             clientID,
			IP:             clientIP,
			RequestLimiter: rate.NewLimiter(rate.Limit(rl.config.ClientRPS), rl.config.ClientBurst),
			MessageLimiter: rate.NewLimiter(rate.Limit(rl.config.MaxMessageRate), rl.config.MaxMessageBurst),
			LastSeen:       time.Now(),
			CreatedAt:      time.Now(),
		}
		rl.clients[clientID] = client
	}
	
	client.ConnectionCount++
	client.LastSeen = time.Now()
	
	return nil
}

// RemoveConnection removes a connection for a client
func (rl *RateLimiter) RemoveConnection(clientID string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	if client, exists := rl.clients[clientID]; exists {
		client.ConnectionCount--
		if client.ConnectionCount < 0 {
			client.ConnectionCount = 0
		}
		client.LastSeen = time.Now()
	}
}

// BlockClient blocks a client for a specified duration
func (rl *RateLimiter) BlockClient(clientID string, duration time.Duration, reason string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	if client, exists := rl.clients[clientID]; exists {
		client.IsBlocked = true
		client.BlockedUntil = time.Now().Add(duration)
	}
}

// UnblockClient unblocks a client
func (rl *RateLimiter) UnblockClient(clientID string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	if client, exists := rl.clients[clientID]; exists {
		client.IsBlocked = false
		client.BlockedUntil = time.Time{}
	}
}

// GetClientStats returns statistics for a client
func (rl *RateLimiter) GetClientStats(clientID string) (*ClientRateLimit, bool) {
	rl.mu.RLock()
	defer rl.mu.RUnlock()
	
	client, exists := rl.clients[clientID]
	if !exists {
		return nil, false
	}
	
	// Return a copy to prevent external modification
	return &ClientRateLimit{
		ID:              client.ID,
		IP:              client.IP,
		ConnectionCount: client.ConnectionCount,
		LastSeen:        client.LastSeen,
		TotalRequests:   client.TotalRequests,
		TotalMessages:   client.TotalMessages,
		ViolationCount:  client.ViolationCount,
		IsBlocked:       client.IsBlocked,
		BlockedUntil:    client.BlockedUntil,
		CreatedAt:       client.CreatedAt,
	}, true
}

// GetAllClientsStats returns statistics for all clients
func (rl *RateLimiter) GetAllClientsStats() map[string]*ClientRateLimit {
	rl.mu.RLock()
	defer rl.mu.RUnlock()
	
	stats := make(map[string]*ClientRateLimit)
	for id, client := range rl.clients {
		stats[id] = &ClientRateLimit{
			ID:              client.ID,
			IP:              client.IP,
			ConnectionCount: client.ConnectionCount,
			LastSeen:        client.LastSeen,
			TotalRequests:   client.TotalRequests,
			TotalMessages:   client.TotalMessages,
			ViolationCount:  client.ViolationCount,
			IsBlocked:       client.IsBlocked,
			BlockedUntil:    client.BlockedUntil,
			CreatedAt:       client.CreatedAt,
		}
	}
	
	return stats
}

// HTTPMiddleware returns an HTTP middleware that applies rate limiting
func (rl *RateLimiter) HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP := getClientIP(r)
		clientID := getClientID(r)
		
		allowed, err := rl.AllowRequest(clientID, clientIP)
		if !allowed {
			http.Error(w, fmt.Sprintf("Rate limit exceeded: %v", err), http.StatusTooManyRequests)
			return
		}
		
		next.ServeHTTP(w, r)
	})
}

// WebSocketMiddleware returns a WebSocket middleware that applies rate limiting
func (rl *RateLimiter) WebSocketMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP := getClientIP(r)
		clientID := getClientID(r)
		
		// Check if we can add a new connection
		if err := rl.AddConnection(clientID, clientIP); err != nil {
			http.Error(w, fmt.Sprintf("Connection limit exceeded: %v", err), http.StatusTooManyRequests)
			return
		}
		
		// Remove connection when done
		defer rl.RemoveConnection(clientID)
		
		next.ServeHTTP(w, r)
	})
}

// Cleanup removes expired entries
func (rl *RateLimiter) Cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	now := time.Now()
	for clientID, client := range rl.clients {
		// Remove clients that haven't been seen for a while and have no connections
		if now.Sub(client.LastSeen) > rl.config.ClientExpiry && client.ConnectionCount == 0 {
			delete(rl.clients, clientID)
		}
	}
}

// Helper methods

func (rl *RateLimiter) getTotalConnections() int {
	total := 0
	for _, client := range rl.clients {
		total += client.ConnectionCount
	}
	return total
}

func (rl *RateLimiter) countConnectionsForIP(ip string) int {
	count := 0
	for _, client := range rl.clients {
		if client.IP == ip {
			count += client.ConnectionCount
		}
	}
	return count
}

func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.config.CleanupInterval)
	defer ticker.Stop()
	
	for range ticker.C {
		rl.Cleanup()
	}
}

// Utility functions

func getClientIP(r *http.Request) string {
	// Try to get the real IP from various headers
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Client-IP"); ip != "" {
		return ip
	}
	
	// Fall back to remote address
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func getClientID(r *http.Request) string {
	// Try to get client ID from various sources
	if id := r.Header.Get("X-Client-ID"); id != "" {
		return id
	}
	if id := r.URL.Query().Get("client_id"); id != "" {
		return id
	}
	
	// Fall back to IP-based ID
	return fmt.Sprintf("ip-%s", getClientIP(r))
}

// RateLimitedContext provides rate limiting information in context
type RateLimitedContext struct {
	ClientID    string
	ClientIP    string
	RemainingRequests int
	RemainingMessages int
	ResetTime   time.Time
}

// WithRateLimitContext adds rate limiting information to context
func (rl *RateLimiter) WithRateLimitContext(ctx context.Context, clientID, clientIP string) context.Context {
	rl.mu.RLock()
	defer rl.mu.RUnlock()
	
	client, exists := rl.clients[clientID]
	if !exists {
		return ctx
	}
	
	rateLimitCtx := &RateLimitedContext{
		ClientID:    clientID,
		ClientIP:    clientIP,
		ResetTime:   time.Now().Add(rl.config.ClientWindow),
	}
	
	// Calculate remaining requests/messages based on rate limiter state
	if client.RequestLimiter != nil {
		rateLimitCtx.RemainingRequests = int(client.RequestLimiter.Tokens())
	}
	if client.MessageLimiter != nil {
		rateLimitCtx.RemainingMessages = int(client.MessageLimiter.Tokens())
	}
	
	return context.WithValue(ctx, "rate_limit", rateLimitCtx)
}

// GetRateLimitContext extracts rate limiting information from context
func GetRateLimitContext(ctx context.Context) (*RateLimitedContext, bool) {
	rateLimitCtx, ok := ctx.Value("rate_limit").(*RateLimitedContext)
	return rateLimitCtx, ok
}