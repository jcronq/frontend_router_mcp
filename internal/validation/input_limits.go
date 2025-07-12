package validation

import (
	"fmt"
	"mime"
	"net/http"
	"strings"
	"time"
)

// InputLimitsValidator provides comprehensive input length and content type validation
type InputLimitsValidator struct {
	config InputLimitsConfig
}

// InputLimitsConfig configures input limits and content type validation
type InputLimitsConfig struct {
	// General input limits
	MaxStringLength      int           `json:"max_string_length"`
	MaxTextLength        int           `json:"max_text_length"`
	MaxFilenameLength    int           `json:"max_filename_length"`
	MaxPathLength        int           `json:"max_path_length"`
	MaxQueryLength       int           `json:"max_query_length"`
	MaxHeaderLength      int           `json:"max_header_length"`
	MaxCookieLength      int           `json:"max_cookie_length"`
	MaxUserAgentLength   int           `json:"max_user_agent_length"`
	MaxRefererLength     int           `json:"max_referer_length"`
	
	// Message-specific limits
	MaxQuestionLength    int           `json:"max_question_length"`
	MaxAnswerLength      int           `json:"max_answer_length"`
	MaxClientIDLength    int           `json:"max_client_id_length"`
	MaxRequestIDLength   int           `json:"max_request_id_length"`
	MaxMessageIDLength   int           `json:"max_message_id_length"`
	MaxErrorMessageLength int          `json:"max_error_message_length"`
	MaxReasonLength      int           `json:"max_reason_length"`
	MaxVersionLength     int           `json:"max_version_length"`
	
	// Content type validation
	AllowedContentTypes  []string      `json:"allowed_content_types"`
	AllowedFileTypes     []string      `json:"allowed_file_types"`
	AllowedMimeTypes     []string      `json:"allowed_mime_types"`
	BlockedContentTypes  []string      `json:"blocked_content_types"`
	BlockedFileTypes     []string      `json:"blocked_file_types"`
	BlockedMimeTypes     []string      `json:"blocked_mime_types"`
	RequireContentType   bool          `json:"require_content_type"`
	ValidateCharset      bool          `json:"validate_charset"`
	AllowedCharsets      []string      `json:"allowed_charsets"`
	
	// File upload limits
	MaxFileSize          int64         `json:"max_file_size"`
	MaxTotalFileSize     int64         `json:"max_total_file_size"`
	MaxFileCount         int           `json:"max_file_count"`
	AllowedFileExtensions []string     `json:"allowed_file_extensions"`
	BlockedFileExtensions []string     `json:"blocked_file_extensions"`
	
	// Advanced validation
	ValidateUTF8         bool          `json:"validate_utf8"`
	ValidateJSON         bool          `json:"validate_json"`
	ValidateXML          bool          `json:"validate_xml"`
	ValidateHTML         bool          `json:"validate_html"`
	ValidateCSS          bool          `json:"validate_css"`
	ValidateJavaScript   bool          `json:"validate_javascript"`
	
	// Performance limits
	MaxProcessingTime    time.Duration `json:"max_processing_time"`
	MaxMemoryUsage       int64         `json:"max_memory_usage"`
	MaxCPUUsage          float64       `json:"max_cpu_usage"`
	
	// Custom validation rules
	CustomRules          map[string]InputRule `json:"custom_rules"`
	
	// Error handling
	StrictMode           bool          `json:"strict_mode"`
	TruncateOnLimit      bool          `json:"truncate_on_limit"`
	LogLimitViolations   bool          `json:"log_limit_violations"`
}

// InputRule defines a custom validation rule
type InputRule struct {
	MinLength    int      `json:"min_length"`
	MaxLength    int      `json:"max_length"`
	Pattern      string   `json:"pattern"`
	AllowedValues []string `json:"allowed_values"`
	Required     bool     `json:"required"`
	Description  string   `json:"description"`
}

