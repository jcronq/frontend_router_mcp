package validation

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// RequestValidator provides request size validation and timeout handling
type RequestValidator struct {
	config RequestValidationConfig
}

// RequestValidationConfig configures request validation parameters
type RequestValidationConfig struct {
	// Size limits
	MaxRequestSize     int64         // Maximum HTTP request size in bytes
	MaxHeaderSize      int64         // Maximum header size in bytes
	MaxBodySize        int64         // Maximum body size in bytes
	MaxQueryLength     int           // Maximum query string length
	MaxPathLength      int           // Maximum URL path length
	MaxCookieSize      int           // Maximum cookie size in bytes
	MaxFormDataSize    int64         // Maximum form data size in bytes
	MaxJSONSize        int64         // Maximum JSON payload size in bytes
	MaxWebSocketMsg    int           // Maximum WebSocket message size in bytes
	MaxWebSocketFrame  int           // Maximum WebSocket frame size in bytes
	
	// Timeout settings
	RequestTimeout     time.Duration // Maximum request processing time
	HeaderTimeout      time.Duration // Maximum time to read headers
	BodyTimeout        time.Duration // Maximum time to read body
	WebSocketTimeout   time.Duration // Maximum WebSocket message timeout
	IdleTimeout        time.Duration // Maximum idle connection time
	
	// Connection limits
	MaxConnections     int           // Maximum concurrent connections
	MaxConnectionsPerIP int          // Maximum connections per IP address
	
	// Content validation
	AllowedMethods     []string      // Allowed HTTP methods
	AllowedContentTypes []string     // Allowed content types
	RequiredHeaders    []string      // Required headers
	BlockedUserAgents  []string      // Blocked user agents
	
	// Security settings
	BlockSuspiciousRequests bool      // Block requests with suspicious patterns
	ValidateContentLength   bool      // Validate Content-Length header
	RequireContentType      bool      // Require Content-Type header
	StrictHostValidation    bool      // Strict Host header validation
	AllowedHosts           []string   // Allowed Host header values
}

// DefaultRequestValidationConfig provides secure default configuration
var DefaultRequestValidationConfig = RequestValidationConfig{
	MaxRequestSize:     1024 * 1024,     // 1MB
	MaxHeaderSize:      8192,            // 8KB
	MaxBodySize:        512 * 1024,      // 512KB
	MaxQueryLength:     2048,            // 2KB
	MaxPathLength:      1024,            // 1KB
	MaxCookieSize:      4096,            // 4KB
	MaxFormDataSize:    1024 * 1024,     // 1MB
	MaxJSONSize:        256 * 1024,      // 256KB
	MaxWebSocketMsg:    65536,           // 64KB
	MaxWebSocketFrame:  16384,           // 16KB
	RequestTimeout:     30 * time.Second,
	HeaderTimeout:      10 * time.Second,
	BodyTimeout:        20 * time.Second,
	WebSocketTimeout:   60 * time.Second,
	IdleTimeout:        120 * time.Second,
	MaxConnections:     1000,
	MaxConnectionsPerIP: 10,
	AllowedMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "HEAD"},
	AllowedContentTypes: []string{
		"application/json",
		"application/x-www-form-urlencoded",
		"multipart/form-data",
		"text/plain",
	},
	RequiredHeaders:         []string{"User-Agent"},
	BlockedUserAgents:       []string{"bot", "crawler", "spider", "scraper"},
	BlockSuspiciousRequests: true,
	ValidateContentLength:   true,
	RequireContentType:      true,
	StrictHostValidation:    true,
	AllowedHosts:           []string{"localhost", "127.0.0.1"},
}

// RequestValidationError represents a request validation error
type RequestValidationError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
	Value   string `json:"value,omitempty"`
}

func (e *RequestValidationError) Error() string {
	return e.Message
}

// NewRequestValidator creates a new request validator
func NewRequestValidator(config RequestValidationConfig) *RequestValidator {
	return &RequestValidator{
		config: config,
	}
}

