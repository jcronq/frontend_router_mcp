package validation

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jcronq/frontend_router_mcp/pkg/messaging"
)

// WebSocketValidator provides comprehensive validation for WebSocket connections
type WebSocketValidator struct {
	validator         *Validator
	sanitizer         *Sanitizer
	rateLimiter       *RateLimiter
	requestValidator  *RequestValidator
	securityHeaders   *SecurityHeadersMiddleware
	config            WebSocketValidationConfig
	
	// Connection tracking
	connections       map[string]*ValidatedConnection
	connectionsMutex  sync.RWMutex
	
	// Message tracking
	messageStats      map[string]*MessageStats
	messageStatsMutex sync.RWMutex
}

// WebSocketValidationConfig configures WebSocket validation
type WebSocketValidationConfig struct {
	// Message validation
	MaxMessageSize        int           `json:"max_message_size"`
	MaxPayloadSize        int           `json:"max_payload_size"`
	AllowedMessageTypes   []string      `json:"allowed_message_types"`
	RequireMessageID      bool          `json:"require_message_id"`
	RequireTimestamp      bool          `json:"require_timestamp"`
	ValidateJSONStructure bool          `json:"validate_json_structure"`
	
	// Connection limits
	MaxConnections        int           `json:"max_connections"`
	MaxConnectionsPerIP   int           `json:"max_connections_per_ip"`
	ConnectionTimeout     time.Duration `json:"connection_timeout"`
	ReadTimeout           time.Duration `json:"read_timeout"`
	WriteTimeout          time.Duration `json:"write_timeout"`
	PingInterval          time.Duration `json:"ping_interval"`
	PongTimeout           time.Duration `json:"pong_timeout"`
	
	// Rate limiting
	MaxMessagesPerSecond  int           `json:"max_messages_per_second"`
	MaxBytesPerSecond     int64         `json:"max_bytes_per_second"`
	BurstLimit            int           `json:"burst_limit"`
	
	// Security settings
	RequireOrigin         bool          `json:"require_origin"`
	AllowedOrigins        []string      `json:"allowed_origins"`
	RequireAuthentication bool          `json:"require_authentication"`
	ValidateSubprotocols  bool          `json:"validate_subprotocols"`
	AllowedSubprotocols   []string      `json:"allowed_subprotocols"`
	
	// Cleanup settings
	CleanupInterval       time.Duration `json:"cleanup_interval"`
	StatsRetentionPeriod  time.Duration `json:"stats_retention_period"`
	
	// Error handling
	SendErrorMessages     bool          `json:"send_error_messages"`
	LogValidationErrors   bool          `json:"log_validation_errors"`
	DisconnectOnError     bool          `json:"disconnect_on_error"`
	MaxErrorsPerConnection int          `json:"max_errors_per_connection"`
}

// DefaultWebSocketValidationConfig provides secure defaults
var DefaultWebSocketValidationConfig = WebSocketValidationConfig{
	MaxMessageSize:        65536,    // 64KB
	MaxPayloadSize:        32768,    // 32KB
	AllowedMessageTypes:   []string{"connect", "disconnect", "user_response", "ask_user", "ping", "pong"},
	RequireMessageID:      true,
	RequireTimestamp:      true,
	ValidateJSONStructure: true,
	MaxConnections:        500,
	MaxConnectionsPerIP:   10,
	ConnectionTimeout:     30 * time.Second,
	ReadTimeout:           60 * time.Second,
	WriteTimeout:          30 * time.Second,
	PingInterval:          30 * time.Second,
	PongTimeout:           10 * time.Second,
	MaxMessagesPerSecond:  100,
	MaxBytesPerSecond:     1024 * 1024, // 1MB
	BurstLimit:            200,
	RequireOrigin:         true,
	AllowedOrigins:        []string{"https://localhost", "https://127.0.0.1"},
	RequireAuthentication: false,
	ValidateSubprotocols:  true,
	AllowedSubprotocols:   []string{"mcp-websocket"},
	CleanupInterval:       5 * time.Minute,
	StatsRetentionPeriod:  24 * time.Hour,
	SendErrorMessages:     true,
	LogValidationErrors:   true,
	DisconnectOnError:     false,
	MaxErrorsPerConnection: 10,
}

// ValidatedConnection represents a validated WebSocket connection
type ValidatedConnection struct {
	ID                string
	Conn              *websocket.Conn
	RemoteAddr        string
	Origin            string
	UserAgent         string
	Subprotocols      []string
	ConnectedAt       time.Time
	LastActivity      time.Time
	MessageCount      int64
	BytesReceived     int64
	BytesSent         int64
	ErrorCount        int
	IsAuthenticated   bool
	UserID            string
	Context           context.Context
	CancelFunc        context.CancelFunc
}