// DefaultInputLimitsConfig provides secure default configuration
var DefaultInputLimitsConfig = InputLimitsConfig{
	MaxStringLength:       1000,
	MaxTextLength:         10000,
	MaxFilenameLength:     255,
	MaxPathLength:         1024,
	MaxQueryLength:        2048,
	MaxHeaderLength:       8192,
	MaxCookieLength:       4096,
	MaxUserAgentLength:    512,
	MaxRefererLength:      2048,
	MaxQuestionLength:     10000,
	MaxAnswerLength:       50000,
	MaxClientIDLength:     64,
	MaxRequestIDLength:    128,
	MaxMessageIDLength:    128,
	MaxErrorMessageLength: 1000,
	MaxReasonLength:       500,
	MaxVersionLength:      32,
	
	AllowedContentTypes: []string{
		"application/json",
		"application/x-www-form-urlencoded",
		"multipart/form-data",
		"text/plain",
		"text/html",
		"text/css",
		"text/javascript",
		"application/javascript",
		"application/xml",
		"text/xml",
	},
	
	AllowedFileTypes: []string{
		"txt", "json", "xml", "html", "css", "js",
		"png", "jpg", "jpeg", "gif", "svg", "webp",
		"pdf", "doc", "docx", "xls", "xlsx", "ppt", "pptx",
		"zip", "tar", "gz", "bz2",
	},
	
	AllowedMimeTypes: []string{
		"text/plain",
		"text/html",
		"text/css",
		"text/javascript",
		"application/json",
		"application/xml",
		"application/pdf",
		"image/png",
		"image/jpeg",
		"image/gif",
		"image/svg+xml",
		"image/webp",
	},
	
	BlockedContentTypes: []string{
		"application/x-msdownload",
		"application/x-msdos-program",
		"application/x-executable",
		"application/x-shellscript",
		"text/x-script",
	},
	
	BlockedFileTypes: []string{
		"exe", "bat", "cmd", "com", "pif", "scr", "vbs", "js",
		"jar", "app", "deb", "dmg", "pkg", "rpm",
		"sh", "bash", "csh", "tcsh", "zsh",
	},
	
	BlockedMimeTypes: []string{
		"application/x-executable",
		"application/x-msdownload",
		"application/x-msdos-program",
		"application/x-winexe",
		"application/x-shellscript",
		"text/x-script",
	},
	
	RequireContentType:    true,
	ValidateCharset:       true,
	AllowedCharsets:       []string{"utf-8", "utf-16", "iso-8859-1", "ascii"},
	MaxFileSize:           10 * 1024 * 1024, // 10MB
	MaxTotalFileSize:      100 * 1024 * 1024, // 100MB
	MaxFileCount:          10,
	AllowedFileExtensions: []string{".txt", ".json", ".xml", ".html", ".css", ".js", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".pdf"},
	BlockedFileExtensions: []string{".exe", ".bat", ".cmd", ".com", ".pif", ".scr", ".vbs", ".jar", ".app", ".deb", ".dmg", ".pkg", ".rpm", ".sh"},
	
	ValidateUTF8:       true,
	ValidateJSON:       true,
	ValidateXML:        true,
	ValidateHTML:       true,
	ValidateCSS:        false,
	ValidateJavaScript: false,
	
	MaxProcessingTime: 30 * time.Second,
	MaxMemoryUsage:    100 * 1024 * 1024, // 100MB
	MaxCPUUsage:       80.0,
	
	CustomRules: map[string]InputRule{
		"email": {
			MinLength:   5,
			MaxLength:   254,
			Pattern:     `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`,
			Required:    false,
			Description: "Valid email address",
		},
		"url": {
			MinLength:   10,
			MaxLength:   2048,
			Pattern:     `^https?://[^\s/$.?#].[^\s]*$`,
			Required:    false,
			Description: "Valid HTTP/HTTPS URL",
		},
		"username": {
			MinLength:   3,
			MaxLength:   32,
			Pattern:     `^[a-zA-Z0-9_-]+$`,
			Required:    false,
			Description: "Alphanumeric username with underscores and hyphens",
		},
	},
	
	StrictMode:         true,
	TruncateOnLimit:    false,
	LogLimitViolations: true,
}