// ValidateHTTPRequest validates an HTTP request
func (rv *RequestValidator) ValidateHTTPRequest(r *http.Request) error {
	// Validate request method
	if err := rv.validateMethod(r.Method); err != nil {
		return err
	}
	
	// Validate URL path
	if err := rv.validatePath(r.URL.Path); err != nil {
		return err
	}
	
	// Validate query string
	if err := rv.validateQuery(r.URL.RawQuery); err != nil {
		return err
	}
	
	// Validate headers
	if err := rv.validateHeaders(r.Header); err != nil {
		return err
	}
	
	// Validate Host header
	if err := rv.validateHost(r.Host); err != nil {
		return err
	}
	
	// Validate User-Agent
	if err := rv.validateUserAgent(r.UserAgent()); err != nil {
		return err
	}
	
	// Validate Content-Length
	if err := rv.validateContentLength(r); err != nil {
		return err
	}
	
	// Validate Content-Type
	if err := rv.validateContentType(r.Header.Get("Content-Type")); err != nil {
		return err
	}
	
	// Validate cookies
	if err := rv.validateCookies(r.Cookies()); err != nil {
		return err
	}
	
	return nil
}

// ValidateRequestBody validates request body size and content
func (rv *RequestValidator) ValidateRequestBody(r *http.Request) error {
	if r.Body == nil {
		return nil
	}
	
	// Create a limited reader to prevent large uploads
	limitedReader := io.LimitReader(r.Body, rv.config.MaxBodySize)
	
	// Read body with size limit
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return &RequestValidationError{
			Code:    http.StatusBadRequest,
			Message: fmt.Sprintf("Failed to read request body: %v", err),
		}
	}
	
	// Check if body was truncated (meaning it exceeded the limit)
	if int64(len(body)) >= rv.config.MaxBodySize {
		return &RequestValidationError{
			Code:    http.StatusRequestEntityTooLarge,
			Message: fmt.Sprintf("Request body too large (max %d bytes)", rv.config.MaxBodySize),
		}
	}
	
	// Validate based on content type
	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/json") {
		return rv.validateJSONBody(body)
	}
	
	return nil
}

// ValidateWebSocketMessage validates WebSocket message size
func (rv *RequestValidator) ValidateWebSocketMessage(messageType int, data []byte) error {
	// Check message size
	if len(data) > rv.config.MaxWebSocketMsg {
		return &RequestValidationError{
			Code:    websocket.CloseMessageTooBig,
			Message: fmt.Sprintf("Message too large (max %d bytes)", rv.config.MaxWebSocketMsg),
		}
	}
	
	// Validate message type
	switch messageType {
	case websocket.TextMessage, websocket.BinaryMessage:
		// These are allowed
	case websocket.CloseMessage:
		// Close messages should be small
		if len(data) > 125 {
			return &RequestValidationError{
				Code:    websocket.CloseProtocolError,
				Message: "Close message too large",
			}
		}
	case websocket.PingMessage, websocket.PongMessage:
		// Ping/Pong messages should be small
		if len(data) > 125 {
			return &RequestValidationError{
				Code:    websocket.CloseProtocolError,
				Message: "Ping/Pong message too large",
			}
		}
	default:
		return &RequestValidationError{
			Code:    websocket.CloseUnsupportedData,
			Message: "Unsupported message type",
		}
	}
	
	return nil
}

// WithTimeout wraps a handler with timeout middleware
func (rv *RequestValidator) WithTimeout(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), rv.config.RequestTimeout)
		defer cancel()
		
		// Replace request context with timeout context
		r = r.WithContext(ctx)
		
		// Channel to signal completion
		done := make(chan struct{})
		
		go func() {
			handler.ServeHTTP(w, r)
			close(done)
		}()
		
		select {
		case <-done:
			// Request completed normally
		case <-ctx.Done():
			// Request timed out
			if ctx.Err() == context.DeadlineExceeded {
				http.Error(w, "Request timeout", http.StatusRequestTimeout)
			} else {
				http.Error(w, "Request cancelled", http.StatusRequestTimeout)
			}
		}
	})
}

