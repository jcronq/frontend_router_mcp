package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/jcronq/frontend_router_mcp/internal/errors"
	"github.com/jcronq/frontend_router_mcp/internal/logger"
)

// AskUserTool implements a tool that allows an agent to ask a question to a user
// and receive their response.
type AskUserTool struct {
	// pendingRequests maps request IDs to channels that will receive user responses
	pendingRequests     map[string]chan string
	pendingRequestsLock sync.Mutex
	
	// questionHandler is called when a new question is received
	questionHandler func(requestID string, question string) error
	
	// logger for structured logging
	logger *logger.Logger
}

// NewAskUserTool creates a new AskUserTool with the given question handler
func NewAskUserTool(questionHandler func(requestID string, question string) error) *AskUserTool {
	return &AskUserTool{
		pendingRequests: make(map[string]chan string),
		questionHandler: questionHandler,
		logger:          logger.WithComponent("ask-user-tool"),
	}
}

// Name returns the name of the tool
func (t *AskUserTool) Name() string {
	return "ask_user"
}

// Description returns a description of the tool
func (t *AskUserTool) Description() string {
	return "Ask a question to the user and receive their response"
}

// GetSchema returns the JSON schema for the tool's parameters
func (t *AskUserTool) GetSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"question": map[string]interface{}{
				"type":        "string",
				"description": "The question to ask the user",
			},
			"timeout_seconds": map[string]interface{}{
				"type":        "number",
				"description": "Maximum time to wait for a response in seconds (default: 300)",
			},
		},
		"required": []string{"question"},
	}
}

// Execute executes the tool with the given parameters
func (t *AskUserTool) Execute(ctx context.Context, params json.RawMessage) (interface{}, error) {
	// Parse parameters
	var p struct {
		Question       string  `json:"question"`
		TimeoutSeconds float64 `json:"timeout_seconds"`
	}
	
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, errors.Wrap(err, errors.ErrorTypeValidation, "failed to parse parameters")
	}
	
	if p.Question == "" {
		return nil, errors.NewValidationError("question parameter is required")
	}
	
	// Set default timeout if not specified
	if p.TimeoutSeconds <= 0 {
		p.TimeoutSeconds = 300 // 5 minutes default timeout
	}
	
	// Generate a unique request ID
	requestID := generateRequestID()
	
	// Create a channel to receive the response
	responseCh := make(chan string, 1)
	
	// Register the pending request
	t.pendingRequestsLock.Lock()
	t.pendingRequests[requestID] = responseCh
	t.pendingRequestsLock.Unlock()
	
	// Clean up when done
	defer func() {
		t.pendingRequestsLock.Lock()
		delete(t.pendingRequests, requestID)
		t.pendingRequestsLock.Unlock()
		close(responseCh)
	}()
	
	// Forward the question to the handler
	if err := t.questionHandler(requestID, p.Question); err != nil {
		return nil, errors.Wrap(err, errors.ErrorTypeInternal, "failed to send question to user").
			WithRequestID(requestID).WithContext("question", p.Question)
	}
	
	t.logger.Info("Waiting for user response to question", "question", p.Question, "request_id", requestID)
	
	// Wait for the response or timeout
	select {
	case response := <-responseCh:
		t.logger.Info("Received user response", "request_id", requestID, "answer", response)
		return map[string]interface{}{
			"answer": response,
		}, nil
	case <-time.After(time.Duration(p.TimeoutSeconds) * time.Second):
		t.logger.Warn("Timeout waiting for user response", "request_id", requestID, "timeout_seconds", p.TimeoutSeconds)
		return nil, errors.NewTimeoutError("timeout waiting for user response").
			WithRequestID(requestID).WithContext("timeout_seconds", p.TimeoutSeconds)
	case <-ctx.Done():
		t.logger.Info("Context cancelled while waiting for user response", "request_id", requestID)
		return nil, errors.Wrap(ctx.Err(), errors.ErrorTypeInternal, "context cancelled").
			WithRequestID(requestID)
	}
}

// HandleUserResponse handles a response from a user
func (t *AskUserTool) HandleUserResponse(requestID string, answer string) error {
	t.pendingRequestsLock.Lock()
	responseCh, exists := t.pendingRequests[requestID]
	t.pendingRequestsLock.Unlock()
	
	if !exists {
		t.logger.Warn("No pending request found for user response", "request_id", requestID)
		return errors.NewNotFoundError("no pending request found with ID: " + requestID).
			WithRequestID(requestID)
	}
	
	// Send the response
	select {
	case responseCh <- answer:
		t.logger.Info("Successfully handled user response", "request_id", requestID)
		return nil
	default:
		t.logger.Error("Failed to send response, channel full or closed", "request_id", requestID)
		return errors.NewInternalError("failed to send response, channel full or closed").
			WithRequestID(requestID)
	}
}

// generateRequestID generates a unique request ID
func generateRequestID() string {
	return fmt.Sprintf("req-%d-%d", time.Now().UnixNano(), time.Now().UnixNano()%1000)
}
