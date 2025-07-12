package health

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"time"
)

// Status represents the health status of a component
type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusUnhealthy Status = "unhealthy"
	StatusWarning   Status = "warning"
	StatusUnknown   Status = "unknown"
)

// ComponentHealth represents the health of a specific component
type ComponentHealth struct {
	Name        string            `json:"name"`
	Status      Status            `json:"status"`
	Message     string            `json:"message,omitempty"`
	LastChecked time.Time         `json:"last_checked"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// HealthReport represents the overall health report
type HealthReport struct {
	Status      Status                     `json:"status"`
	Timestamp   time.Time                  `json:"timestamp"`
	Version     string                     `json:"version"`
	Components  map[string]ComponentHealth `json:"components"`
	SystemInfo  SystemInfo                 `json:"system_info"`
	Uptime      time.Duration              `json:"uptime"`
	RequestID   string                     `json:"request_id,omitempty"`
}

// SystemInfo contains system-level information
type SystemInfo struct {
	Goroutines   int    `json:"goroutines"`
	MemoryUsage  int64  `json:"memory_usage_bytes"`
	CPUCount     int    `json:"cpu_count"`
	GoVersion    string `json:"go_version"`
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
}

// HealthChecker interface for components that can be health checked
type HealthChecker interface {
	HealthCheck(ctx context.Context) ComponentHealth
}

// Manager manages health checks for all components
type Manager struct {
	components map[string]HealthChecker
	startTime  time.Time
	version    string
	mu         sync.RWMutex
}

// NewManager creates a new health manager
func NewManager(version string) *Manager {
	return &Manager{
		components: make(map[string]HealthChecker),
		startTime:  time.Now(),
		version:    version,
	}
}

// RegisterComponent registers a component for health checking
func (m *Manager) RegisterComponent(name string, checker HealthChecker) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.components[name] = checker
}

// UnregisterComponent removes a component from health checking
func (m *Manager) UnregisterComponent(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.components, name)
}

// GetHealthReport generates a comprehensive health report
func (m *Manager) GetHealthReport(ctx context.Context, requestID string) *HealthReport {
	m.mu.RLock()
	defer m.mu.RUnlock()

	report := &HealthReport{
		Status:     StatusHealthy,
		Timestamp:  time.Now(),
		Version:    m.version,
		Components: make(map[string]ComponentHealth),
		SystemInfo: m.getSystemInfo(),
		Uptime:     time.Since(m.startTime),
		RequestID:  requestID,
	}

	// Check all components
	for name, checker := range m.components {
		componentHealth := checker.HealthCheck(ctx)
		report.Components[name] = componentHealth

		// Update overall status based on component status
		if componentHealth.Status == StatusUnhealthy {
			report.Status = StatusUnhealthy
		} else if componentHealth.Status == StatusWarning && report.Status == StatusHealthy {
			report.Status = StatusWarning
		}
	}

	return report
}

// IsReady checks if all components are ready (healthy or warning)
func (m *Manager) IsReady(ctx context.Context) bool {
	report := m.GetHealthReport(ctx, "")
	for _, component := range report.Components {
		if component.Status == StatusUnhealthy || component.Status == StatusUnknown {
			return false
		}
	}
	return true
}

// IsLive checks if the service is live (at least one component is healthy)
func (m *Manager) IsLive(ctx context.Context) bool {
	report := m.GetHealthReport(ctx, "")
	if len(report.Components) == 0 {
		return true // If no components registered, assume live
	}
	
	for _, component := range report.Components {
		if component.Status == StatusHealthy || component.Status == StatusWarning {
			return true
		}
	}
	return false
}

// getSystemInfo collects system information
func (m *Manager) getSystemInfo() SystemInfo {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return SystemInfo{
		Goroutines:   runtime.NumGoroutine(),
		MemoryUsage:  int64(memStats.Alloc),
		CPUCount:     runtime.NumCPU(),
		GoVersion:    runtime.Version(),
		OS:           runtime.GOOS,
		Architecture: runtime.GOARCH,
	}
}

// HTTPHandler creates HTTP handlers for health endpoints
func (m *Manager) HTTPHandler() http.Handler {
	mux := http.NewServeMux()
	
	// Health endpoint - detailed health information
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = fmt.Sprintf("health-%d", time.Now().UnixNano())
		}
		
		report := m.GetHealthReport(ctx, requestID)
		
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-ID", requestID)
		
		// Set HTTP status based on overall health
		switch report.Status {
		case StatusHealthy:
			w.WriteHeader(http.StatusOK)
		case StatusWarning:
			w.WriteHeader(http.StatusOK)
		case StatusUnhealthy:
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		
		json.NewEncoder(w).Encode(report)
	})
	
	// Ready endpoint - simple readiness check
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = fmt.Sprintf("ready-%d", time.Now().UnixNano())
		}
		
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-ID", requestID)
		
		if m.IsReady(ctx) {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status": "ready",
				"timestamp": time.Now(),
				"request_id": requestID,
			})
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status": "not_ready",
				"timestamp": time.Now(),
				"request_id": requestID,
			})
		}
	})
	
	// Live endpoint - simple liveness check
	mux.HandleFunc("/live", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = fmt.Sprintf("live-%d", time.Now().UnixNano())
		}
		
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-ID", requestID)
		
		if m.IsLive(ctx) {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status": "alive",
				"timestamp": time.Now(),
				"request_id": requestID,
			})
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status": "not_alive",
				"timestamp": time.Now(),
				"request_id": requestID,
			})
		}
	})
	
	return mux
}

// WebSocketHealthChecker implements health checking for WebSocket server
type WebSocketHealthChecker struct {
	server interface {
		GetClientCount() int
		IsRunning() bool
	}
}

// NewWebSocketHealthChecker creates a new WebSocket health checker
func NewWebSocketHealthChecker(server interface {
	GetClientCount() int
	IsRunning() bool
}) *WebSocketHealthChecker {
	return &WebSocketHealthChecker{server: server}
}

// HealthCheck performs health check for WebSocket server
func (w *WebSocketHealthChecker) HealthCheck(ctx context.Context) ComponentHealth {
	if !w.server.IsRunning() {
		return ComponentHealth{
			Name:        "websocket_server",
			Status:      StatusUnhealthy,
			Message:     "WebSocket server is not running",
			LastChecked: time.Now(),
		}
	}
	
	clientCount := w.server.GetClientCount()
	return ComponentHealth{
		Name:        "websocket_server",
		Status:      StatusHealthy,
		Message:     "WebSocket server is running",
		LastChecked: time.Now(),
		Metadata: map[string]string{
			"client_count": fmt.Sprintf("%d", clientCount),
		},
	}
}

// MCPHealthChecker implements health checking for MCP server
type MCPHealthChecker struct {
	server interface {
		IsRunning() bool
	}
}

// NewMCPHealthChecker creates a new MCP health checker
func NewMCPHealthChecker(server interface {
	IsRunning() bool
}) *MCPHealthChecker {
	return &MCPHealthChecker{server: server}
}

// HealthCheck performs health check for MCP server
func (m *MCPHealthChecker) HealthCheck(ctx context.Context) ComponentHealth {
	if !m.server.IsRunning() {
		return ComponentHealth{
			Name:        "mcp_server",
			Status:      StatusUnhealthy,
			Message:     "MCP server is not running",
			LastChecked: time.Now(),
		}
	}
	
	return ComponentHealth{
		Name:        "mcp_server",
		Status:      StatusHealthy,
		Message:     "MCP server is running",
		LastChecked: time.Now(),
	}
}

// ChannelHealthChecker implements health checking for communication channels
type ChannelHealthChecker struct {
	channels map[string]interface{
		Len() int
		Cap() int
	}
}

// NewChannelHealthChecker creates a new channel health checker
func NewChannelHealthChecker() *ChannelHealthChecker {
	return &ChannelHealthChecker{
		channels: make(map[string]interface{
			Len() int
			Cap() int
		}),
	}
}

// RegisterChannel registers a channel for health monitoring
func (c *ChannelHealthChecker) RegisterChannel(name string, ch interface{
	Len() int
	Cap() int
}) {
	c.channels[name] = ch
}

// HealthCheck performs health check for communication channels
func (c *ChannelHealthChecker) HealthCheck(ctx context.Context) ComponentHealth {
	metadata := make(map[string]string)
	status := StatusHealthy
	message := "All channels are healthy"
	
	for name, ch := range c.channels {
		length := ch.Len()
		capacity := ch.Cap()
		utilization := float64(length) / float64(capacity) * 100
		
		metadata[fmt.Sprintf("%s_length", name)] = fmt.Sprintf("%d", length)
		metadata[fmt.Sprintf("%s_capacity", name)] = fmt.Sprintf("%d", capacity)
		metadata[fmt.Sprintf("%s_utilization", name)] = fmt.Sprintf("%.2f%%", utilization)
		
		// Mark as warning if utilization is high
		if utilization > 80 {
			status = StatusWarning
			message = fmt.Sprintf("Channel %s utilization is high: %.2f%%", name, utilization)
		}
		
		// Mark as unhealthy if channel is full
		if length >= capacity {
			status = StatusUnhealthy
			message = fmt.Sprintf("Channel %s is full", name)
			break
		}
	}
	
	return ComponentHealth{
		Name:        "communication_channels",
		Status:      status,
		Message:     message,
		LastChecked: time.Now(),
		Metadata:    metadata,
	}
}