// MessageStats tracks message statistics
type MessageStats struct {
	ClientID          string
	MessageType       string
	Count             int64
	TotalBytes        int64
	LastMessageTime   time.Time
	ErrorCount        int64
	ValidationErrors  []string
}

// WebSocketError represents a WebSocket validation error
type WebSocketError struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	Type      string `json:"type"`
	Timestamp time.Time `json:"timestamp"`
}

// NewWebSocketValidator creates a new WebSocket validator
func NewWebSocketValidator(config WebSocketValidationConfig) *WebSocketValidator {
	validator := NewValidator()
	sanitizer := NewSanitizer()
	rateLimiter := NewRateLimiter(DefaultRateLimitConfig)
	requestValidator := NewRequestValidator(DefaultRequestValidationConfig)
	securityHeaders := NewSecurityHeadersMiddleware(DefaultSecurityHeadersConfig)
	
	wsv := &WebSocketValidator{
		validator:         validator,
		sanitizer:         sanitizer,
		rateLimiter:       rateLimiter,
		requestValidator:  requestValidator,
		securityHeaders:   securityHeaders,
		config:            config,
		connections:       make(map[string]*ValidatedConnection),
		messageStats:      make(map[string]*MessageStats),
	}
	
	// Start cleanup goroutine
	go wsv.cleanupLoop()
	
	return wsv
}

// ValidateUpgrade validates WebSocket upgrade requests
func (wsv *WebSocketValidator) ValidateUpgrade(w http.ResponseWriter, r *http.Request) error {
	// Validate HTTP request
	if err := wsv.requestValidator.ValidateHTTPRequest(r); err != nil {
		return fmt.Errorf("HTTP validation failed: %w", err)
	}
	
	// Validate WebSocket-specific headers
	if err := wsv.validateWebSocketHeaders(r); err != nil {
		return fmt.Errorf("WebSocket header validation failed: %w", err)
	}
	
	// Validate origin
	if wsv.config.RequireOrigin {
		if err := wsv.validateOrigin(r); err != nil {
			return fmt.Errorf("Origin validation failed: %w", err)
		}
	}
	
	// Validate subprotocols
	if wsv.config.ValidateSubprotocols {
		if err := wsv.validateSubprotocols(r); err != nil {
			return fmt.Errorf("Subprotocol validation failed: %w", err)
		}
	}
	
	// Check connection limits
	if err := wsv.checkConnectionLimits(r); err != nil {
		return fmt.Errorf("Connection limit exceeded: %w", err)
	}
	
	return nil
}

// ValidateConnection validates and tracks a new WebSocket connection
func (wsv *WebSocketValidator) ValidateConnection(conn *websocket.Conn, r *http.Request) (*ValidatedConnection, error) {
	clientID := generateClientID()
	
	// Create connection context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), wsv.config.ConnectionTimeout)
	
	validatedConn := &ValidatedConnection{
		ID:              clientID,
		Conn:            conn,
		RemoteAddr:      r.RemoteAddr,
		Origin:          r.Header.Get("Origin"),
		UserAgent:       r.UserAgent(),
		Subprotocols:    websocket.Subprotocols(r),
		ConnectedAt:     time.Now(),
		LastActivity:    time.Now(),
		Context:         ctx,
		CancelFunc:      cancel,
	}
	
	// Configure connection timeouts
	conn.SetReadDeadline(time.Now().Add(wsv.config.ReadTimeout))
	conn.SetWriteDeadline(time.Now().Add(wsv.config.WriteTimeout))
	
	// Set up ping/pong handlers
	wsv.setupPingPongHandlers(conn, validatedConn)
	
	// Track connection
	wsv.connectionsMutex.Lock()
	wsv.connections[clientID] = validatedConn
	wsv.connectionsMutex.Unlock()
	
	// Add to rate limiter
	if err := wsv.rateLimiter.AddConnection(clientID, getClientIP(r)); err != nil {
		wsv.removeConnection(clientID)
		return nil, fmt.Errorf("Rate limit exceeded: %w", err)
	}
	
	log.Printf("WebSocket connection validated: %s from %s", clientID, validatedConn.RemoteAddr)
	
	return validatedConn, nil
}

