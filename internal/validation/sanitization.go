package validation

import (
	"html"
	"net/url"
	"regexp"
	"strings"
	"unicode"

	"github.com/microcosm-cc/bluemonday"
)

// SanitizationPolicy defines different levels of sanitization
type SanitizationPolicy int

const (
	// StrictPolicy removes all HTML and dangerous content
	StrictPolicy SanitizationPolicy = iota
	// BasicPolicy allows basic formatting but removes dangerous content
	BasicPolicy
	// RelaxedPolicy allows more HTML but still removes dangerous content
	RelaxedPolicy
)

// Sanitizer provides comprehensive HTML/XSS sanitization
type Sanitizer struct {
	strictPolicy  *bluemonday.Policy
	basicPolicy   *bluemonday.Policy
	relaxedPolicy *bluemonday.Policy
}

// NewSanitizer creates a new sanitizer with predefined policies
func NewSanitizer() *Sanitizer {
	return &Sanitizer{
		strictPolicy:  createStrictPolicy(),
		basicPolicy:   createBasicPolicy(),
		relaxedPolicy: createRelaxedPolicy(),
	}
}

// SanitizeHTML sanitizes HTML content based on the specified policy
func (s *Sanitizer) SanitizeHTML(input string, policy SanitizationPolicy) string {
	switch policy {
	case StrictPolicy:
		return s.strictPolicy.Sanitize(input)
	case BasicPolicy:
		return s.basicPolicy.Sanitize(input)
	case RelaxedPolicy:
		return s.relaxedPolicy.Sanitize(input)
	default:
		return s.strictPolicy.Sanitize(input)
	}
}

// SanitizeUserInput sanitizes user input with comprehensive XSS protection
func (s *Sanitizer) SanitizeUserInput(input string) string {
	// Step 1: Remove dangerous characters and patterns
	sanitized := s.removeDangerousPatterns(input)
	
	// Step 2: Apply HTML sanitization
	sanitized = s.strictPolicy.Sanitize(sanitized)
	
	// Step 3: Escape HTML entities
	sanitized = html.EscapeString(sanitized)
	
	// Step 4: Remove/replace dangerous Unicode characters
	sanitized = s.sanitizeUnicode(sanitized)
	
	// Step 5: Normalize whitespace
	sanitized = s.normalizeWhitespace(sanitized)
	
	return strings.TrimSpace(sanitized)
}

// SanitizeJSON sanitizes JSON content to prevent injection attacks
func (s *Sanitizer) SanitizeJSON(input string) string {
	// Remove dangerous patterns that could break JSON parsing
	sanitized := s.removeDangerousJSONPatterns(input)
	
	// Escape dangerous characters
	sanitized = s.escapeJSONDangerousChars(sanitized)
	
	return sanitized
}

// SanitizeURL sanitizes and validates URLs
func (s *Sanitizer) SanitizeURL(input string) (string, error) {
	// Parse the URL
	parsedURL, err := url.Parse(input)
	if err != nil {
		return "", err
	}
	
	// Only allow safe schemes
	safeSchemes := []string{"http", "https", "ftp", "ftps"}
	schemeAllowed := false
	for _, scheme := range safeSchemes {
		if parsedURL.Scheme == scheme {
			schemeAllowed = true
			break
		}
	}
	
	if !schemeAllowed {
		return "", &ValidationError{
			Field:   "url",
			Tag:     "safe_url",
			Message: "URL scheme not allowed",
		}
	}
	
	// Sanitize the URL components
	parsedURL.Path = s.SanitizeUserInput(parsedURL.Path)
	parsedURL.RawQuery = s.SanitizeUserInput(parsedURL.RawQuery)
	parsedURL.Fragment = s.SanitizeUserInput(parsedURL.Fragment)
	
	return parsedURL.String(), nil
}

// RemoveXSSPatterns removes known XSS attack patterns
func (s *Sanitizer) RemoveXSSPatterns(input string) string {
	// Common XSS patterns to remove
	xssPatterns := []string{
		`<script[^>]*>.*?</script>`,
		`<iframe[^>]*>.*?</iframe>`,
		`<object[^>]*>.*?</object>`,
		`<embed[^>]*>.*?</embed>`,
		`<applet[^>]*>.*?</applet>`,
		`<meta[^>]*>`,
		`<base[^>]*>`,
		`<link[^>]*>`,
		`<style[^>]*>.*?</style>`,
		`javascript:`,
		`vbscript:`,
		`data:`,
		`onload\s*=`,
		`onerror\s*=`,
		`onclick\s*=`,
		`onmouseover\s*=`,
		`onmouseout\s*=`,
		`onfocus\s*=`,
		`onblur\s*=`,
		`onchange\s*=`,
		`onsubmit\s*=`,
		`onreset\s*=`,
		`onkeydown\s*=`,
		`onkeyup\s*=`,
		`onkeypress\s*=`,
	}
	
	result := input
	for _, pattern := range xssPatterns {
		re := regexp.MustCompile(`(?i)` + pattern)
		result = re.ReplaceAllString(result, "")
	}
	
	return result
}