// WithSizeLimit wraps a handler with size limit middleware
func (rv *RequestValidator) WithSizeLimit(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Validate request size
		if err := rv.ValidateHTTPRequest(r); err != nil {
			if reqErr, ok := err.(*RequestValidationError); ok {
				http.Error(w, reqErr.Message, reqErr.Code)
			} else {
				http.Error(w, err.Error(), http.StatusBadRequest)
			}
			return
		}
		
		// Validate request body if present
		if r.Body != nil {
			if err := rv.ValidateRequestBody(r); err != nil {
				if reqErr, ok := err.(*RequestValidationError); ok {
					http.Error(w, reqErr.Message, reqErr.Code)
				} else {
					http.Error(w, err.Error(), http.StatusBadRequest)
				}
				return
			}
		}
		
		handler.ServeHTTP(w, r)
	})
}

// Validation helper methods

func (rv *RequestValidator) validateMethod(method string) error {
	for _, allowed := range rv.config.AllowedMethods {
		if method == allowed {
			return nil
		}
	}
	
	return &RequestValidationError{
		Code:    http.StatusMethodNotAllowed,
		Message: fmt.Sprintf("Method %s not allowed", method),
		Field:   "method",
		Value:   method,
	}
}

func (rv *RequestValidator) validatePath(path string) error {
	if len(path) > rv.config.MaxPathLength {
		return &RequestValidationError{
			Code:    http.StatusRequestURITooLong,
			Message: fmt.Sprintf("Path too long (max %d characters)", rv.config.MaxPathLength),
			Field:   "path",
		}
	}
	
	// Check for suspicious patterns
	if rv.config.BlockSuspiciousRequests {
		suspiciousPatterns := []string{
			"../", "..\\", "%2e%2e", "%2f", "%5c",
			"<script", "javascript:", "vbscript:",
			"eval(", "alert(", "document.cookie",
		}
		
		lowerPath := strings.ToLower(path)
		for _, pattern := range suspiciousPatterns {
			if strings.Contains(lowerPath, pattern) {
				return &RequestValidationError{
					Code:    http.StatusBadRequest,
					Message: "Suspicious path detected",
					Field:   "path",
				}
			}
		}
	}
	
	return nil
}

func (rv *RequestValidator) validateQuery(query string) error {
	if len(query) > rv.config.MaxQueryLength {
		return &RequestValidationError{
			Code:    http.StatusRequestURITooLong,
			Message: fmt.Sprintf("Query string too long (max %d characters)", rv.config.MaxQueryLength),
			Field:   "query",
		}
	}
	
	return nil
}

func (rv *RequestValidator) validateHeaders(headers http.Header) error {
	// Calculate total header size
	totalSize := int64(0)
	for name, values := range headers {
		for _, value := range values {
			totalSize += int64(len(name) + len(value) + 4) // +4 for ": " and "\r\n"
		}
	}
	
	if totalSize > rv.config.MaxHeaderSize {
		return &RequestValidationError{
			Code:    http.StatusRequestHeaderFieldsTooLarge,
			Message: fmt.Sprintf("Headers too large (max %d bytes)", rv.config.MaxHeaderSize),
			Field:   "headers",
		}
	}
	
	// Check for required headers
	for _, required := range rv.config.RequiredHeaders {
		if headers.Get(required) == "" {
			return &RequestValidationError{
				Code:    http.StatusBadRequest,
				Message: fmt.Sprintf("Required header %s missing", required),
				Field:   "headers",
			}
		}
	}
	
	return nil
}

func (rv *RequestValidator) validateHost(host string) error {
	if !rv.config.StrictHostValidation {
		return nil
	}
	
	// Check if host is in allowed list
	for _, allowed := range rv.config.AllowedHosts {
		if host == allowed {
			return nil
		}
	}
	
	return &RequestValidationError{
		Code:    http.StatusBadRequest,
		Message: fmt.Sprintf("Host %s not allowed", host),
		Field:   "host",
		Value:   host,
	}
}

func (rv *RequestValidator) validateUserAgent(userAgent string) error {
	if userAgent == "" {
		return &RequestValidationError{
			Code:    http.StatusBadRequest,
			Message: "User-Agent header required",
			Field:   "user-agent",
		}
	}
	
	// Check for blocked user agents
	lowerUA := strings.ToLower(userAgent)
	for _, blocked := range rv.config.BlockedUserAgents {
		if strings.Contains(lowerUA, strings.ToLower(blocked)) {
			return &RequestValidationError{
				Code:    http.StatusForbidden,
				Message: "User-Agent blocked",
				Field:   "user-agent",
			}
		}
	}
	
	return nil
}

