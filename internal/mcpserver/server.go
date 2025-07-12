package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/jcronq/frontend_router_mcp/pkg/messaging"
)

// Tool represents an MCP tool that can be executed
type Tool interface {
	Name() string
	Description() string
	Execute(ctx context.Context, params json.RawMessage) (interface{}, error)
}

// Server handles MCP tool execution requests
type Server struct {
	tools     map[string]Tool
	messageCh chan *messaging.Message
}

// NewServer creates a new MCP server
func NewServer(messageCh chan *messaging.Message) *Server {
	return &Server{
		tools:     make(map[string]Tool),
		messageCh: messageCh,
	}
}

// RegisterTool registers a new tool with the server
func (s *Server) RegisterTool(tool Tool) {
	s.tools[tool.Name()] = tool
}

// Start starts the MCP server
func (s *Server) Start(addr string) error {
	http.HandleFunc("/execute", s.handleExecute)
	http.HandleFunc("/tools", s.handleListTools)

	// Start processing incoming messages
	go s.processMessages()

	log.Printf("MCP server starting on %s\n", addr)
	return http.ListenAndServe(addr, nil)
}

// processMessages processes incoming messages from the message channel
func (s *Server) processMessages() {
	for msg := range s.messageCh {
		go s.handleMessage(msg)
	}
}

// handleMessage processes a single incoming message
func (s *Server) handleMessage(msg *messaging.Message) {
	log.Printf("Processing message from client %s: %s\n", msg.ClientID, string(msg.Payload))

	// Parse the message payload
	var payload struct {
		Type    string          `json:"type"`
		Tool    string          `json:"tool"`
		Params  json.RawMessage `json:"params"`
	}

	// The Payload field already contains the raw JSON
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		log.Printf("Failed to unmarshal message payload: %v", err)
		s.sendErrorResponse(msg.ClientID, "invalid_payload", fmt.Sprintf("Invalid message format: %v", err))
		return
	}

	// Set default type if not provided
	if payload.Type == "" {
		payload.Type = "tool_request"
	}

	// Handle different message types
	switch payload.Type {
	case "tool_request":
		if payload.Tool == "" {
			s.sendErrorResponse(msg.ClientID, "missing_tool", "No tool specified in request")
			return
		}
		s.handleToolRequest(msg.ClientID, payload.Tool, payload.Params)
	default:
		log.Printf("Unknown message type: %s", payload.Type)
		s.sendErrorResponse(msg.ClientID, "unknown_message_type", 
			fmt.Sprintf("Unknown message type: %s", payload.Type))
	}
}

// handleToolRequest processes a tool execution request
func (s *Server) handleToolRequest(clientID, toolName string, params json.RawMessage) {
	tool, exists := s.tools[toolName]
	if !exists {
		log.Printf("Tool not found: %s", toolName)
		s.sendErrorResponse(clientID, "tool_not_found", fmt.Sprintf("Tool '%s' not found", toolName))
		return
	}

	// Execute the tool in a goroutine to avoid blocking
	go func() {
		result, err := tool.Execute(context.Background(), params)
		if err != nil {
			log.Printf("Tool execution error: %v", err)
			s.sendErrorResponse(clientID, "execution_error", err.Error())
			return
		}

		// Send the result back to the client
		s.sendResponse(clientID, result)
	}()
}

// sendResponse sends a response back to the client
func (s *Server) sendResponse(clientID string, data interface{}) {
	response := map[string]interface{}{
		"type": "tool_response",
		"data": data,
	}

	msg, err := messaging.NewMessage("tool_response", response)
	if err != nil {
		log.Printf("Failed to create response message: %v", err)
		return
	}

	// Set the client ID to ensure the response goes to the right client
	msg.ClientID = clientID

	// Send the message back through the channel
	s.messageCh <- msg
}

// sendErrorResponse sends an error response to the client
func (s *Server) sendErrorResponse(clientID, errorType, message string) {
	errResponse := map[string]interface{}{
		"type":    "error",
		"error":   errorType,
		"message": message,
	}

	msg, err := messaging.NewMessage("error", errResponse)
	if err != nil {
		log.Printf("Failed to create error message: %v", err)
		return
	}

	// Set the client ID
	msg.ClientID = clientID

	// Send the error message back through the channel
	s.messageCh <- msg
}

// handleExecute handles tool execution requests
func (s *Server) handleExecute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Tool    string          `json:"tool"`
		Params  json.RawMessage `json:"params"`
		ClientID string         `json:"client_id,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	tool, exists := s.tools[req.Tool]
	if !exists {
		http.Error(w, "Tool not found", http.StatusNotFound)
		return
	}

	result, err := tool.Execute(r.Context(), req.Params)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// handleListTools lists all available tools
func (s *Server) handleListTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tools := make([]map[string]string, 0, len(s.tools))
	for _, tool := range s.tools {
		tools = append(tools, map[string]string{
			"name":        tool.Name(),
			"description": tool.Description(),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tools)
}
