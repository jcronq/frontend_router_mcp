package validation

import (
	"encoding/json"
	"time"
)

// WebSocketMessage represents a validated WebSocket message
type WebSocketMessage struct {
	Type      string          `json:"type" validate:"required,safe_message_type"`
	Payload   json.RawMessage `json:"payload,omitempty" validate:"required"`
	Timestamp time.Time       `json:"timestamp,omitempty"`
	ClientID  string          `json:"client_id,omitempty" validate:"omitempty,safe_client_id"`
}

// UserQuestion represents a validated user question
type UserQuestion struct {
	RequestID string `json:"request_id" validate:"required,safe_client_id"`
	Question  string `json:"question" validate:"required,safe_question,no_html,no_script"`
}

// UserResponse represents a validated user response
type UserResponse struct {
	RequestID string `json:"request_id" validate:"required,safe_client_id"`
	Answer    string `json:"answer" validate:"required,safe_answer,no_script"`
}

// AskUserParams represents validated parameters for the ask_user tool
type AskUserParams struct {
	Question       string  `json:"question" validate:"required,safe_question,no_html,no_script,max=10000"`
	TimeoutSeconds float64 `json:"timeout_seconds" validate:"omitempty,min=1,max=3600"`
}

// ConnectMessage represents a validated connect message
type ConnectMessage struct {
	ClientID string `json:"client_id,omitempty" validate:"omitempty,safe_client_id"`
	Version  string `json:"version,omitempty" validate:"omitempty,max=32"`
}

// DisconnectMessage represents a validated disconnect message
type DisconnectMessage struct {
	ClientID string `json:"client_id,omitempty" validate:"omitempty,safe_client_id"`
	Reason   string `json:"reason,omitempty" validate:"omitempty,max=256,safe_string"`
}

// MCPToolRequest represents a validated MCP tool request
type MCPToolRequest struct {
	Name      string                 `json:"name" validate:"required,max=64,safe_string"`
	Arguments map[string]interface{} `json:"arguments" validate:"required"`
}

// SecurityConfig represents security configuration options
type SecurityConfig struct {
	MaxConnections       int           `json:"max_connections" validate:"min=1,max=10000"`
	MaxMessageSize       int           `json:"max_message_size" validate:"min=1024,max=1048576"`
	MaxPayloadSize       int           `json:"max_payload_size" validate:"min=512,max=524288"`
	RateLimitRequests    int           `json:"rate_limit_requests" validate:"min=1,max=1000"`
	RateLimitWindow      time.Duration `json:"rate_limit_window" validate:"min=1s,max=1h"`
	ConnectionTimeout    time.Duration `json:"connection_timeout" validate:"min=1s,max=300s"`
	ReadTimeout          time.Duration `json:"read_timeout" validate:"min=1s,max=300s"`
	WriteTimeout         time.Duration `json:"write_timeout" validate:"min=1s,max=300s"`
	EnableCSRF           bool          `json:"enable_csrf"`
	EnableSecurityHeaders bool         `json:"enable_security_headers"`
	AllowedOrigins       []string      `json:"allowed_origins" validate:"dive,max=256"`
}

// DefaultSecurityConfig provides secure default configuration
var DefaultSecurityConfig = SecurityConfig{
	MaxConnections:       100,
	MaxMessageSize:       65536,  // 64KB
	MaxPayloadSize:       32768,  // 32KB
	RateLimitRequests:    100,    // 100 requests per window
	RateLimitWindow:      time.Minute,
	ConnectionTimeout:    30 * time.Second,
	ReadTimeout:          60 * time.Second,
	WriteTimeout:         30 * time.Second,
	EnableCSRF:           true,
	EnableSecurityHeaders: true,
	AllowedOrigins:       []string{"http://localhost:8080", "https://localhost:8080"},
}

// ValidationResult contains the result of input validation
type ValidationResult struct {
	Valid      bool     `json:"valid"`
	Errors     []string `json:"errors,omitempty"`
	Warnings   []string `json:"warnings,omitempty"`
	Sanitized  bool     `json:"sanitized,omitempty"`
}