// NewInputLimitsValidator creates a new input limits validator
func NewInputLimitsValidator(config InputLimitsConfig) *InputLimitsValidator {
	return &InputLimitsValidator{
		config: config,
	}
}

// ValidateStringLength validates string length
func (ilv *InputLimitsValidator) ValidateStringLength(field, value string, maxLength int) error {
	if len(value) > maxLength {
		if ilv.config.StrictMode {
			return fmt.Errorf("%s exceeds maximum length of %d characters (got %d)", field, maxLength, len(value))
		}
		if ilv.config.TruncateOnLimit {
			// Truncate the string (this would modify the original value)
			return fmt.Errorf("%s truncated to %d characters", field, maxLength)
		}
	}
	return nil
}

// ValidateInputLimits validates all input limits for a given input
func (ilv *InputLimitsValidator) ValidateInputLimits(input interface{}) error {
	switch v := input.(type) {
	case *UserQuestion:
		if err := ilv.ValidateStringLength("question", v.Question, ilv.config.MaxQuestionLength); err != nil {
			return err
		}
		if err := ilv.ValidateStringLength("request_id", v.RequestID, ilv.config.MaxRequestIDLength); err != nil {
			return err
		}
		
	case *UserResponse:
		if err := ilv.ValidateStringLength("answer", v.Answer, ilv.config.MaxAnswerLength); err != nil {
			return err
		}
		if err := ilv.ValidateStringLength("request_id", v.RequestID, ilv.config.MaxRequestIDLength); err != nil {
			return err
		}
		
	case *AskUserParams:
		if err := ilv.ValidateStringLength("question", v.Question, ilv.config.MaxQuestionLength); err != nil {
			return err
		}
		
	case *ConnectMessage:
		if err := ilv.ValidateStringLength("client_id", v.ClientID, ilv.config.MaxClientIDLength); err != nil {
			return err
		}
		if err := ilv.ValidateStringLength("version", v.Version, ilv.config.MaxVersionLength); err != nil {
			return err
		}
		
	case *DisconnectMessage:
		if err := ilv.ValidateStringLength("client_id", v.ClientID, ilv.config.MaxClientIDLength); err != nil {
			return err
		}
		if err := ilv.ValidateStringLength("reason", v.Reason, ilv.config.MaxReasonLength); err != nil {
			return err
		}
		
	case *WebSocketMessage:
		if err := ilv.ValidateStringLength("type", v.Type, ilv.config.MaxStringLength); err != nil {
			return err
		}
		if err := ilv.ValidateStringLength("client_id", v.ClientID, ilv.config.MaxClientIDLength); err != nil {
			return err
		}
		if len(v.Payload) > ilv.config.MaxTextLength {
			return fmt.Errorf("payload exceeds maximum length of %d bytes", ilv.config.MaxTextLength)
		}
	}
	
	return nil
}

// ValidateContentType validates HTTP content type
func (ilv *InputLimitsValidator) ValidateContentType(r *http.Request) error {
	contentType := r.Header.Get("Content-Type")
	
	// Check if content type is required
	if ilv.config.RequireContentType && contentType == "" {
		return fmt.Errorf("Content-Type header is required")
	}
	
	if contentType == "" {
		return nil // Allow empty content type if not required
	}
	
	// Parse content type
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return fmt.Errorf("invalid Content-Type header: %w", err)
	}
	
	// Check against blocked content types
	for _, blocked := range ilv.config.BlockedContentTypes {
		if mediaType == blocked {
			return fmt.Errorf("content type %s is blocked", mediaType)
		}
	}
	
	// Check against allowed content types
	if len(ilv.config.AllowedContentTypes) > 0 {
		allowed := false
		for _, allowedType := range ilv.config.AllowedContentTypes {
			if mediaType == allowedType {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("content type %s is not allowed", mediaType)
		}
	}
	
	// Validate charset if specified
	if ilv.config.ValidateCharset {
		if charset, exists := params["charset"]; exists {
			if err := ilv.validateCharset(charset); err != nil {
				return err
			}
		}
	}
	
	return nil
}