func (rv *RequestValidator) validateContentLength(r *http.Request) error {
	if !rv.config.ValidateContentLength {
		return nil
	}
	
	contentLength := r.Header.Get("Content-Length")
	if contentLength == "" && r.Body != nil {
		return &RequestValidationError{
			Code:    http.StatusBadRequest,
			Message: "Content-Length header required",
			Field:   "content-length",
		}
	}
	
	if contentLength != "" {
		length, err := strconv.ParseInt(contentLength, 10, 64)
		if err != nil {
			return &RequestValidationError{
				Code:    http.StatusBadRequest,
				Message: "Invalid Content-Length header",
				Field:   "content-length",
			}
		}
		
		if length > rv.config.MaxBodySize {
			return &RequestValidationError{
				Code:    http.StatusRequestEntityTooLarge,
				Message: fmt.Sprintf("Content-Length too large (max %d bytes)", rv.config.MaxBodySize),
				Field:   "content-length",
			}
		}
	}
	
	return nil
}

func (rv *RequestValidator) validateContentType(contentType string) error {
	if !rv.config.RequireContentType {
		return nil
	}
	
	if contentType == "" {
		return &RequestValidationError{
			Code:    http.StatusBadRequest,
			Message: "Content-Type header required",
			Field:   "content-type",
		}
	}
	
	// Extract the main content type (before semicolon)
	mainType := strings.Split(contentType, ";")[0]
	mainType = strings.TrimSpace(mainType)
	
	for _, allowed := range rv.config.AllowedContentTypes {
		if mainType == allowed {
			return nil
		}
	}
	
	return &RequestValidationError{
		Code:    http.StatusUnsupportedMediaType,
		Message: fmt.Sprintf("Content-Type %s not allowed", mainType),
		Field:   "content-type",
		Value:   mainType,
	}
}

func (rv *RequestValidator) validateCookies(cookies []*http.Cookie) error {
	totalSize := 0
	for _, cookie := range cookies {
		totalSize += len(cookie.String())
	}
	
	if totalSize > rv.config.MaxCookieSize {
		return &RequestValidationError{
			Code:    http.StatusBadRequest,
			Message: fmt.Sprintf("Cookies too large (max %d bytes)", rv.config.MaxCookieSize),
			Field:   "cookies",
		}
	}
	
	return nil
}

func (rv *RequestValidator) validateJSONBody(body []byte) error {
	if int64(len(body)) > rv.config.MaxJSONSize {
		return &RequestValidationError{
			Code:    http.StatusRequestEntityTooLarge,
			Message: fmt.Sprintf("JSON body too large (max %d bytes)", rv.config.MaxJSONSize),
			Field:   "body",
		}
	}
	
	// Basic JSON structure validation
	if len(body) > 0 {
		// Check for balanced braces/brackets
		if !isValidJSONStructure(body) {
			return &RequestValidationError{
				Code:    http.StatusBadRequest,
				Message: "Invalid JSON structure",
				Field:   "body",
			}
		}
	}
	
	return nil
}

// isValidJSONStructure performs basic JSON structure validation
func isValidJSONStructure(data []byte) bool {
	braceCount := 0
	bracketCount := 0
	inString := false
	escaped := false
	
	for _, b := range data {
		if escaped {
			escaped = false
			continue
		}
		
		switch b {
		case '\\':
			if inString {
				escaped = true
			}
		case '"':
			inString = !inString
		case '{':
			if !inString {
				braceCount++
			}
		case '}':
			if !inString {
				braceCount--
			}
		case '[':
			if !inString {
				bracketCount++
			}
		case ']':
			if !inString {
				bracketCount--
			}
		}
		
		// Check for negative counts (malformed JSON)
		if braceCount < 0 || bracketCount < 0 {
			return false
		}
	}
	
	// Check if all braces and brackets are balanced
	return braceCount == 0 && bracketCount == 0
}