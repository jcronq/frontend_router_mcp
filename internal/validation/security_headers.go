package validation

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/csrf"
	"github.com/unrolled/secure"
)

// SecurityHeadersConfig configures security headers and CSRF protection
type SecurityHeadersConfig struct {
	// CSRF Protection
	EnableCSRF           bool          `json:"enable_csrf"`
	CSRFTokenLength      int           `json:"csrf_token_length"`
	CSRFSecretKey        string        `json:"csrf_secret_key"`
	CSRFCookieName       string        `json:"csrf_cookie_name"`
	CSRFHeaderName       string        `json:"csrf_header_name"`
	CSRFFieldName        string        `json:"csrf_field_name"`
	CSRFCookieMaxAge     int           `json:"csrf_cookie_max_age"`
	CSRFCookieHttpOnly   bool          `json:"csrf_cookie_http_only"`
	CSRFCookieSecure     bool          `json:"csrf_cookie_secure"`
	CSRFCookieSameSite   http.SameSite `json:"csrf_cookie_same_site"`
	CSRFTrustedOrigins   []string      `json:"csrf_trusted_origins"`
	
	// Security Headers
	EnableHSTS           bool          `json:"enable_hsts"`
	HSTSMaxAge           int           `json:"hsts_max_age"`
	HSTSIncludeSubDomains bool         `json:"hsts_include_subdomains"`
	HSTSPreload          bool          `json:"hsts_preload"`
	
	EnableXSSProtection  bool          `json:"enable_xss_protection"`
	XSSProtectionValue   string        `json:"xss_protection_value"`
	
	EnableFrameOptions   bool          `json:"enable_frame_options"`
	FrameOptionsValue    string        `json:"frame_options_value"`
	
	EnableContentTypeOptions bool       `json:"enable_content_type_options"`
	ContentTypeOptionsValue  string     `json:"content_type_options_value"`
	
	EnableReferrerPolicy bool          `json:"enable_referrer_policy"`
	ReferrerPolicyValue  string        `json:"referrer_policy_value"`
	
	EnableCSP            bool          `json:"enable_csp"`
	CSPValue             string        `json:"csp_value"`
	CSPReportOnly        bool          `json:"csp_report_only"`
	
	EnablePermissionsPolicy bool       `json:"enable_permissions_policy"`
	PermissionsPolicyValue  string     `json:"permissions_policy_value"`
	
	// CORS Settings
	EnableCORS           bool          `json:"enable_cors"`
	CORSAllowOrigins     []string      `json:"cors_allow_origins"`
	CORSAllowMethods     []string      `json:"cors_allow_methods"`
	CORSAllowHeaders     []string      `json:"cors_allow_headers"`
	CORSExposeHeaders    []string      `json:"cors_expose_headers"`
	CORSMaxAge           int           `json:"cors_max_age"`
	CORSAllowCredentials bool          `json:"cors_allow_credentials"`
	
	// Custom Headers
	CustomHeaders        map[string]string `json:"custom_headers"`
	
	// Security Settings
	RequireSecureHeaders bool          `json:"require_secure_headers"`
	ValidateOrigin       bool          `json:"validate_origin"`
	ValidateReferer      bool          `json:"validate_referer"`
	BlockMixedContent    bool          `json:"block_mixed_content"`
	
	// Development Settings
	IsDevelopment        bool          `json:"is_development"`
	ForceHTTPS           bool          `json:"force_https"`
}