// ValidateFileType validates file type and extension
func (ilv *InputLimitsValidator) ValidateFileType(filename string, contentType string) error {
	// Extract file extension
	ext := strings.ToLower(filename[strings.LastIndex(filename, "."):])
	
	// Check against blocked file extensions
	for _, blocked := range ilv.config.BlockedFileExtensions {
		if ext == blocked {
			return fmt.Errorf("file extension %s is blocked", ext)
		}
	}
	
	// Check against allowed file extensions
	if len(ilv.config.AllowedFileExtensions) > 0 {
		allowed := false
		for _, allowedExt := range ilv.config.AllowedFileExtensions {
			if ext == allowedExt {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("file extension %s is not allowed", ext)
		}
	}
	
	// Validate MIME type
	if contentType != "" {
		mediaType, _, err := mime.ParseMediaType(contentType)
		if err != nil {
			return fmt.Errorf("invalid MIME type: %w", err)
		}
		
		// Check against blocked MIME types
		for _, blocked := range ilv.config.BlockedMimeTypes {
			if mediaType == blocked {
				return fmt.Errorf("MIME type %s is blocked", mediaType)
			}
		}
		
		// Check against allowed MIME types
		if len(ilv.config.AllowedMimeTypes) > 0 {
			allowed := false
			for _, allowedType := range ilv.config.AllowedMimeTypes {
				if mediaType == allowedType {
					allowed = true
					break
				}
			}
			if !allowed {
				return fmt.Errorf("MIME type %s is not allowed", mediaType)
			}
		}
	}
	
	return nil
}

// ValidateFileSize validates file size limits
func (ilv *InputLimitsValidator) ValidateFileSize(fileSize int64, totalSize int64, fileCount int) error {
	// Check individual file size
	if fileSize > ilv.config.MaxFileSize {
		return fmt.Errorf("file size %d exceeds maximum allowed size %d", fileSize, ilv.config.MaxFileSize)
	}
	
	// Check total file size
	if totalSize > ilv.config.MaxTotalFileSize {
		return fmt.Errorf("total file size %d exceeds maximum allowed size %d", totalSize, ilv.config.MaxTotalFileSize)
	}
	
	// Check file count
	if fileCount > ilv.config.MaxFileCount {
		return fmt.Errorf("file count %d exceeds maximum allowed count %d", fileCount, ilv.config.MaxFileCount)
	}
	
	return nil
}

// ValidateCustomRule validates input against a custom rule
func (ilv *InputLimitsValidator) ValidateCustomRule(field, value, ruleName string) error {
	rule, exists := ilv.config.CustomRules[ruleName]
	if !exists {
		return fmt.Errorf("custom rule %s not found", ruleName)
	}
	
	// Check if required
	if rule.Required && value == "" {
		return fmt.Errorf("%s is required", field)
	}
	
	// Check length limits
	if len(value) < rule.MinLength {
		return fmt.Errorf("%s must be at least %d characters", field, rule.MinLength)
	}
	
	if len(value) > rule.MaxLength {
		return fmt.Errorf("%s must not exceed %d characters", field, rule.MaxLength)
	}
	
	// Check pattern if specified
	if rule.Pattern != "" {
		if err := validatePattern(value, rule.Pattern); err != nil {
			return fmt.Errorf("%s does not match required pattern: %w", field, err)
		}
	}
	
	// Check allowed values if specified
	if len(rule.AllowedValues) > 0 {
		allowed := false
		for _, allowedValue := range rule.AllowedValues {
			if value == allowedValue {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("%s is not an allowed value", field)
		}
	}
	
	return nil
}

// ValidateHTTPHeaders validates HTTP header limits
func (ilv *InputLimitsValidator) ValidateHTTPHeaders(r *http.Request) error {
	// Validate User-Agent length
	userAgent := r.UserAgent()
	if len(userAgent) > ilv.config.MaxUserAgentLength {
		return fmt.Errorf("User-Agent header exceeds maximum length of %d characters", ilv.config.MaxUserAgentLength)
	}
	
	// Validate Referer length
	referer := r.Referer()
	if len(referer) > ilv.config.MaxRefererLength {
		return fmt.Errorf("Referer header exceeds maximum length of %d characters", ilv.config.MaxRefererLength)
	}
	
	// Validate individual header lengths
	for name, values := range r.Header {
		for _, value := range values {
			if len(value) > ilv.config.MaxHeaderLength {
				return fmt.Errorf("header %s exceeds maximum length of %d characters", name, ilv.config.MaxHeaderLength)
			}
		}
	}
	
	// Validate cookie lengths
	for _, cookie := range r.Cookies() {
		if len(cookie.String()) > ilv.config.MaxCookieLength {
			return fmt.Errorf("cookie %s exceeds maximum length of %d characters", cookie.Name, ilv.config.MaxCookieLength)
		}
	}
	
	return nil
}

// ValidateUTF8 validates UTF-8 encoding
func (ilv *InputLimitsValidator) ValidateUTF8(field, value string) error {
	if !ilv.config.ValidateUTF8 {
		return nil
	}
	
	if !isValidUTF8(value) {
		return fmt.Errorf("%s contains invalid UTF-8 characters", field)
	}
	
	return nil
}

// TruncateString truncates a string to the specified length
func (ilv *InputLimitsValidator) TruncateString(value string, maxLength int) string {
	if len(value) <= maxLength {
		return value
	}
	
	// Truncate at UTF-8 character boundary
	if maxLength > 0 {
		for i := maxLength; i >= 0; i-- {
			if i == 0 || isValidUTF8(value[:i]) {
				return value[:i]
			}
		}
	}
	
	return ""
}

// GetInputLimits returns the current input limits configuration
func (ilv *InputLimitsValidator) GetInputLimits() InputLimitsConfig {
	return ilv.config
}

// UpdateInputLimits updates the input limits configuration
func (ilv *InputLimitsValidator) UpdateInputLimits(config InputLimitsConfig) {
	ilv.config = config
}

// Helper functions

func (ilv *InputLimitsValidator) validateCharset(charset string) error {
	// Normalize charset name
	normalizedCharset := strings.ToLower(strings.ReplaceAll(charset, "-", ""))
	
	for _, allowed := range ilv.config.AllowedCharsets {
		normalizedAllowed := strings.ToLower(strings.ReplaceAll(allowed, "-", ""))
		if normalizedCharset == normalizedAllowed {
			return nil
		}
	}
	
	return fmt.Errorf("charset %s is not allowed", charset)
}

func validatePattern(value, pattern string) error {
	// This is a simplified pattern validation
	// In a real implementation, you would use regexp.MatchString
	if pattern == "" {
		return nil
	}
	
	// For now, just check if it's not empty
	if value == "" {
		return fmt.Errorf("value cannot be empty")
	}
	
	return nil
}

func isValidUTF8(s string) bool {
	// Simple UTF-8 validation
	for _, r := range s {
		if r == '\uFFFD' {
			return false // Invalid UTF-8 sequence
		}
	}
	return true
}

// InputLimitsMiddleware returns HTTP middleware for input limits validation
func (ilv *InputLimitsValidator) InputLimitsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Validate content type
		if err := ilv.ValidateContentType(r); err != nil {
			http.Error(w, err.Error(), http.StatusUnsupportedMediaType)
			return
		}
		
		// Validate HTTP headers
		if err := ilv.ValidateHTTPHeaders(r); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		
		next.ServeHTTP(w, r)
	})
}