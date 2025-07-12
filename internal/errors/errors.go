package errors

import (
	"context"
	"errors"
	"fmt"
	"runtime"
)

// ErrorType represents different types of errors
type ErrorType string

const (
	// ErrorTypeValidation represents validation errors
	ErrorTypeValidation ErrorType = "validation"
	// ErrorTypeNetwork represents network errors
	ErrorTypeNetwork ErrorType = "network"
	// ErrorTypeTimeout represents timeout errors
	ErrorTypeTimeout ErrorType = "timeout"
	// ErrorTypeInternal represents internal server errors
	ErrorTypeInternal ErrorType = "internal"
	// ErrorTypeNotFound represents not found errors
	ErrorTypeNotFound ErrorType = "not_found"
	// ErrorTypeUnauthorized represents unauthorized errors
	ErrorTypeUnauthorized ErrorType = "unauthorized"
	// ErrorTypeConflict represents conflict errors
	ErrorTypeConflict ErrorType = "conflict"
)

// AppError represents an application-specific error
type AppError struct {
	Type      ErrorType
	Message   string
	Cause     error
	Context   map[string]interface{}
	File      string
	Line      int
	RequestID string
}

// Error implements the error interface
func (e *AppError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

// Unwrap returns the underlying error
func (e *AppError) Unwrap() error {
	return e.Cause
}

// New creates a new AppError
func New(errorType ErrorType, message string) *AppError {
	_, file, line, _ := runtime.Caller(1)
	return &AppError{
		Type:    errorType,
		Message: message,
		File:    file,
		Line:    line,
		Context: make(map[string]interface{}),
	}
}

// Wrap wraps an existing error with additional context
func Wrap(err error, errorType ErrorType, message string) *AppError {
	_, file, line, _ := runtime.Caller(1)
	return &AppError{
		Type:    errorType,
		Message: message,
		Cause:   err,
		File:    file,
		Line:    line,
		Context: make(map[string]interface{}),
	}
}

// WithContext adds context to the error
func (e *AppError) WithContext(key string, value interface{}) *AppError {
	e.Context[key] = value
	return e
}

// WithRequestID adds a request ID to the error
func (e *AppError) WithRequestID(requestID string) *AppError {
	e.RequestID = requestID
	return e
}

// IsType checks if the error is of a specific type
func IsType(err error, errorType ErrorType) bool {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Type == errorType
	}
	return false
}

// GetType returns the error type
func GetType(err error) ErrorType {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Type
	}
	return ErrorTypeInternal
}

// GetContext returns the error context
func GetContext(err error) map[string]interface{} {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Context
	}
	return nil
}

// GetRequestID returns the request ID from the error
func GetRequestID(err error) string {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.RequestID
	}
	return ""
}

// NewValidationError creates a new validation error
func NewValidationError(message string) *AppError {
	return New(ErrorTypeValidation, message)
}

// NewNetworkError creates a new network error
func NewNetworkError(message string) *AppError {
	return New(ErrorTypeNetwork, message)
}

// NewTimeoutError creates a new timeout error
func NewTimeoutError(message string) *AppError {
	return New(ErrorTypeTimeout, message)
}

// NewInternalError creates a new internal error
func NewInternalError(message string) *AppError {
	return New(ErrorTypeInternal, message)
}

// NewNotFoundError creates a new not found error
func NewNotFoundError(message string) *AppError {
	return New(ErrorTypeNotFound, message)
}

// Recovery handles panics and converts them to errors
func Recovery(ctx context.Context) func() error {
	return func() error {
		if r := recover(); r != nil {
			_, file, line, _ := runtime.Caller(1)
			err := &AppError{
				Type:    ErrorTypeInternal,
				Message: fmt.Sprintf("panic recovered: %v", r),
				File:    file,
				Line:    line,
				Context: map[string]interface{}{
					"panic": r,
				},
			}
			
			// Try to get request ID from context
			if requestID, ok := ctx.Value("request_id").(string); ok {
				err.RequestID = requestID
			}
			
			return err
		}
		return nil
	}
}

// SafeGoroutine executes a function in a goroutine with panic recovery
func SafeGoroutine(ctx context.Context, fn func() error, onError func(error)) {
	go func() {
		defer func() {
			if err := Recovery(ctx)(); err != nil {
				onError(err)
			}
		}()
		
		if err := fn(); err != nil {
			onError(err)
		}
	}()
}