// ValidateMessage validates a WebSocket message
func (wsv *WebSocketValidator) ValidateMessage(conn *ValidatedConnection, messageType int, data []byte) (*messaging.Message, error) {
	// Check rate limiting
	if allowed, err := wsv.rateLimiter.AllowMessage(conn.ID); !allowed {
		return nil, fmt.Errorf("Rate limit exceeded: %w", err)
	}
	
	// Validate message size
	if err := wsv.requestValidator.ValidateWebSocketMessage(messageType, data); err != nil {
		return nil, fmt.Errorf("Message size validation failed: %w", err)
	}
	
	// Only validate text messages (JSON)
	if messageType != websocket.TextMessage {
		return nil, fmt.Errorf("Only text messages are supported")
	}
	
	// Parse message
	var rawMessage struct {
		Type      string          `json:"type"`
		Payload   json.RawMessage `json:"payload,omitempty"`
		Timestamp time.Time       `json:"timestamp,omitempty"`
		ID        string          `json:"id,omitempty"`
	}
	
	if err := json.Unmarshal(data, &rawMessage); err != nil {
		return nil, fmt.Errorf("Invalid JSON message: %w", err)
	}
	
	// Validate message structure
	if err := wsv.validateMessageStructure(&rawMessage); err != nil {
		return nil, fmt.Errorf("Message structure validation failed: %w", err)
	}
	
	// Validate message type
	if err := wsv.validateMessageType(rawMessage.Type); err != nil {
		return nil, fmt.Errorf("Message type validation failed: %w", err)
	}
	
	// Validate payload
	if err := wsv.validatePayload(rawMessage.Type, rawMessage.Payload); err != nil {
		return nil, fmt.Errorf("Payload validation failed: %w", err)
	}
	
	// Create validated message
	msg := &messaging.Message{
		ID:        rawMessage.ID,
		Type:      rawMessage.Type,
		Payload:   rawMessage.Payload,
		Timestamp: rawMessage.Timestamp,
		ClientID:  conn.ID,
	}
	
	// Update connection stats
	wsv.updateConnectionStats(conn, len(data))
	
	// Update message stats
	wsv.updateMessageStats(conn.ID, rawMessage.Type, len(data))
	
	return msg, nil
}

// validateWebSocketHeaders validates WebSocket-specific headers
func (wsv *WebSocketValidator) validateWebSocketHeaders(r *http.Request) error {
	// Check required headers
	if r.Header.Get("Upgrade") != "websocket" {
		return fmt.Errorf("Missing or invalid Upgrade header")
	}
	
	if !strings.Contains(r.Header.Get("Connection"), "Upgrade") {
		return fmt.Errorf("Missing or invalid Connection header")
	}
	
	if r.Header.Get("Sec-WebSocket-Version") != "13" {
		return fmt.Errorf("Unsupported WebSocket version")
	}
	
	if r.Header.Get("Sec-WebSocket-Key") == "" {
		return fmt.Errorf("Missing Sec-WebSocket-Key header")
	}
	
	return nil
}

// validateOrigin validates the Origin header
func (wsv *WebSocketValidator) validateOrigin(r *http.Request) error {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return fmt.Errorf("Missing Origin header")
	}
	
	for _, allowed := range wsv.config.AllowedOrigins {
		if origin == allowed {
			return nil
		}
	}
	
	return fmt.Errorf("Origin %s not allowed", origin)
}

// validateSubprotocols validates WebSocket subprotocols
func (wsv *WebSocketValidator) validateSubprotocols(r *http.Request) error {
	requestedProtocols := websocket.Subprotocols(r)
	
	for _, requested := range requestedProtocols {
		for _, allowed := range wsv.config.AllowedSubprotocols {
			if requested == allowed {
				return nil
			}
		}
	}
	
	if len(requestedProtocols) > 0 {
		return fmt.Errorf("None of the requested subprotocols are allowed")
	}
	
	return nil
}

// checkConnectionLimits checks connection limits
func (wsv *WebSocketValidator) checkConnectionLimits(r *http.Request) error {
	wsv.connectionsMutex.RLock()
	defer wsv.connectionsMutex.RUnlock()
	
	// Check global connection limit
	if len(wsv.connections) >= wsv.config.MaxConnections {
		return fmt.Errorf("Maximum connections exceeded")
	}
	
	// Check per-IP connection limit
	clientIP := getClientIP(r)
	ipConnections := 0
	for _, conn := range wsv.connections {
		if strings.HasPrefix(conn.RemoteAddr, clientIP) {
			ipConnections++
		}
	}
	
	if ipConnections >= wsv.config.MaxConnectionsPerIP {
		return fmt.Errorf("Maximum connections per IP exceeded")
	}
	
	return nil
}

