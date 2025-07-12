package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// LogLevel represents the severity level of a log entry
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
	FATAL
)

// String returns the string representation of the log level
func (l LogLevel) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	case FATAL:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// LogEntry represents a structured log entry
type LogEntry struct {
	Timestamp     time.Time              `json:"timestamp"`
	Level         string                 `json:"level"`
	Message       string                 `json:"message"`
	RequestID     string                 `json:"request_id,omitempty"`
	TraceID       string                 `json:"trace_id,omitempty"`
	SpanID        string                 `json:"span_id,omitempty"`
	UserID        string                 `json:"user_id,omitempty"`
	Service       string                 `json:"service"`
	Component     string                 `json:"component,omitempty"`
	Operation     string                 `json:"operation,omitempty"`
	Duration      *time.Duration         `json:"duration,omitempty"`
	StatusCode    *int                   `json:"status_code,omitempty"`
	Error         string                 `json:"error,omitempty"`
	Stack         string                 `json:"stack,omitempty"`
	Fields        map[string]interface{} `json:"fields,omitempty"`
	Caller        string                 `json:"caller,omitempty"`
	HTTPMethod    string                 `json:"http_method,omitempty"`
	HTTPPath      string                 `json:"http_path,omitempty"`
	HTTPUserAgent string                 `json:"http_user_agent,omitempty"`
	ClientIP      string                 `json:"client_ip,omitempty"`
}

// Logger provides structured logging with correlation IDs
type Logger struct {
	service    string
	component  string
	level      LogLevel
	output     io.Writer
	mu         sync.RWMutex
	requestID  string
	traceID    string
	spanID     string
	userID     string
	fields     map[string]interface{}
}

// Config holds logger configuration
type Config struct {
	Service   string
	Component string
	Level     LogLevel
	Output    io.Writer
}

// NewLogger creates a new structured logger
func NewLogger(config Config) *Logger {
	if config.Output == nil {
		config.Output = os.Stdout
	}
	
	return &Logger{
		service:   config.Service,
		component: config.Component,
		level:     config.Level,
		output:    config.Output,
		fields:    make(map[string]interface{}),
	}
}

// Clone creates a copy of the logger with the same configuration
func (l *Logger) Clone() *Logger {
	l.mu.RLock()
	defer l.mu.RUnlock()
	
	clone := &Logger{
		service:   l.service,
		component: l.component,
		level:     l.level,
		output:    l.output,
		requestID: l.requestID,
		traceID:   l.traceID,
		spanID:    l.spanID,
		userID:    l.userID,
		fields:    make(map[string]interface{}),
	}
	
	// Copy fields
	for k, v := range l.fields {
		clone.fields[k] = v
	}
	
	return clone
}

// WithRequestID adds a request ID to the logger context
func (l *Logger) WithRequestID(requestID string) *Logger {
	clone := l.Clone()
	clone.requestID = requestID
	return clone
}

// WithTraceID adds a trace ID to the logger context
func (l *Logger) WithTraceID(traceID string) *Logger {
	clone := l.Clone()
	clone.traceID = traceID
	return clone
}

// WithSpanID adds a span ID to the logger context
func (l *Logger) WithSpanID(spanID string) *Logger {
	clone := l.Clone()
	clone.spanID = spanID
	return clone
}

// WithUserID adds a user ID to the logger context
func (l *Logger) WithUserID(userID string) *Logger {
	clone := l.Clone()
	clone.userID = userID
	return clone
}

// WithComponent adds a component to the logger context
func (l *Logger) WithComponent(component string) *Logger {
	clone := l.Clone()
	clone.component = component
	return clone
}

// WithField adds a field to the logger context
func (l *Logger) WithField(key string, value interface{}) *Logger {
	clone := l.Clone()
	clone.fields[key] = value
	return clone
}

// WithFields adds multiple fields to the logger context
func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	clone := l.Clone()
	for k, v := range fields {
		clone.fields[k] = v
	}
	return clone
}