// removeDangerousPatterns removes dangerous patterns from input
func (s *Sanitizer) removeDangerousPatterns(input string) string {
	// Remove script injection patterns
	result := s.RemoveXSSPatterns(input)
	
	// Remove SQL injection patterns
	result = s.removeSQLInjectionPatterns(result)
	
	// Remove command injection patterns
	result = s.removeCommandInjectionPatterns(result)
	
	// Remove path traversal patterns
	result = s.removePathTraversalPatterns(result)
	
	return result
}

// removeSQLInjectionPatterns removes SQL injection patterns
func (s *Sanitizer) removeSQLInjectionPatterns(input string) string {
	sqlPatterns := []string{
		`(?i)union\s+select`,
		`(?i)drop\s+table`,
		`(?i)delete\s+from`,
		`(?i)insert\s+into`,
		`(?i)update\s+.*\s+set`,
		`(?i)exec\s*\(`,
		`(?i)execute\s*\(`,
		`(?i)sp_\w+`,
		`(?i)xp_\w+`,
		`--`,
		`/\*`,
		`\*/`,
		`';`,
		`'or`,
		`'and`,
		`'union`,
		`'drop`,
		`'delete`,
		`'insert`,
		`'update`,
		`'exec`,
		`'execute`,
	}
	
	result := input
	for _, pattern := range sqlPatterns {
		re := regexp.MustCompile(pattern)
		result = re.ReplaceAllString(result, "")
	}
	
	return result
}

// removeCommandInjectionPatterns removes command injection patterns
func (s *Sanitizer) removeCommandInjectionPatterns(input string) string {
	commandPatterns := []string{
		`(?i)rm\s+`,
		`(?i)del\s+`,
		`(?i)format\s+`,
		`(?i)shutdown\s+`,
		`(?i)reboot\s+`,
		`(?i)halt\s+`,
		`(?i)poweroff\s+`,
		`(?i)wget\s+`,
		`(?i)curl\s+`,
		`(?i)nc\s+`,
		`(?i)netcat\s+`,
		`(?i)telnet\s+`,
		`(?i)ssh\s+`,
		`(?i)ftp\s+`,
		`(?i)tftp\s+`,
		`&&`,
		`\|\|`,
		`\|`,
		`\$\(`,
		`\$\{`,
		"`",
		`;`,
	}
	
	result := input
	for _, pattern := range commandPatterns {
		re := regexp.MustCompile(pattern)
		result = re.ReplaceAllString(result, "")
	}
	
	return result
}

// removePathTraversalPatterns removes path traversal patterns
func (s *Sanitizer) removePathTraversalPatterns(input string) string {
	pathPatterns := []string{
		`\.\./`,
		`\.\.\\`,
		`\.\.\.\./`,
		`\.\.\.\.\\`,
		`%2e%2e%2f`,
		`%2e%2e%5c`,
		`%252e%252e%252f`,
		`%252e%252e%255c`,
		`..%2f`,
		`..%5c`,
		`%2e%2e/`,
		`%2e%2e\\`,
	}
	
	result := input
	for _, pattern := range pathPatterns {
		re := regexp.MustCompile(`(?i)` + pattern)
		result = re.ReplaceAllString(result, "")
	}
	
	return result
}

// removeDangerousJSONPatterns removes patterns that could break JSON parsing
func (s *Sanitizer) removeDangerousJSONPatterns(input string) string {
	// Remove patterns that could break JSON structure
	jsonPatterns := []string{
		`"}[^"]*{"`,  // JSON injection
		`":\s*"[^"]*"[^,}]*[,}]`, // Malformed JSON values
		`__proto__`,   // Prototype pollution
		`constructor`, // Constructor pollution
		`prototype`,   // Prototype access
	}
	
	result := input
	for _, pattern := range jsonPatterns {
		re := regexp.MustCompile(pattern)
		result = re.ReplaceAllString(result, "")
	}
	
	return result
}