// DefaultSecurityHeadersConfig provides secure default configuration
var DefaultSecurityHeadersConfig = SecurityHeadersConfig{
	EnableCSRF:           true,
	CSRFTokenLength:      32,
	CSRFCookieName:       "csrf_token",
	CSRFHeaderName:       "X-CSRF-Token",
	CSRFFieldName:        "csrf_token",
	CSRFCookieMaxAge:     3600, // 1 hour
	CSRFCookieHttpOnly:   true,
	CSRFCookieSecure:     true,
	CSRFCookieSameSite:   http.SameSiteStrictMode,
	CSRFTrustedOrigins:   []string{"https://localhost", "https://127.0.0.1"},
	
	EnableHSTS:           true,
	HSTSMaxAge:           31536000, // 1 year
	HSTSIncludeSubDomains: true,
	HSTSPreload:          true,
	
	EnableXSSProtection:  true,
	XSSProtectionValue:   "1; mode=block",
	
	EnableFrameOptions:   true,
	FrameOptionsValue:    "DENY",
	
	EnableContentTypeOptions: true,
	ContentTypeOptionsValue:  "nosniff",
	
	EnableReferrerPolicy: true,
	ReferrerPolicyValue:  "strict-origin-when-cross-origin",
	
	EnableCSP:            true,
	CSPValue:             "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self'; media-src 'self'; object-src 'none'; child-src 'none'; frame-src 'none'; worker-src 'none'; frame-ancestors 'none'; form-action 'self'; base-uri 'self'; manifest-src 'self';",
	CSPReportOnly:        false,
	
	EnablePermissionsPolicy: true,
	PermissionsPolicyValue:  "camera=(), microphone=(), geolocation=(), interest-cohort=()",
	
	EnableCORS:           true,
	CORSAllowOrigins:     []string{"https://localhost", "https://127.0.0.1"},
	CORSAllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
	CORSAllowHeaders:     []string{"Content-Type", "Authorization", "X-Requested-With", "X-CSRF-Token"},
	CORSExposeHeaders:    []string{"Content-Length", "X-JSON"},
	CORSMaxAge:           86400, // 24 hours
	CORSAllowCredentials: true,
	
	CustomHeaders:        make(map[string]string),
	RequireSecureHeaders: true,
	ValidateOrigin:       true,
	ValidateReferer:      true,
	BlockMixedContent:    true,
	IsDevelopment:        false,
	ForceHTTPS:           true,
}

// SecurityHeadersMiddleware provides comprehensive security headers
type SecurityHeadersMiddleware struct {
	config       SecurityHeadersConfig
	secureMiddleware *secure.Secure
	csrfMiddleware   func(http.Handler) http.Handler
}

// NewSecurityHeadersMiddleware creates a new security headers middleware
func NewSecurityHeadersMiddleware(config SecurityHeadersConfig) *SecurityHeadersMiddleware {
	middleware := &SecurityHeadersMiddleware{
		config: config,
	}
	
	// Initialize secure middleware
	middleware.secureMiddleware = secure.New(secure.Options{
		AllowedHosts:          []string{},
		AllowedHostsAreRegex:  false,
		HostsProxyHeaders:     []string{"X-Forwarded-Host"},
		SSLRedirect:           config.ForceHTTPS && !config.IsDevelopment,
		SSLTemporaryRedirect:  false,
		SSLHost:              "",
		SSLProxyHeaders:      map[string]string{"X-Forwarded-Proto": "https"},
		STSSeconds:           int64(config.HSTSMaxAge),
		STSIncludeSubdomains: config.HSTSIncludeSubDomains,
		STSPreload:          config.HSTSPreload,
		ForceSTSHeader:      config.EnableHSTS,
		FrameDeny:           config.EnableFrameOptions && config.FrameOptionsValue == "DENY",
		CustomFrameOptionsValue: config.FrameOptionsValue,
		ContentTypeNosniff:  config.EnableContentTypeOptions,
		BrowserXssFilter:    config.EnableXSSProtection,
		ContentSecurityPolicy: config.CSPValue,
		ReferrerPolicy:      config.ReferrerPolicyValue,
		PermissionsPolicy:   config.PermissionsPolicyValue,
		IsDevelopment:       config.IsDevelopment,
	})
	
	// Initialize CSRF middleware if enabled
	if config.EnableCSRF {
		middleware.initializeCSRF()
	}
	
	return middleware
}

// Middleware returns the combined security middleware
func (shm *SecurityHeadersMiddleware) Middleware(next http.Handler) http.Handler {
	handler := next
	
	// Apply CSRF protection if enabled
	if shm.config.EnableCSRF && shm.csrfMiddleware != nil {
		handler = shm.csrfMiddleware(handler)
	}
	
	// Apply security headers
	handler = shm.secureMiddleware.Handler(handler)
	
	// Apply custom security logic
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Add custom security headers
		shm.addCustomHeaders(w, r)
		
		// Validate origin if enabled
		if shm.config.ValidateOrigin {
			if err := shm.validateOrigin(r); err != nil {
				http.Error(w, "Invalid origin", http.StatusForbidden)
				return
			}
		}
		
		// Validate referer if enabled
		if shm.config.ValidateReferer {
			if err := shm.validateReferer(r); err != nil {
				http.Error(w, "Invalid referer", http.StatusForbidden)
				return
			}
		}
		
		// Handle CORS preflight requests
		if shm.config.EnableCORS && r.Method == "OPTIONS" {
			shm.handleCORSPreflight(w, r)
			return
		}
		
		// Add CORS headers for actual requests
		if shm.config.EnableCORS {
			shm.addCORSHeaders(w, r)
		}
		
		handler.ServeHTTP(w, r)
	})
}