// WithError adds an error to the logger context
func (l *Logger) WithError(err error) *Logger {
	clone := l.Clone()
	if err != nil {
		clone.fields["error"] = err.Error()
	}
	return clone
}

// Debug logs a debug message
func (l *Logger) Debug(message string) {
	l.log(DEBUG, message, nil)
}

// Debugf logs a formatted debug message
func (l *Logger) Debugf(format string, args ...interface{}) {
	l.log(DEBUG, fmt.Sprintf(format, args...), nil)
}

// Info logs an info message
func (l *Logger) Info(message string) {
	l.log(INFO, message, nil)
}

// Infof logs a formatted info message
func (l *Logger) Infof(format string, args ...interface{}) {
	l.log(INFO, fmt.Sprintf(format, args...), nil)
}

// Warn logs a warning message
func (l *Logger) Warn(message string) {
	l.log(WARN, message, nil)
}

// Warnf logs a formatted warning message
func (l *Logger) Warnf(format string, args ...interface{}) {
	l.log(WARN, fmt.Sprintf(format, args...), nil)
}

// Error logs an error message
func (l *Logger) Error(message string) {
	l.log(ERROR, message, nil)
}

// Errorf logs a formatted error message
func (l *Logger) Errorf(format string, args ...interface{}) {
	l.log(ERROR, fmt.Sprintf(format, args...), nil)
}

// Fatal logs a fatal message and exits
func (l *Logger) Fatal(message string) {
	l.log(FATAL, message, nil)
	os.Exit(1)
}

// Fatalf logs a formatted fatal message and exits
func (l *Logger) Fatalf(format string, args ...interface{}) {
	l.log(FATAL, fmt.Sprintf(format, args...), nil)
	os.Exit(1)
}

// LogHTTP logs an HTTP request/response
func (l *Logger) LogHTTP(method, path, userAgent, clientIP string, statusCode int, duration time.Duration) {
	entry := l.createEntry(INFO, "HTTP request processed")
	entry.HTTPMethod = method
	entry.HTTPPath = path
	entry.HTTPUserAgent = userAgent
	entry.ClientIP = clientIP
	entry.StatusCode = &statusCode
	entry.Duration = &duration
	
	l.writeEntry(entry)
}

// LogOperation logs an operation with duration
func (l *Logger) LogOperation(operation string, duration time.Duration, err error) {
	level := INFO
	message := fmt.Sprintf("Operation %s completed", operation)
	
	if err != nil {
		level = ERROR
		message = fmt.Sprintf("Operation %s failed: %v", operation, err)
	}
	
	entry := l.createEntry(level, message)
	entry.Operation = operation
	entry.Duration = &duration
	
	if err != nil {
		entry.Error = err.Error()
	}
	
	l.writeEntry(entry)
}

// StartSpan starts a new span for tracing
func (l *Logger) StartSpan(operation string) *Logger {
	spanID := generateSpanID()
	traceID := l.traceID
	if traceID == "" {
		traceID = generateTraceID()
	}
	
	span := l.WithTraceID(traceID).WithSpanID(spanID).WithField("operation", operation)
	span.Infof("Starting span: %s", operation)
	return span
}

// FinishSpan finishes a span
func (l *Logger) FinishSpan(operation string, start time.Time, err error) {
	duration := time.Since(start)
	
	if err != nil {
		l.WithError(err).Errorf("Span %s failed after %v", operation, duration)
	} else {
		l.Infof("Span %s completed in %v", operation, duration)
	}
}

// log writes a log entry
func (l *Logger) log(level LogLevel, message string, err error) {
	if level < l.level {
		return
	}
	
	entry := l.createEntry(level, message)
	
	if err != nil {
		entry.Error = err.Error()
		if level >= ERROR {
			entry.Stack = getStack()
		}
	}
	
	l.writeEntry(entry)
}

