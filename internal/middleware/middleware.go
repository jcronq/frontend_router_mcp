package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/jcronq/frontend_router_mcp/internal/logging"
	"github.com/jcronq/frontend_router_mcp/internal/metrics"
)

// RequestIDMiddleware adds a request ID to the context
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get request ID from header or generate a new one
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = logging.GenerateRequestID()
		}
		
		// Add request ID to response header
		w.Header().Set("X-Request-ID", requestID)
		
		// Add request ID to context
		ctx := logging.WithRequestIDContext(r.Context(), requestID)
		r = r.WithContext(ctx)
		
		next.ServeHTTP(w, r)
	})
}

// TracingMiddleware adds tracing information to the context
func TracingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get or generate trace ID
		traceID := r.Header.Get("X-Trace-ID")
		if traceID == "" {
			traceID = logging.GenerateTraceID()
		}
		
		// Generate span ID
		spanID := logging.GenerateRequestID()[:8]
		
		// Add trace headers to response
		w.Header().Set("X-Trace-ID", traceID)
		w.Header().Set("X-Span-ID", spanID)
		
		// Add to context
		ctx := logging.WithTraceIDContext(r.Context(), traceID)
		ctx = context.WithValue(ctx, logging.SpanIDKey, spanID)
		r = r.WithContext(ctx)
		
		next.ServeHTTP(w, r)
	})
}

// LoggingMiddleware adds structured logging to HTTP requests
func LoggingMiddleware(logger *logging.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			
			// Get request context information
			requestID := logging.GetRequestID(r.Context())
			traceID := logging.GetTraceID(r.Context())
			spanID := ""
			if sid, ok := r.Context().Value(logging.SpanIDKey).(string); ok {
				spanID = sid
			}
			
			// Create request logger
			requestLogger := logger.
				WithRequestID(requestID).
				WithTraceID(traceID).
				WithSpanID(spanID).
				WithComponent("http").
				WithFields(map[string]interface{}{
					"method":     r.Method,
					"path":       r.URL.Path,
					"user_agent": r.Header.Get("User-Agent"),
					"client_ip":  getClientIP(r),
				})
			
			// Add logger to context
			ctx := logging.ToContext(r.Context(), requestLogger)
			r = r.WithContext(ctx)
			
			// Log request start
			requestLogger.Infof("HTTP request started: %s %s", r.Method, r.URL.Path)
			
			// Create response writer wrapper
			rw := &responseWriter{
				ResponseWriter: w,
				statusCode:     200,
				size:          0,
			}
			
			// Process request
			next.ServeHTTP(rw, r)
			
			// Log request completion
			duration := time.Since(start)
			requestLogger.LogHTTP(
				r.Method,
				r.URL.Path,
				r.Header.Get("User-Agent"),
				getClientIP(r),
				rw.statusCode,
				duration,
			)
		})
	}
}

// MetricsMiddleware adds metrics collection to HTTP requests
func MetricsMiddleware(metrics *metrics.Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			
			// Increment in-flight requests
			metrics.SetHTTPRequestsInFlight(1)
			defer func() {
				metrics.SetHTTPRequestsInFlight(-1)
			}()
			
			// Create response writer wrapper
			rw := &responseWriter{
				ResponseWriter: w,
				statusCode:     200,
				size:          0,
			}
			
			// Process request
			next.ServeHTTP(rw, r)
			
			// Record metrics
			duration := time.Since(start)
			endpoint := getEndpointName(r.URL.Path)
			metrics.RecordHTTPRequest(r.Method, endpoint, rw.statusCode, duration, rw.size)
		})
	}
}

// CORSMiddleware adds CORS headers
func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID, X-Trace-ID")
		w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID, X-Trace-ID")
		
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		
		next.ServeHTTP(w, r)
	})
}

// SecurityMiddleware adds security headers
func SecurityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		
		next.ServeHTTP(w, r)
	})
}

// RecoveryMiddleware recovers from panics and logs them
func RecoveryMiddleware(logger *logging.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					requestLogger := logging.FromContext(r.Context())
					requestLogger.WithField("panic", err).Error("HTTP request panic recovered")
					
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write([]byte(`{"error": "internal server error", "request_id": "` + logging.GetRequestID(r.Context()) + `"}`))
				}
			}()
			
			next.ServeHTTP(w, r)
		})
	}
}

// HealthCheckMiddleware bypasses other middleware for health checks
func HealthCheckMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip complex middleware for health checks
		if strings.HasPrefix(r.URL.Path, "/health") ||
			strings.HasPrefix(r.URL.Path, "/ready") ||
			strings.HasPrefix(r.URL.Path, "/live") ||
			strings.HasPrefix(r.URL.Path, "/metrics") {
			next.ServeHTTP(w, r)
			return
		}
		
		next.ServeHTTP(w, r)
	})
}

// RateLimitMiddleware provides basic rate limiting
func RateLimitMiddleware(maxRequests int, window time.Duration) func(http.Handler) http.Handler {
	type client struct {
		requests int
		window   time.Time
	}
	
	clients := make(map[string]*client)
	
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := getClientIP(r)
			now := time.Now()
			
			// Clean up old entries
			for k, v := range clients {
				if now.Sub(v.window) > window {
					delete(clients, k)
				}
			}
			
			// Check rate limit
			if c, exists := clients[ip]; exists {
				if now.Sub(c.window) < window {
					if c.requests >= maxRequests {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusTooManyRequests)
						w.Write([]byte(`{"error": "rate limit exceeded", "retry_after": ` + window.String() + `}`))
						return
					}
					c.requests++
				} else {
					c.requests = 1
					c.window = now
				}
			} else {
				clients[ip] = &client{requests: 1, window: now}
			}
			
			next.ServeHTTP(w, r)
		})
	}
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

// getClientIP extracts the client IP from the request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	
	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	
	// Fall back to remote address
	ip := r.RemoteAddr
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}
	
	return ip
}

// getEndpointName normalizes endpoint names for metrics
func getEndpointName(path string) string {
	// Remove trailing slash
	if len(path) > 1 && strings.HasSuffix(path, "/") {
		path = path[:len(path)-1]
	}
	
	// Normalize common paths
	switch {
	case strings.HasPrefix(path, "/health"):
		return "/health"
	case strings.HasPrefix(path, "/ready"):
		return "/ready"
	case strings.HasPrefix(path, "/live"):
		return "/live"
	case strings.HasPrefix(path, "/metrics"):
		return "/metrics"
	case strings.HasPrefix(path, "/ws"):
		return "/ws"
	default:
		return path
	}
}

// ChainMiddleware chains multiple middleware functions
func ChainMiddleware(middlewares ...func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		for i := len(middlewares) - 1; i >= 0; i-- {
			next = middlewares[i](next)
		}
		return next
	}
}