// WebSocketUpgradeMiddleware provides security for WebSocket upgrades
func (shm *SecurityHeadersMiddleware) WebSocketUpgradeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Validate WebSocket upgrade request
		if err := shm.validateWebSocketUpgrade(r); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		
		// Add security headers for WebSocket upgrade
		shm.addWebSocketSecurityHeaders(w, r)
		
		next.ServeHTTP(w, r)
	})
}

// initializeCSRF initializes CSRF protection
func (shm *SecurityHeadersMiddleware) initializeCSRF() {
	// Generate secret key if not provided
	if shm.config.CSRFSecretKey == "" {
		shm.config.CSRFSecretKey = generateSecretKey(32)
	}
	
	// Configure CSRF middleware
	shm.csrfMiddleware = csrf.Protect(
		[]byte(shm.config.CSRFSecretKey),
		csrf.Secure(shm.config.CSRFCookieSecure),
		csrf.HttpOnly(shm.config.CSRFCookieHttpOnly),
		csrf.SameSite(shm.config.CSRFCookieSameSite),
		csrf.MaxAge(shm.config.CSRFCookieMaxAge),
		csrf.FieldName(shm.config.CSRFFieldName),
		csrf.CookieName(shm.config.CSRFCookieName),
		csrf.RequestHeader(shm.config.CSRFHeaderName),
		csrf.TrustedOrigins(shm.config.CSRFTrustedOrigins...),
		csrf.ErrorHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "CSRF token validation failed", http.StatusForbidden)
		})),
	)
}

// addCustomHeaders adds custom security headers
func (shm *SecurityHeadersMiddleware) addCustomHeaders(w http.ResponseWriter, r *http.Request) {
	// Add custom headers
	for name, value := range shm.config.CustomHeaders {
		w.Header().Set(name, value)
	}
	
	// Add security-specific headers
	if shm.config.BlockMixedContent {
		w.Header().Set("Content-Security-Policy", "upgrade-insecure-requests")
	}
	
	// Add server header (optional)
	w.Header().Set("Server", "Frontend-Router-MCP")
	
	// Add security headers for API responses
	if strings.HasPrefix(r.URL.Path, "/api/") {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
	}
}

// validateOrigin validates the Origin header
func (shm *SecurityHeadersMiddleware) validateOrigin(r *http.Request) error {
	origin := r.Header.Get("Origin")
	if origin == "" {
		// Allow requests without Origin header (e.g., same-origin requests)
		return nil
	}
	
	// Check against allowed origins
	for _, allowed := range shm.config.CORSAllowOrigins {
		if origin == allowed {
			return nil
		}
	}
	
	return fmt.Errorf("origin %s not allowed", origin)
}

// validateReferer validates the Referer header
func (shm *SecurityHeadersMiddleware) validateReferer(r *http.Request) error {
	referer := r.Header.Get("Referer")
	if referer == "" {
		// Allow requests without Referer header
		return nil
	}
	
	// For state-changing requests, validate referer
	if r.Method == "POST" || r.Method == "PUT" || r.Method == "DELETE" {
		// Check if referer matches allowed origins
		for _, allowed := range shm.config.CORSAllowOrigins {
			if strings.HasPrefix(referer, allowed) {
				return nil
			}
		}
		
		return fmt.Errorf("referer %s not allowed", referer)
	}
	
	return nil
}

// handleCORSPreflight handles CORS preflight requests
func (shm *SecurityHeadersMiddleware) handleCORSPreflight(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	
	// Check if origin is allowed
	originAllowed := false
	for _, allowed := range shm.config.CORSAllowOrigins {
		if origin == allowed {
			originAllowed = true
			break
		}
	}
	
	if !originAllowed {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	
	// Add CORS headers
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Methods", strings.Join(shm.config.CORSAllowMethods, ", "))
	w.Header().Set("Access-Control-Allow-Headers", strings.Join(shm.config.CORSAllowHeaders, ", "))
	w.Header().Set("Access-Control-Max-Age", fmt.Sprintf("%d", shm.config.CORSMaxAge))
	
	if shm.config.CORSAllowCredentials {
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	}
	
	w.WriteHeader(http.StatusOK)
}

// addCORSHeaders adds CORS headers for actual requests
func (shm *SecurityHeadersMiddleware) addCORSHeaders(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	
	// Check if origin is allowed
	for _, allowed := range shm.config.CORSAllowOrigins {
		if origin == allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Expose-Headers", strings.Join(shm.config.CORSExposeHeaders, ", "))
			
			if shm.config.CORSAllowCredentials {
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}
			
			break
		}
	}
}

