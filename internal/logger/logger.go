package logger

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"time"
)

// ContextKey is the type used for context keys
type ContextKey string

const (
	// RequestIDKey is the context key for request IDs
	RequestIDKey ContextKey = "request_id"
	// ClientIDKey is the context key for client IDs
	ClientIDKey ContextKey = "client_id"
	// ComponentKey is the context key for component names
	ComponentKey ContextKey = "component"
)

// Logger wraps slog.Logger with additional functionality
type Logger struct {
	*slog.Logger
}

// Config holds logger configuration
type Config struct {
	Level     slog.Level
	AddSource bool
	JSON      bool
}

// DefaultConfig returns default logger configuration
func DefaultConfig() Config {
	return Config{
		Level:     slog.LevelInfo,
		AddSource: true,
		JSON:      false,
	}
}

// New creates a new logger with the given configuration
func New(config Config) *Logger {
	var handler slog.Handler
	
	opts := &slog.HandlerOptions{
		Level:     config.Level,
		AddSource: config.AddSource,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Customize timestamp format
			if a.Key == slog.TimeKey {
				return slog.String("timestamp", a.Value.Time().Format(time.RFC3339))
			}
			return a
		},
	}

	if config.JSON {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	return &Logger{slog.New(handler)}
}

// WithContext adds context values to the logger
func (l *Logger) WithContext(ctx context.Context) *Logger {
	attrs := make([]slog.Attr, 0, 3)
	
	if requestID := ctx.Value(RequestIDKey); requestID != nil {
		attrs = append(attrs, slog.String("request_id", requestID.(string)))
	}
	
	if clientID := ctx.Value(ClientIDKey); clientID != nil {
		attrs = append(attrs, slog.String("client_id", clientID.(string)))
	}
	
	if component := ctx.Value(ComponentKey); component != nil {
		attrs = append(attrs, slog.String("component", component.(string)))
	}
	
	// Convert []slog.Attr to []any
	args := make([]any, len(attrs))
	for i, attr := range attrs {
		args[i] = attr
	}
	return &Logger{l.Logger.With(args...)}
}

// WithComponent adds a component name to the logger
func (l *Logger) WithComponent(component string) *Logger {
	return &Logger{l.Logger.With(slog.String("component", component))}
}

// WithRequestID adds a request ID to the logger
func (l *Logger) WithRequestID(requestID string) *Logger {
	return &Logger{l.Logger.With(slog.String("request_id", requestID))}
}

// WithClientID adds a client ID to the logger
func (l *Logger) WithClientID(clientID string) *Logger {
	return &Logger{l.Logger.With(slog.String("client_id", clientID))}
}

// WithError adds error information to the logger
func (l *Logger) WithError(err error) *Logger {
	return &Logger{l.Logger.With(slog.String("error", err.Error()))}
}

// LogError logs an error with stack trace information
func (l *Logger) LogError(ctx context.Context, err error, msg string, args ...interface{}) {
	// Get caller information
	_, file, line, ok := runtime.Caller(1)
	if ok {
		l.WithContext(ctx).Error(fmt.Sprintf(msg, args...), 
			slog.String("error", err.Error()),
			slog.String("file", file),
			slog.Int("line", line))
	} else {
		l.WithContext(ctx).Error(fmt.Sprintf(msg, args...), 
			slog.String("error", err.Error()))
	}
}

// LogPanic logs a panic with recovery information
func (l *Logger) LogPanic(ctx context.Context, recovered interface{}, msg string, args ...interface{}) {
	// Get caller information
	_, file, line, ok := runtime.Caller(1)
	if ok {
		l.WithContext(ctx).Error(fmt.Sprintf(msg, args...), 
			slog.Any("panic", recovered),
			slog.String("file", file),
			slog.Int("line", line))
	} else {
		l.WithContext(ctx).Error(fmt.Sprintf(msg, args...), 
			slog.Any("panic", recovered))
	}
}

// Fatal logs a fatal error and exits
func (l *Logger) Fatal(ctx context.Context, err error, msg string, args ...interface{}) {
	l.LogError(ctx, err, msg, args...)
	os.Exit(1)
}

// Default logger instance
var defaultLogger *Logger

// Initialize sets up the default logger
func Initialize(config Config) {
	defaultLogger = New(config)
}

// GetDefault returns the default logger
func GetDefault() *Logger {
	if defaultLogger == nil {
		defaultLogger = New(DefaultConfig())
	}
	return defaultLogger
}

// Context functions for the default logger
func WithContext(ctx context.Context) *Logger {
	return GetDefault().WithContext(ctx)
}

func WithComponent(component string) *Logger {
	return GetDefault().WithComponent(component)
}

func WithRequestID(requestID string) *Logger {
	return GetDefault().WithRequestID(requestID)
}

func WithClientID(clientID string) *Logger {
	return GetDefault().WithClientID(clientID)
}

func WithError(err error) *Logger {
	return GetDefault().WithError(err)
}