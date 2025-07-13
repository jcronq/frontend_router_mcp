package validation

import (
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
	"unicode"

	"github.com/go-playground/validator/v10"
	"github.com/microcosm-cc/bluemonday"
)

// Validator wraps the go-playground validator with custom security rules
type Validator struct {
	validator *validator.Validate
	sanitizer *bluemonday.Policy
}

// ValidationError represents a validation error with details
type ValidationError struct {
	Field   string `json:"field"`
	Tag     string `json:"tag"`
	Message string `json:"message"`
	Value   string `json:"value,omitempty"`
}

// Error implements the error interface
func (ve ValidationError) Error() string {
	return ve.Message
}

// ValidationErrors represents multiple validation errors
type ValidationErrors struct {
	Errors []ValidationError `json:"errors"`
}

func (ve ValidationErrors) Error() string {
	if len(ve.Errors) == 0 {
		return "validation failed"
	}
	
	messages := make([]string, len(ve.Errors))
	for i, err := range ve.Errors {
		messages[i] = err.Message
	}
	return strings.Join(messages, ", ")
}

// InputLimits defines limits for various input types
type InputLimits struct {
	MaxMessageSize   int    // Maximum message size in bytes
	MaxQuestionLen   int    // Maximum question length
	MaxAnswerLen     int    // Maximum answer length
	MaxClientIDLen   int    // Maximum client ID length
	MaxPayloadSize   int    // Maximum JSON payload size
	AllowedMsgTypes  []string // Allowed message types
}

// DefaultInputLimits provides secure default limits
var DefaultInputLimits = InputLimits{
	MaxMessageSize:  65536,  // 64KB
	MaxQuestionLen:  10000,  // 10K chars
	MaxAnswerLen:    50000,  // 50K chars
	MaxClientIDLen:  64,     // 64 chars
	MaxPayloadSize:  32768,  // 32KB
	AllowedMsgTypes: []string{"connect", "disconnect", "user_response", "ask_user", "connect_ack"},
}

// NewValidator creates a new validator instance with custom security rules
func NewValidator() *Validator {
	v := validator.New()
	
	// Create a strict HTML sanitizer policy
	sanitizer := bluemonday.StrictPolicy()
	
	// Register custom validation rules
	v.RegisterValidation("safe_string", validateSafeString)
	v.RegisterValidation("no_html", validateNoHTML)
	v.RegisterValidation("safe_client_id", validateSafeClientID)
	v.RegisterValidation("safe_message_type", validateSafeMessageType)
	v.RegisterValidation("safe_question", validateSafeQuestion)
	v.RegisterValidation("safe_answer", validateSafeAnswer)
	v.RegisterValidation("no_script", validateNoScript)
	v.RegisterValidation("no_sql_injection", validateNoSQLInjection)
	
	return &Validator{
		validator: v,
		sanitizer: sanitizer,
	}
}

// ValidateStruct validates a struct according to its validation tags
func (v *Validator) ValidateStruct(s interface{}) error {
	err := v.validator.Struct(s)
	if err == nil {
		return nil
	}
	
	// Convert validator errors to our custom error type
	var validationErrors ValidationErrors
	
	if validatorErrors, ok := err.(validator.ValidationErrors); ok {
		for _, fieldErr := range validatorErrors {
			validationErrors.Errors = append(validationErrors.Errors, ValidationError{
				Field:   fieldErr.Field(),
				Tag:     fieldErr.Tag(),
				Message: getErrorMessage(fieldErr),
				Value:   fmt.Sprintf("%v", fieldErr.Value()),
			})
		}
	} else {
		validationErrors.Errors = append(validationErrors.Errors, ValidationError{
			Field:   "unknown",
			Tag:     "validation",
			Message: err.Error(),
		})
	}
	
	return validationErrors
}