// createEntry creates a new log entry
func (l *Logger) createEntry(level LogLevel, message string) *LogEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()
	
	entry := &LogEntry{
		Timestamp: time.Now().UTC(),
		Level:     level.String(),
		Message:   message,
		Service:   l.service,
		Component: l.component,
		RequestID: l.requestID,
		TraceID:   l.traceID,
		SpanID:    l.spanID,
		UserID:    l.userID,
		Caller:    getCaller(),
	}
	
	if len(l.fields) > 0 {
		entry.Fields = make(map[string]interface{})
		for k, v := range l.fields {
			entry.Fields[k] = v
		}
	}
	
	return entry
}

// writeEntry writes a log entry to the output
func (l *Logger) writeEntry(entry *LogEntry) {
	data, err := json.Marshal(entry)
	if err != nil {
		// Fallback to simple logging if JSON marshaling fails
		fmt.Fprintf(l.output, "[%s] %s: %s\n", entry.Timestamp.Format(time.RFC3339), entry.Level, entry.Message)
		return
	}
	
	l.output.Write(data)
	l.output.Write([]byte("\n"))
}

// getCaller returns the caller information
func getCaller() string {
	// Skip getCaller, writeEntry, createEntry, log, and the actual log method
	_, file, line, ok := runtime.Caller(5)
	if !ok {
		return ""
	}
	
	// Get just the filename, not the full path
	parts := strings.Split(file, "/")
	if len(parts) > 0 {
		file = parts[len(parts)-1]
	}
	
	return fmt.Sprintf("%s:%d", file, line)
}

// getStack returns the current stack trace
func getStack() string {
	buf := make([]byte, 1024)
	for {
		n := runtime.Stack(buf, false)
		if n < len(buf) {
			return string(buf[:n])
		}
		buf = make([]byte, 2*len(buf))
	}
}

// generateTraceID generates a new trace ID
func generateTraceID() string {
	return uuid.New().String()
}

// generateSpanID generates a new span ID
func generateSpanID() string {
	return uuid.New().String()[:8]
}

// ContextKey is a type for context keys
type ContextKey string

const (
	// LoggerKey is the context key for the logger
	LoggerKey ContextKey = "logger"
	// RequestIDKey is the context key for request ID
	RequestIDKey ContextKey = "request_id"
	// TraceIDKey is the context key for trace ID
	TraceIDKey ContextKey = "trace_id"
	// SpanIDKey is the context key for span ID
	SpanIDKey ContextKey = "span_id"
	// UserIDKey is the context key for user ID
	UserIDKey ContextKey = "user_id"
)

// FromContext extracts a logger from the context
func FromContext(ctx context.Context) *Logger {
	if logger, ok := ctx.Value(LoggerKey).(*Logger); ok {
		return logger
	}
	return NewLogger(Config{Service: "unknown", Level: INFO})
}

// ToContext adds a logger to the context
func ToContext(ctx context.Context, logger *Logger) context.Context {
	return context.WithValue(ctx, LoggerKey, logger)
}

// GetRequestID extracts the request ID from the context
func GetRequestID(ctx context.Context) string {
	if requestID, ok := ctx.Value(RequestIDKey).(string); ok {
		return requestID
	}
	return ""
}

// WithRequestIDContext adds a request ID to the context
func WithRequestIDContext(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, requestID)
}

// GetTraceID extracts the trace ID from the context
func GetTraceID(ctx context.Context) string {
	if traceID, ok := ctx.Value(TraceIDKey).(string); ok {
		return traceID
	}
	return ""
}

// WithTraceIDContext adds a trace ID to the context
func WithTraceIDContext(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, TraceIDKey, traceID)
}

// GenerateRequestID generates a new request ID
func GenerateRequestID() string {
	return uuid.New().String()
}

// GenerateTraceID generates a new trace ID
func GenerateTraceID() string {
	return uuid.New().String()
}

// SetLogLevel sets the log level for a logger
func (l *Logger) SetLogLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}