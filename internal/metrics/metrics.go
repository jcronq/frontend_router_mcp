package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds all Prometheus metrics
type Metrics struct {
	// HTTP metrics
	HTTPRequestsTotal     *prometheus.CounterVec
	HTTPRequestDuration   *prometheus.HistogramVec
	HTTPResponseSize      *prometheus.HistogramVec
	HTTPRequestsInFlight  prometheus.Gauge

	// WebSocket metrics
	WebSocketConnections     prometheus.Gauge
	WebSocketMessagesTotal   *prometheus.CounterVec
	WebSocketMessageSize     *prometheus.HistogramVec
	WebSocketConnectionsTotal *prometheus.CounterVec

	// MCP metrics
	MCPRequestsTotal      *prometheus.CounterVec
	MCPRequestDuration    *prometheus.HistogramVec
	MCPToolExecutions     *prometheus.CounterVec
	MCPActiveRequests     prometheus.Gauge

	// System metrics
	SystemInfo            *prometheus.GaugeVec
	GoroutinesActive      prometheus.Gauge
	MemoryUsage           prometheus.Gauge
	ChannelUtilization    *prometheus.GaugeVec

	// Business metrics
	UserQuestionsTotal    *prometheus.CounterVec
	UserResponseTime      *prometheus.HistogramVec
	ActiveUserSessions    prometheus.Gauge
}

// NewMetrics creates a new metrics instance
func NewMetrics() *Metrics {
	return &Metrics{
		// HTTP metrics
		HTTPRequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "http_requests_total",
				Help: "Total number of HTTP requests",
			},
			[]string{"method", "endpoint", "status_code"},
		),
		HTTPRequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "http_request_duration_seconds",
				Help:    "HTTP request duration in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"method", "endpoint", "status_code"},
		),
		HTTPResponseSize: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "http_response_size_bytes",
				Help:    "HTTP response size in bytes",
				Buckets: prometheus.ExponentialBuckets(100, 10, 8),
			},
			[]string{"method", "endpoint", "status_code"},
		),
		HTTPRequestsInFlight: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "http_requests_in_flight",
				Help: "Number of HTTP requests currently being processed",
			},
		),

		// WebSocket metrics
		WebSocketConnections: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "websocket_connections_active",
				Help: "Number of active WebSocket connections",
			},
		),
		WebSocketMessagesTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "websocket_messages_total",
				Help: "Total number of WebSocket messages",
			},
			[]string{"direction", "message_type"},
		),
		WebSocketMessageSize: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "websocket_message_size_bytes",
				Help:    "WebSocket message size in bytes",
				Buckets: prometheus.ExponentialBuckets(64, 4, 10),
			},
			[]string{"direction", "message_type"},
		),
		WebSocketConnectionsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "websocket_connections_total",
				Help: "Total number of WebSocket connections",
			},
			[]string{"event"},
		),

		// MCP metrics
		MCPRequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "mcp_requests_total",
				Help: "Total number of MCP requests",
			},
			[]string{"method", "status"},
		),
		MCPRequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "mcp_request_duration_seconds",
				Help:    "MCP request duration in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"method", "status"},
		),
		MCPToolExecutions: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "mcp_tool_executions_total",
				Help: "Total number of MCP tool executions",
			},
			[]string{"tool_name", "status"},
		),
		MCPActiveRequests: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "mcp_active_requests",
				Help: "Number of active MCP requests",
			},
		),

		// System metrics
		SystemInfo: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "system_info",
				Help: "System information",
			},
			[]string{"version", "go_version", "os", "arch"},
		),
		GoroutinesActive: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "goroutines_active",
				Help: "Number of active goroutines",
			},
		),
		MemoryUsage: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "memory_usage_bytes",
				Help: "Memory usage in bytes",
			},
		),
		ChannelUtilization: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "channel_utilization_percent",
				Help: "Channel utilization percentage",
			},
			[]string{"channel_name"},
		),

		// Business metrics
		UserQuestionsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "user_questions_total",
				Help: "Total number of user questions",
			},
			[]string{"status"},
		),
		UserResponseTime: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "user_response_time_seconds",
				Help:    "User response time in seconds",
				Buckets: []float64{1, 5, 10, 30, 60, 300, 600, 1800},
			},
			[]string{"status"},
		),
		ActiveUserSessions: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "active_user_sessions",
				Help: "Number of active user sessions",
			},
		),
	}
}

// RecordHTTPRequest records HTTP request metrics
func (m *Metrics) RecordHTTPRequest(method, endpoint string, statusCode int, duration time.Duration, responseSize int) {
	labels := prometheus.Labels{
		"method":      method,
		"endpoint":    endpoint,
		"status_code": strconv.Itoa(statusCode),
	}
	
	m.HTTPRequestsTotal.With(labels).Inc()
	m.HTTPRequestDuration.With(labels).Observe(duration.Seconds())
	m.HTTPResponseSize.With(labels).Observe(float64(responseSize))
}