// ClientInfo represents information about a connected client
type ClientInfo struct {
	ID            string    `json:"id" validate:"required,safe_client_id"`
	RemoteAddr    string    `json:"remote_addr" validate:"required,max=256"`
	UserAgent     string    `json:"user_agent" validate:"omitempty,max=512,safe_string"`
	ConnectedAt   time.Time `json:"connected_at"`
	LastActivity  time.Time `json:"last_activity"`
	MessageCount  int       `json:"message_count"`
	BytesSent     int64     `json:"bytes_sent"`
	BytesReceived int64     `json:"bytes_received"`
}

// RateLimitInfo represents rate limiting information
type RateLimitInfo struct {
	ClientID     string    `json:"client_id" validate:"required,safe_client_id"`
	RequestCount int       `json:"request_count"`
	WindowStart  time.Time `json:"window_start"`
	WindowEnd    time.Time `json:"window_end"`
	IsLimited    bool      `json:"is_limited"`
}

// SecurityEvent represents a security-related event
type SecurityEvent struct {
	ID          string                 `json:"id" validate:"required"`
	Type        string                 `json:"type" validate:"required,max=64"`
	ClientID    string                 `json:"client_id" validate:"required,safe_client_id"`
	RemoteAddr  string                 `json:"remote_addr" validate:"required,max=256"`
	Timestamp   time.Time              `json:"timestamp"`
	Severity    string                 `json:"severity" validate:"required,oneof=low medium high critical"`
	Message     string                 `json:"message" validate:"required,max=1000,safe_string"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Resolved    bool                   `json:"resolved"`
	ResolvedAt  *time.Time             `json:"resolved_at,omitempty"`
}

// Common validation tags and their meanings:
// - required: field is required
// - max: maximum length/value
// - min: minimum length/value
// - safe_string: contains no dangerous content
// - safe_client_id: valid client ID format
// - safe_message_type: valid message type
// - safe_question: valid question content
// - safe_answer: valid answer content
// - no_html: contains no HTML tags
// - no_script: contains no script tags
// - no_sql_injection: contains no SQL injection patterns
// - oneof: value must be one of the specified values
// - dive: validate each element in a slice/array

// ValidateAndSanitize validates and sanitizes input data
func ValidateAndSanitize(validator *Validator, data interface{}) (*ValidationResult, error) {
	result := &ValidationResult{
		Valid:    true,
		Errors:   []string{},
		Warnings: []string{},
	}
	
	// Validate the struct
	if err := validator.ValidateStruct(data); err != nil {
		result.Valid = false
		if validationErrors, ok := err.(ValidationErrors); ok {
			for _, ve := range validationErrors.Errors {
				result.Errors = append(result.Errors, ve.Message)
			}
		} else {
			result.Errors = append(result.Errors, err.Error())
		}
		return result, nil
	}
	
	// Perform sanitization based on type
	switch v := data.(type) {
	case *UserQuestion:
		original := v.Question
		v.Question = validator.SanitizeQuestion(v.Question)
		if original != v.Question {
			result.Sanitized = true
			result.Warnings = append(result.Warnings, "Question content was sanitized")
		}
		
	case *UserResponse:
		original := v.Answer
		v.Answer = validator.SanitizeAnswer(v.Answer)
		if original != v.Answer {
			result.Sanitized = true
			result.Warnings = append(result.Warnings, "Answer content was sanitized")
		}
		
	case *AskUserParams:
		original := v.Question
		v.Question = validator.SanitizeQuestion(v.Question)
		if original != v.Question {
			result.Sanitized = true
			result.Warnings = append(result.Warnings, "Question content was sanitized")
		}
	}
	
	return result, nil
}

// SanitizeStruct sanitizes all string fields in a struct
func SanitizeStruct(validator *Validator, data interface{}) error {
	switch v := data.(type) {
	case *UserQuestion:
		v.Question = validator.SanitizeQuestion(v.Question)
	case *UserResponse:
		v.Answer = validator.SanitizeAnswer(v.Answer)
	case *AskUserParams:
		v.Question = validator.SanitizeQuestion(v.Question)
	case *DisconnectMessage:
		if v.Reason != "" {
			v.Reason = validator.SanitizeString(v.Reason)
		}
	}
	
	return nil
}