// SanitizeString sanitizes a string by removing HTML tags and escaping special characters
func (v *Validator) SanitizeString(input string) string {
	// First, remove HTML tags
	cleaned := v.sanitizer.Sanitize(input)
	
	// Then escape HTML entities
	escaped := html.EscapeString(cleaned)
	
	// Remove any remaining dangerous characters
	return sanitizeSpecialChars(escaped)
}

// SanitizeQuestion sanitizes a question string with specific rules
func (v *Validator) SanitizeQuestion(question string) string {
	// Apply basic sanitization
	sanitized := v.SanitizeString(question)
	
	// Apply question-specific rules
	sanitized = strings.TrimSpace(sanitized)
	
	// Ensure it doesn't exceed max length
	if len(sanitized) > DefaultInputLimits.MaxQuestionLen {
		sanitized = sanitized[:DefaultInputLimits.MaxQuestionLen]
	}
	
	return sanitized
}

// SanitizeAnswer sanitizes an answer string with specific rules
func (v *Validator) SanitizeAnswer(answer string) string {
	// Apply basic sanitization
	sanitized := v.SanitizeString(answer)
	
	// Apply answer-specific rules
	sanitized = strings.TrimSpace(sanitized)
	
	// Ensure it doesn't exceed max length
	if len(sanitized) > DefaultInputLimits.MaxAnswerLen {
		sanitized = sanitized[:DefaultInputLimits.MaxAnswerLen]
	}
	
	return sanitized
}

// ValidateMessageSize checks if a message exceeds size limits
func (v *Validator) ValidateMessageSize(message []byte) error {
	if len(message) > DefaultInputLimits.MaxMessageSize {
		return fmt.Errorf("message size %d exceeds limit %d", len(message), DefaultInputLimits.MaxMessageSize)
	}
	return nil
}

// ValidatePayloadSize checks if a payload exceeds size limits
func (v *Validator) ValidatePayloadSize(payload []byte) error {
	if len(payload) > DefaultInputLimits.MaxPayloadSize {
		return fmt.Errorf("payload size %d exceeds limit %d", len(payload), DefaultInputLimits.MaxPayloadSize)
	}
	return nil
}

// ValidateMessageType checks if a message type is allowed
func (v *Validator) ValidateMessageType(msgType string) error {
	for _, allowed := range DefaultInputLimits.AllowedMsgTypes {
		if msgType == allowed {
			return nil
		}
	}
	return fmt.Errorf("message type '%s' is not allowed", msgType)
}

// Custom validation functions

func validateSafeString(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	
	// Check for dangerous patterns
	dangerousPatterns := []string{
		"<script", "</script>", "javascript:", "vbscript:", "onload=", "onerror=",
		"eval(", "document.cookie", "window.location", "alert(", "confirm(",
	}
	
	lowerValue := strings.ToLower(value)
	for _, pattern := range dangerousPatterns {
		if strings.Contains(lowerValue, pattern) {
			return false
		}
	}
	
	return true
}

func validateNoHTML(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	
	// Check for HTML tags
	htmlRegex := regexp.MustCompile(`<[^>]*>`)
	return !htmlRegex.MatchString(value)
}

func validateSafeClientID(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	
	// Client ID should only contain alphanumeric characters, hyphens, and underscores
	safeRegex := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	return safeRegex.MatchString(value) && len(value) <= DefaultInputLimits.MaxClientIDLen
}

func validateSafeMessageType(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	
	// Check against allowed message types
	for _, allowed := range DefaultInputLimits.AllowedMsgTypes {
		if value == allowed {
			return true
		}
	}
	
	return false
}

func validateSafeQuestion(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	
	// Questions should not contain script tags or malicious content
	return validateSafeString(fl) && len(value) <= DefaultInputLimits.MaxQuestionLen
}

func validateSafeAnswer(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	
	// Answers should not contain script tags or malicious content
	return validateSafeString(fl) && len(value) <= DefaultInputLimits.MaxAnswerLen
}