// RecordWebSocketConnection records WebSocket connection metrics
func (m *Metrics) RecordWebSocketConnection(event string) {
	m.WebSocketConnectionsTotal.With(prometheus.Labels{"event": event}).Inc()
	
	switch event {
	case "connect":
		m.WebSocketConnections.Inc()
	case "disconnect":
		m.WebSocketConnections.Dec()
	}
}

// RecordWebSocketMessage records WebSocket message metrics
func (m *Metrics) RecordWebSocketMessage(direction, messageType string, size int) {
	labels := prometheus.Labels{
		"direction":    direction,
		"message_type": messageType,
	}
	
	m.WebSocketMessagesTotal.With(labels).Inc()
	m.WebSocketMessageSize.With(labels).Observe(float64(size))
}

// RecordMCPRequest records MCP request metrics
func (m *Metrics) RecordMCPRequest(method, status string, duration time.Duration) {
	labels := prometheus.Labels{
		"method": method,
		"status": status,
	}
	
	m.MCPRequestsTotal.With(labels).Inc()
	m.MCPRequestDuration.With(labels).Observe(duration.Seconds())
}

// RecordMCPToolExecution records MCP tool execution metrics
func (m *Metrics) RecordMCPToolExecution(toolName, status string) {
	m.MCPToolExecutions.With(prometheus.Labels{
		"tool_name": toolName,
		"status":    status,
	}).Inc()
}

// RecordUserQuestion records user question metrics
func (m *Metrics) RecordUserQuestion(status string, responseTime time.Duration) {
	m.UserQuestionsTotal.With(prometheus.Labels{"status": status}).Inc()
	m.UserResponseTime.With(prometheus.Labels{"status": status}).Observe(responseTime.Seconds())
}

// UpdateSystemMetrics updates system-level metrics
func (m *Metrics) UpdateSystemMetrics(info SystemInfo) {
	m.SystemInfo.With(prometheus.Labels{
		"version":    info.Version,
		"go_version": info.GoVersion,
		"os":         info.OS,
		"arch":       info.Architecture,
	}).Set(1)
	
	m.GoroutinesActive.Set(float64(info.Goroutines))
	m.MemoryUsage.Set(float64(info.MemoryUsage))
}

// UpdateChannelMetrics updates channel utilization metrics
func (m *Metrics) UpdateChannelMetrics(channelName string, utilization float64) {
	m.ChannelUtilization.With(prometheus.Labels{
		"channel_name": channelName,
	}).Set(utilization)
}

// SetActiveUserSessions sets the number of active user sessions
func (m *Metrics) SetActiveUserSessions(count int) {
	m.ActiveUserSessions.Set(float64(count))
}

// SetMCPActiveRequests sets the number of active MCP requests
func (m *Metrics) SetMCPActiveRequests(count int) {
	m.MCPActiveRequests.Set(float64(count))
}

// SetHTTPRequestsInFlight sets the number of HTTP requests in flight
func (m *Metrics) SetHTTPRequestsInFlight(count int) {
	m.HTTPRequestsInFlight.Set(float64(count))
}

// SystemInfo contains system information for metrics
type SystemInfo struct {
	Version      string
	GoVersion    string
	OS           string
	Architecture string
	Goroutines   int
	MemoryUsage  int64
}

// Handler returns the Prometheus metrics HTTP handler
func (m *Metrics) Handler() http.Handler {
	return promhttp.Handler()
}

// InstrumentHTTPHandler wraps an HTTP handler with metrics instrumentation
func (m *Metrics) InstrumentHTTPHandler(endpoint string, handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		m.SetHTTPRequestsInFlight(1)
		
		// Create a response writer that captures the status code and response size
		rw := &responseWriter{
			ResponseWriter: w,
			statusCode:     200,
			size:          0,
		}
		
		// Call the actual handler
		handler.ServeHTTP(rw, r)
		
		// Record metrics
		duration := time.Since(start)
		m.RecordHTTPRequest(r.Method, endpoint, rw.statusCode, duration, rw.size)
		m.SetHTTPRequestsInFlight(-1)
	})
}

// responseWriter wraps http.ResponseWriter to capture status code and response size
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	size       int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(data []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(data)
	rw.size += n
	return n, err
}

// StartMetricsCollector starts a goroutine that periodically collects system metrics
func (m *Metrics) StartMetricsCollector(interval time.Duration, systemInfo SystemInfo) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		
		for range ticker.C {
			m.UpdateSystemMetrics(systemInfo)
		}
	}()
}