// escapeJSONDangerousChars escapes characters that could be dangerous in JSON
func (s *Sanitizer) escapeJSONDangerousChars(input string) string {
	result := input
	
	// Escape dangerous characters
	result = strings.ReplaceAll(result, `\`, `\\`)
	result = strings.ReplaceAll(result, `"`, `\"`)
	result = strings.ReplaceAll(result, "\n", `\n`)
	result = strings.ReplaceAll(result, "\r", `\r`)
	result = strings.ReplaceAll(result, "\t", `\t`)
	
	return result
}

// sanitizeUnicode removes or replaces dangerous Unicode characters
func (s *Sanitizer) sanitizeUnicode(input string) string {
	var result strings.Builder
	
	for _, r := range input {
		// Skip control characters except for common whitespace
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			continue
		}
		
		// Skip private use characters
		if unicode.In(r, unicode.Co) {
			continue
		}
		
		// Skip surrogate characters
		if unicode.IsSurrogate(r) {
			continue
		}
		
		// Skip non-printable characters
		if !unicode.IsPrint(r) && !unicode.IsSpace(r) {
			continue
		}
		
		result.WriteRune(r)
	}
	
	return result.String()
}

// normalizeWhitespace normalizes whitespace characters
func (s *Sanitizer) normalizeWhitespace(input string) string {
	// Replace multiple whitespace with single space
	re := regexp.MustCompile(`\s+`)
	result := re.ReplaceAllString(input, " ")
	
	// Remove leading/trailing whitespace
	result = strings.TrimSpace(result)
	
	return result
}

// Policy creation functions

func createStrictPolicy() *bluemonday.Policy {
	// Strict policy that removes all HTML
	return bluemonday.StrictPolicy()
}

func createBasicPolicy() *bluemonday.Policy {
	// Basic policy that allows some safe formatting
	policy := bluemonday.NewPolicy()
	
	// Allow basic text formatting
	policy.AllowElements("b", "strong", "i", "em", "u", "s", "br", "p")
	
	// Allow links but sanitize them
	policy.AllowAttrs("href").OnElements("a")
	policy.RequireNoFollowOnLinks(true)
	
	return policy
}

func createRelaxedPolicy() *bluemonday.Policy {
	// Relaxed policy that allows more HTML but still removes dangerous content
	policy := bluemonday.NewPolicy()
	
	// Allow basic text formatting
	policy.AllowElements("b", "strong", "i", "em", "u", "s", "br", "p", "div", "span")
	
	// Allow lists
	policy.AllowElements("ul", "ol", "li")
	
	// Allow headings
	policy.AllowElements("h1", "h2", "h3", "h4", "h5", "h6")
	
	// Allow links but sanitize them
	policy.AllowAttrs("href").OnElements("a")
	policy.RequireNoFollowOnLinks(true)
	
	// Allow images but sanitize them
	policy.AllowAttrs("src", "alt").OnElements("img")
	policy.AllowAttrs("width", "height").OnElements("img")
	
	// Allow tables
	policy.AllowElements("table", "thead", "tbody", "tfoot", "tr", "td", "th")
	
	// Allow code blocks
	policy.AllowElements("code", "pre")
	
	return policy
}

// AdvancedSanitizationOptions provides additional sanitization options
type AdvancedSanitizationOptions struct {
	RemoveComments      bool
	RemoveEmptyElements bool
	MaxLength           int
	AllowedProtocols    []string
	BlockedDomains      []string
	CustomPatterns      []string
}

// AdvancedSanitize performs advanced sanitization with custom options
func (s *Sanitizer) AdvancedSanitize(input string, options AdvancedSanitizationOptions) string {
	result := input
	
	// Apply custom patterns
	for _, pattern := range options.CustomPatterns {
		re := regexp.MustCompile(pattern)
		result = re.ReplaceAllString(result, "")
	}
	
	// Remove comments if requested
	if options.RemoveComments {
		result = s.removeComments(result)
	}
	
	// Remove empty elements if requested
	if options.RemoveEmptyElements {
		result = s.removeEmptyElements(result)
	}
	
	// Truncate if max length is specified
	if options.MaxLength > 0 && len(result) > options.MaxLength {
		result = result[:options.MaxLength]
	}
	
	// Apply basic sanitization
	result = s.SanitizeUserInput(result)
	
	return result
}

// removeComments removes HTML comments
func (s *Sanitizer) removeComments(input string) string {
	re := regexp.MustCompile(`<!--.*?-->`)
	return re.ReplaceAllString(input, "")
}

// removeEmptyElements removes empty HTML elements
func (s *Sanitizer) removeEmptyElements(input string) string {
	re := regexp.MustCompile(`<(\w+)[^>]*></\1>`)
	return re.ReplaceAllString(input, "")
}