func validateNoScript(fl validator.FieldLevel) bool {
	value := strings.ToLower(fl.Field().String())
	
	// Check for script-related patterns
	scriptPatterns := []string{
		"<script", "</script>", "javascript:", "vbscript:",
		"onload=", "onerror=", "onclick=", "onmouseover=",
	}
	
	for _, pattern := range scriptPatterns {
		if strings.Contains(value, pattern) {
			return false
		}
	}
	
	return true
}

func validateNoSQLInjection(fl validator.FieldLevel) bool {
	value := strings.ToLower(fl.Field().String())
	
	// Check for common SQL injection patterns
	sqlPatterns := []string{
		"union select", "drop table", "delete from", "insert into",
		"update set", "exec(", "execute(", "sp_", "xp_",
		"--", "/*", "*/", "';", "'or", "'and",
	}
	
	for _, pattern := range sqlPatterns {
		if strings.Contains(value, pattern) {
			return false
		}
	}
	
	return true
}

// Helper functions

func sanitizeSpecialChars(input string) string {
	// Remove or escape dangerous special characters
	var result strings.Builder
	
	for _, r := range input {
		// Allow printable ASCII characters and common Unicode characters
		if unicode.IsPrint(r) && r < 127 {
			// Skip potentially dangerous characters
			switch r {
			case '<', '>', '"', '\'', '&':
				// These are already escaped by html.EscapeString
				result.WriteRune(r)
			case '\\', '`':
				// Escape backslashes and backticks
				result.WriteString("\\")
				result.WriteRune(r)
			default:
				result.WriteRune(r)
			}
		} else if unicode.IsSpace(r) {
			// Allow whitespace
			result.WriteRune(r)
		}
		// Skip other characters
	}
	
	return result.String()
}

func getErrorMessage(fieldErr validator.FieldError) string {
	switch fieldErr.Tag() {
	case "required":
		return fmt.Sprintf("%s is required", fieldErr.Field())
	case "max":
		return fmt.Sprintf("%s cannot exceed %s characters", fieldErr.Field(), fieldErr.Param())
	case "min":
		return fmt.Sprintf("%s must be at least %s characters", fieldErr.Field(), fieldErr.Param())
	case "safe_string":
		return fmt.Sprintf("%s contains potentially dangerous content", fieldErr.Field())
	case "no_html":
		return fmt.Sprintf("%s cannot contain HTML tags", fieldErr.Field())
	case "safe_client_id":
		return fmt.Sprintf("%s contains invalid characters or is too long", fieldErr.Field())
	case "safe_message_type":
		return fmt.Sprintf("%s is not an allowed message type", fieldErr.Field())
	case "safe_question":
		return fmt.Sprintf("%s contains invalid content or is too long", fieldErr.Field())
	case "safe_answer":
		return fmt.Sprintf("%s contains invalid content or is too long", fieldErr.Field())
	case "no_script":
		return fmt.Sprintf("%s cannot contain script tags or executable content", fieldErr.Field())
	case "no_sql_injection":
		return fmt.Sprintf("%s contains potentially malicious SQL patterns", fieldErr.Field())
	default:
		return fmt.Sprintf("%s is invalid", fieldErr.Field())
	}
}

// IsValidURL validates if a string is a valid URL
func IsValidURL(str string) bool {
	u, err := url.Parse(str)
	return err == nil && u.Scheme != "" && u.Host != ""
}

// ContainsDangerousContent checks if content contains dangerous patterns
func ContainsDangerousContent(content string) bool {
	dangerousPatterns := []string{
		"<script", "</script>", "javascript:", "vbscript:",
		"onload=", "onerror=", "onclick=", "onmouseover=",
		"eval(", "document.cookie", "window.location",
		"alert(", "confirm(", "prompt(",
	}
	
	lowerContent := strings.ToLower(content)
	for _, pattern := range dangerousPatterns {
		if strings.Contains(lowerContent, pattern) {
			return true
		}
	}
	
	return false
}