// validateMessageStructure validates message structure
func (wsv *WebSocketValidator) validateMessageStructure(msg *struct {
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Timestamp time.Time       `json:"timestamp,omitempty"`
	ID        string          `json:"id,omitempty"`
}) error {
	// Validate required fields
	if msg.Type == "" {
		return fmt.Errorf("Message type is required")
	}
	
	if wsv.config.RequireMessageID && msg.ID == "" {
		return fmt.Errorf("Message ID is required")
	}
	
	if wsv.config.RequireTimestamp && msg.Timestamp.IsZero() {
		return fmt.Errorf("Message timestamp is required")
	}
	
	// Validate payload size
	if len(msg.Payload) > wsv.config.MaxPayloadSize {
		return fmt.Errorf("Payload too large (max %d bytes)", wsv.config.MaxPayloadSize)
	}
	
	return nil
}

// validateMessageType validates message type
func (wsv *WebSocketValidator) validateMessageType(msgType string) error {
	// Sanitize message type
	sanitizedType := wsv.sanitizer.SanitizeUserInput(msgType)
	if sanitizedType != msgType {
		return fmt.Errorf("Message type contains invalid characters")
	}
	
	// Check against allowed types
	for _, allowed := range wsv.config.AllowedMessageTypes {
		if msgType == allowed {
			return nil
		}
	}
	
	return fmt.Errorf("Message type %s not allowed", msgType)
}

// validatePayload validates message payload
func (wsv *WebSocketValidator) validatePayload(msgType string, payload json.RawMessage) error {
	if len(payload) == 0 {
		return nil // Empty payload is allowed
	}
	
	// Validate based on message type
	switch msgType {
	case "user_response":
		var userResponse UserResponse
		if err := json.Unmarshal(payload, &userResponse); err != nil {
			return fmt.Errorf("Invalid user_response payload: %w", err)
		}
		
		if err := wsv.validator.ValidateStruct(&userResponse); err != nil {
			return fmt.Errorf("User response validation failed: %w", err)
		}
		
		// Sanitize the response
		userResponse.Answer = wsv.sanitizer.SanitizeUserInput(userResponse.Answer)
		
	case "ask_user":
		var askUser UserQuestion
		if err := json.Unmarshal(payload, &askUser); err != nil {
			return fmt.Errorf("Invalid ask_user payload: %w", err)
		}
		
		if err := wsv.validator.ValidateStruct(&askUser); err != nil {
			return fmt.Errorf("Ask user validation failed: %w", err)
		}
		
		// Sanitize the question
		askUser.Question = wsv.sanitizer.SanitizeUserInput(askUser.Question)
		
	case "connect":
		var connectMsg ConnectMessage
		if err := json.Unmarshal(payload, &connectMsg); err != nil {
			return fmt.Errorf("Invalid connect payload: %w", err)
		}
		
		if err := wsv.validator.ValidateStruct(&connectMsg); err != nil {
			return fmt.Errorf("Connect message validation failed: %w", err)
		}
		
	case "disconnect":
		var disconnectMsg DisconnectMessage
		if err := json.Unmarshal(payload, &disconnectMsg); err != nil {
			return fmt.Errorf("Invalid disconnect payload: %w", err)
		}
		
		if err := wsv.validator.ValidateStruct(&disconnectMsg); err != nil {
			return fmt.Errorf("Disconnect message validation failed: %w", err)
		}
	}
	
	return nil
}

// setupPingPongHandlers sets up ping/pong handlers for a connection
func (wsv *WebSocketValidator) setupPingPongHandlers(conn *websocket.Conn, validatedConn *ValidatedConnection) {
	// Set ping handler
	conn.SetPingHandler(func(appData string) error {
		validatedConn.LastActivity = time.Now()
		conn.SetReadDeadline(time.Now().Add(wsv.config.ReadTimeout))
		
		// Send pong response
		err := conn.WriteControl(websocket.PongMessage, []byte{}, time.Now().Add(wsv.config.WriteTimeout))
		if err != nil {
			log.Printf("Failed to send pong to %s: %v", validatedConn.ID, err)
		}
		
		return nil
	})
	
	// Set pong handler
	conn.SetPongHandler(func(appData string) error {
		validatedConn.LastActivity = time.Now()
		conn.SetReadDeadline(time.Now().Add(wsv.config.ReadTimeout))
		return nil
	})
}