// validateWebSocketUpgrade validates WebSocket upgrade requests
func (shm *SecurityHeadersMiddleware) validateWebSocketUpgrade(r *http.Request) error {
	// Check for required WebSocket headers
	if r.Header.Get("Upgrade") != "websocket" {
		return fmt.Errorf("missing or invalid Upgrade header")
	}
	
	if r.Header.Get("Connection") != "Upgrade" {
		return fmt.Errorf("missing or invalid Connection header")
	}
	
	if r.Header.Get("Sec-WebSocket-Version") != "13" {
		return fmt.Errorf("unsupported WebSocket version")
	}
	
	if r.Header.Get("Sec-WebSocket-Key") == "" {
		return fmt.Errorf("missing Sec-WebSocket-Key header")
	}
	
	// Validate origin for WebSocket connections
	if shm.config.ValidateOrigin {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return fmt.Errorf("missing Origin header")
		}
		
		originAllowed := false
		for _, allowed := range shm.config.CORSAllowOrigins {
			if origin == allowed {
				originAllowed = true
				break
			}
		}
		
		if !originAllowed {
			return fmt.Errorf("origin %s not allowed for WebSocket connections", origin)
		}
	}
	
	return nil
}

// addWebSocketSecurityHeaders adds security headers for WebSocket upgrades
func (shm *SecurityHeadersMiddleware) addWebSocketSecurityHeaders(w http.ResponseWriter, r *http.Request) {
	// Add security headers that are compatible with WebSocket upgrades
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	
	// Add custom headers
	for name, value := range shm.config.CustomHeaders {
		w.Header().Set(name, value)
	}
}

// GetCSRFToken returns the CSRF token for the current request
func (shm *SecurityHeadersMiddleware) GetCSRFToken(r *http.Request) string {
	if !shm.config.EnableCSRF {
		return ""
	}
	
	return csrf.Token(r)
}

// ValidateCSRFToken validates a CSRF token
func (shm *SecurityHeadersMiddleware) ValidateCSRFToken(r *http.Request, token string) bool {
	if !shm.config.EnableCSRF {
		return true
	}
	
	expectedToken := csrf.Token(r)
	return subtle.ConstantTimeCompare([]byte(token), []byte(expectedToken)) == 1
}

// generateSecretKey generates a random secret key
func generateSecretKey(length int) string {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		panic(err)
	}
	return base64.URLEncoding.EncodeToString(bytes)
}

// SecurityAuditLog logs security events
type SecurityAuditLog struct {
	Timestamp time.Time `json:"timestamp"`
	ClientIP  string    `json:"client_ip"`
	UserAgent string    `json:"user_agent"`
	Event     string    `json:"event"`
	Details   string    `json:"details"`
	Severity  string    `json:"severity"`
}

// LogSecurityEvent logs a security event
func (shm *SecurityHeadersMiddleware) LogSecurityEvent(r *http.Request, event, details, severity string) {
	log := SecurityAuditLog{
		Timestamp: time.Now(),
		ClientIP:  getClientIP(r),
		UserAgent: r.UserAgent(),
		Event:     event,
		Details:   details,
		Severity:  severity,
	}
	
	// In a real implementation, you would log this to a security monitoring system
	fmt.Printf("SECURITY EVENT: %+v\n", log)
}

// IsSecureContext checks if the request is in a secure context
func (shm *SecurityHeadersMiddleware) IsSecureContext(r *http.Request) bool {
	// Check if request is over HTTPS
	if r.TLS != nil {
		return true
	}
	
	// Check for forwarded HTTPS headers
	if r.Header.Get("X-Forwarded-Proto") == "https" {
		return true
	}
	
	// Allow localhost in development
	if shm.config.IsDevelopment {
		host := r.Host
		if strings.HasPrefix(host, "localhost") || strings.HasPrefix(host, "127.0.0.1") {
			return true
		}
	}
	
	return false
}