// updateConnectionStats updates connection statistics
func (wsv *WebSocketValidator) updateConnectionStats(conn *ValidatedConnection, messageSize int) {
	wsv.connectionsMutex.Lock()
	defer wsv.connectionsMutex.Unlock()
	
	conn.MessageCount++
	conn.BytesReceived += int64(messageSize)
	conn.LastActivity = time.Now()
}

// updateMessageStats updates message statistics
func (wsv *WebSocketValidator) updateMessageStats(clientID, messageType string, messageSize int) {
	wsv.messageStatsMutex.Lock()
	defer wsv.messageStatsMutex.Unlock()
	
	key := fmt.Sprintf("%s:%s", clientID, messageType)
	stats, exists := wsv.messageStats[key]
	if !exists {
		stats = &MessageStats{
			ClientID:    clientID,
			MessageType: messageType,
		}
		wsv.messageStats[key] = stats
	}
	
	stats.Count++
	stats.TotalBytes += int64(messageSize)
	stats.LastMessageTime = time.Now()
}

// removeConnection removes a connection from tracking
func (wsv *WebSocketValidator) removeConnection(clientID string) {
	wsv.connectionsMutex.Lock()
	defer wsv.connectionsMutex.Unlock()
	
	if conn, exists := wsv.connections[clientID]; exists {
		// Cancel connection context
		if conn.CancelFunc != nil {
			conn.CancelFunc()
		}
		
		// Remove from rate limiter
		wsv.rateLimiter.RemoveConnection(clientID)
		
		// Remove from connections map
		delete(wsv.connections, clientID)
		
		log.Printf("WebSocket connection removed: %s", clientID)
	}
}

// GetConnectionStats returns statistics for a connection
func (wsv *WebSocketValidator) GetConnectionStats(clientID string) (*ValidatedConnection, bool) {
	wsv.connectionsMutex.RLock()
	defer wsv.connectionsMutex.RUnlock()
	
	conn, exists := wsv.connections[clientID]
	return conn, exists
}

// GetAllConnectionStats returns statistics for all connections
func (wsv *WebSocketValidator) GetAllConnectionStats() map[string]*ValidatedConnection {
	wsv.connectionsMutex.RLock()
	defer wsv.connectionsMutex.RUnlock()
	
	stats := make(map[string]*ValidatedConnection)
	for id, conn := range wsv.connections {
		stats[id] = conn
	}
	
	return stats
}

// cleanupLoop periodically cleans up expired connections and stats
func (wsv *WebSocketValidator) cleanupLoop() {
	ticker := time.NewTicker(wsv.config.CleanupInterval)
	defer ticker.Stop()
	
	for range ticker.C {
		wsv.cleanup()
	}
}

// cleanup removes expired connections and stats
func (wsv *WebSocketValidator) cleanup() {
	now := time.Now()
	
	// Clean up connections
	wsv.connectionsMutex.Lock()
	for clientID, conn := range wsv.connections {
		// Remove inactive connections
		if now.Sub(conn.LastActivity) > wsv.config.ConnectionTimeout {
			log.Printf("Removing inactive connection: %s", clientID)
			if conn.CancelFunc != nil {
				conn.CancelFunc()
			}
			delete(wsv.connections, clientID)
		}
	}
	wsv.connectionsMutex.Unlock()
	
	// Clean up message stats
	wsv.messageStatsMutex.Lock()
	for key, stats := range wsv.messageStats {
		if now.Sub(stats.LastMessageTime) > wsv.config.StatsRetentionPeriod {
			delete(wsv.messageStats, key)
		}
	}
	wsv.messageStatsMutex.Unlock()
}

// SendValidationError sends a validation error to the client
func (wsv *WebSocketValidator) SendValidationError(conn *websocket.Conn, err error) {
	if !wsv.config.SendErrorMessages {
		return
	}
	
	wsError := WebSocketError{
		Code:      websocket.CloseUnsupportedData,
		Message:   err.Error(),
		Type:      "validation_error",
		Timestamp: time.Now(),
	}
	
	errorData, jsonErr := json.Marshal(wsError)
	if jsonErr != nil {
		log.Printf("Failed to marshal validation error: %v", jsonErr)
		return
	}
	
	if sendErr := conn.WriteMessage(websocket.TextMessage, errorData); sendErr != nil {
		log.Printf("Failed to send validation error: %v", sendErr)
	}
}

// generateClientID generates a unique client ID
func generateClientID() string {
	return fmt.Sprintf("ws-%d", time.Now().UnixNano())
}