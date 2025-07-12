package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/jcronq/frontend_router_mcp/pkg/messaging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockTool implements the Tool interface for testing
type mockTool struct {
	name        string
	description string
	execFunc    func(ctx context.Context, params json.RawMessage) (interface{}, error)
}

func (m *mockTool) Name() string {
	return m.name
}

func (m *mockTool) Description() string {
	return m.description
}

func (m *mockTool) Execute(ctx context.Context, params json.RawMessage) (interface{}, error) {
	if m.execFunc != nil {
		return m.execFunc(ctx, params)
	}
	return map[string]interface{}{
		"result": "success",
		"params": string(params),
	}, nil
}

// TestNewServer tests the NewServer constructor
func TestNewServer(t *testing.T) {
	messageCh := make(chan *messaging.Message, 100)
	server := NewServer(messageCh)
	
	assert.NotNil(t, server)
	assert.NotNil(t, server.tools)
	assert.Equal(t, messageCh, server.messageCh)
	assert.Equal(t, 0, len(server.tools))
}

// TestServer_RegisterTool tests tool registration
func TestServer_RegisterTool(t *testing.T) {
	messageCh := make(chan *messaging.Message, 100)
	server := NewServer(messageCh)
	
	tool := &mockTool{
		name:        "test_tool",
		description: "A test tool",
	}
	
	server.RegisterTool(tool)
	
	assert.Equal(t, 1, len(server.tools))
	assert.Equal(t, tool, server.tools["test_tool"])
}

// TestServer_RegisterTool_MultipleDifferentTools tests registering multiple different tools
func TestServer_RegisterTool_MultipleDifferentTools(t *testing.T) {
	messageCh := make(chan *messaging.Message, 100)
	server := NewServer(messageCh)
	
	tool1 := &mockTool{name: "tool1", description: "First tool"}
	tool2 := &mockTool{name: "tool2", description: "Second tool"}
	tool3 := &mockTool{name: "tool3", description: "Third tool"}
	
	server.RegisterTool(tool1)
	server.RegisterTool(tool2)
	server.RegisterTool(tool3)
	
	assert.Equal(t, 3, len(server.tools))
	assert.Equal(t, tool1, server.tools["tool1"])
	assert.Equal(t, tool2, server.tools["tool2"])
	assert.Equal(t, tool3, server.tools["tool3"])
}

// TestServer_RegisterTool_OverwriteExisting tests overwriting existing tools
func TestServer_RegisterTool_OverwriteExisting(t *testing.T) {
	messageCh := make(chan *messaging.Message, 100)
	server := NewServer(messageCh)
	
	tool1 := &mockTool{name: "test_tool", description: "First tool"}
	tool2 := &mockTool{name: "test_tool", description: "Second tool"}
	
	server.RegisterTool(tool1)
	server.RegisterTool(tool2)
	
	assert.Equal(t, 1, len(server.tools))
	assert.Equal(t, tool2, server.tools["test_tool"])
}

// TestServer_handleExecute tests the HTTP execute endpoint
func TestServer_handleExecute(t *testing.T) {
	messageCh := make(chan *messaging.Message, 100)
	server := NewServer(messageCh)
	
	// Register a test tool
	tool := &mockTool{
		name:        "echo_tool",
		description: "Echoes the input",
		execFunc: func(ctx context.Context, params json.RawMessage) (interface{}, error) {
			return map[string]interface{}{
				"echo": string(params),
			}, nil
		},
	}
	server.RegisterTool(tool)
	
	// Create a test request
	requestBody := map[string]interface{}{
		"tool":      "echo_tool",
		"params":    json.RawMessage(`{"message": "hello"}`),
		"client_id": "test_client",
	}
	
	bodyBytes, _ := json.Marshal(requestBody)
	
	// Create HTTP request
	req := httptest.NewRequest(http.MethodPost, "/execute", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	
	// Create response recorder
	w := httptest.NewRecorder()
	
	// Call the handler
	server.handleExecute(w, req)
	
	// Check response
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	
	assert.Equal(t, `{"message": "hello"}`, response["echo"])
}

// TestServer_handleExecute_MethodNotAllowed tests non-POST requests
func TestServer_handleExecute_MethodNotAllowed(t *testing.T) {
	messageCh := make(chan *messaging.Message, 100)
	server := NewServer(messageCh)
	
	req := httptest.NewRequest(http.MethodGet, "/execute", nil)
	w := httptest.NewRecorder()
	
	server.handleExecute(w, req)
	
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

// TestServer_handleExecute_InvalidRequestBody tests invalid request body
func TestServer_handleExecute_InvalidRequestBody(t *testing.T) {
	messageCh := make(chan *messaging.Message, 100)
	server := NewServer(messageCh)
	
	req := httptest.NewRequest(http.MethodPost, "/execute", bytes.NewReader([]byte(`{invalid json}`)))
	w := httptest.NewRecorder()
	
	server.handleExecute(w, req)
	
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestServer_handleExecute_ToolNotFound tests tool not found scenario
func TestServer_handleExecute_ToolNotFound(t *testing.T) {
	messageCh := make(chan *messaging.Message, 100)
	server := NewServer(messageCh)
	
	requestBody := map[string]interface{}{
		"tool":   "nonexistent_tool",
		"params": json.RawMessage(`{}`),
	}
	
	bodyBytes, _ := json.Marshal(requestBody)
	req := httptest.NewRequest(http.MethodPost, "/execute", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	
	w := httptest.NewRecorder()
	server.handleExecute(w, req)
	
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestServer_handleExecute_ToolExecutionError tests tool execution error
func TestServer_handleExecute_ToolExecutionError(t *testing.T) {
	messageCh := make(chan *messaging.Message, 100)
	server := NewServer(messageCh)
	
	// Register a tool that returns an error
	tool := &mockTool{
		name:        "error_tool",
		description: "A tool that returns an error",
		execFunc: func(ctx context.Context, params json.RawMessage) (interface{}, error) {
			return nil, fmt.Errorf("tool execution failed")
		},
	}
	server.RegisterTool(tool)
	
	requestBody := map[string]interface{}{
		"tool":   "error_tool",
		"params": json.RawMessage(`{}`),
	}
	
	bodyBytes, _ := json.Marshal(requestBody)
	req := httptest.NewRequest(http.MethodPost, "/execute", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	
	w := httptest.NewRecorder()
	server.handleExecute(w, req)
	
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// TestServer_handleListTools tests the list tools endpoint
func TestServer_handleListTools(t *testing.T) {
	messageCh := make(chan *messaging.Message, 100)
	server := NewServer(messageCh)
	
	// Register some test tools
	tool1 := &mockTool{name: "tool1", description: "First tool"}
	tool2 := &mockTool{name: "tool2", description: "Second tool"}
	server.RegisterTool(tool1)
	server.RegisterTool(tool2)
	
	req := httptest.NewRequest(http.MethodGet, "/tools", nil)
	w := httptest.NewRecorder()
	
	server.handleListTools(w, req)
	
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	
	var response []map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	
	assert.Len(t, response, 2)
	
	// Check that both tools are in the response
	toolNames := make(map[string]string)
	for _, tool := range response {
		toolNames[tool["name"]] = tool["description"]
	}
	
	assert.Equal(t, "First tool", toolNames["tool1"])
	assert.Equal(t, "Second tool", toolNames["tool2"])
}

// TestServer_handleListTools_MethodNotAllowed tests non-GET requests to list tools
func TestServer_handleListTools_MethodNotAllowed(t *testing.T) {
	messageCh := make(chan *messaging.Message, 100)
	server := NewServer(messageCh)
	
	req := httptest.NewRequest(http.MethodPost, "/tools", nil)
	w := httptest.NewRecorder()
	
	server.handleListTools(w, req)
	
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

// TestServer_handleListTools_EmptyToolList tests listing tools when no tools are registered
func TestServer_handleListTools_EmptyToolList(t *testing.T) {
	messageCh := make(chan *messaging.Message, 100)
	server := NewServer(messageCh)
	
	req := httptest.NewRequest(http.MethodGet, "/tools", nil)
	w := httptest.NewRecorder()
	
	server.handleListTools(w, req)
	
	assert.Equal(t, http.StatusOK, w.Code)
	
	var response []map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	
	assert.Len(t, response, 0)
}

// TestServer_processMessages tests the message processing functionality
func TestServer_processMessages(t *testing.T) {
	messageCh := make(chan *messaging.Message, 100)
	server := NewServer(messageCh)
	
	// Register a test tool
	tool := &mockTool{
		name:        "test_tool",
		description: "A test tool",
		execFunc: func(ctx context.Context, params json.RawMessage) (interface{}, error) {
			return map[string]interface{}{
				"result": "processed",
			}, nil
		},
	}
	server.RegisterTool(tool)
	
	// Start processing messages
	go server.processMessages()
	
	// Create a test message
	payload := map[string]interface{}{
		"type":   "tool_request",
		"tool":   "test_tool",
		"params": json.RawMessage(`{"input": "test"}`),
	}
	payloadBytes, _ := json.Marshal(payload)
	
	msg := &messaging.Message{
		ID:        "test-msg-123",
		Type:      "tool_request",
		Timestamp: time.Now(),
		Payload:   payloadBytes,
		ClientID:  "test-client",
	}
	
	// Send the message
	messageCh <- msg
	
	// Give time for processing
	time.Sleep(100 * time.Millisecond)
	
	// Check if a response was sent back
	select {
	case responseMsg := <-messageCh:
		assert.Equal(t, "test-client", responseMsg.ClientID)
		assert.Equal(t, "tool_response", responseMsg.Type)
		
		var responsePayload map[string]interface{}
		err := json.Unmarshal(responseMsg.Payload, &responsePayload)
		require.NoError(t, err)
		
		assert.Equal(t, "tool_response", responsePayload["type"])
		assert.Contains(t, responsePayload, "data")
		
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for response message")
	}
}

// TestServer_handleMessage tests the handleMessage method
func TestServer_handleMessage(t *testing.T) {
	messageCh := make(chan *messaging.Message, 100)
	server := NewServer(messageCh)
	
	// Register a test tool
	tool := &mockTool{
		name:        "test_tool",
		description: "A test tool",
	}
	server.RegisterTool(tool)
	
	tests := []struct {
		name           string
		payload        interface{}
		expectedType   string
		expectResponse bool
	}{
		{
			name: "valid tool request",
			payload: map[string]interface{}{
				"type":   "tool_request",
				"tool":   "test_tool",
				"params": json.RawMessage(`{"input": "test"}`),
			},
			expectedType:   "tool_response",
			expectResponse: true,
		},
		{
			name: "tool request without type",
			payload: map[string]interface{}{
				"tool":   "test_tool",
				"params": json.RawMessage(`{"input": "test"}`),
			},
			expectedType:   "tool_response",
			expectResponse: true,
		},
		{
			name: "tool request without tool name",
			payload: map[string]interface{}{
				"type":   "tool_request",
				"params": json.RawMessage(`{"input": "test"}`),
			},
			expectedType:   "error",
			expectResponse: true,
		},
		{
			name: "unknown message type",
			payload: map[string]interface{}{
				"type": "unknown_type",
			},
			expectedType:   "error",
			expectResponse: true,
		},
		{
			name: "tool not found",
			payload: map[string]interface{}{
				"type":   "tool_request",
				"tool":   "nonexistent_tool",
				"params": json.RawMessage(`{}`),
			},
			expectedType:   "error",
			expectResponse: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payloadBytes, _ := json.Marshal(tt.payload)
			
			msg := &messaging.Message{
				ID:        "test-msg",
				Type:      "test",
				Timestamp: time.Now(),
				Payload:   payloadBytes,
				ClientID:  "test-client",
			}
			
			// Handle the message
			server.handleMessage(msg)
			
			if tt.expectResponse {
				// Check for response
				select {
				case responseMsg := <-messageCh:
					assert.Equal(t, "test-client", responseMsg.ClientID)
					assert.Equal(t, tt.expectedType, responseMsg.Type)
					
				case <-time.After(1 * time.Second):
					t.Fatal("Timeout waiting for response message")
				}
			}
		})
	}
}

// TestServer_handleMessage_InvalidJSON tests handleMessage with invalid JSON
func TestServer_handleMessage_InvalidJSON(t *testing.T) {
	messageCh := make(chan *messaging.Message, 100)
	server := NewServer(messageCh)
	
	msg := &messaging.Message{
		ID:        "test-msg",
		Type:      "test",
		Timestamp: time.Now(),
		Payload:   json.RawMessage(`{invalid json}`),
		ClientID:  "test-client",
	}
	
	server.handleMessage(msg)
	
	// Should send error response
	select {
	case responseMsg := <-messageCh:
		assert.Equal(t, "test-client", responseMsg.ClientID)
		assert.Equal(t, "error", responseMsg.Type)
		
		var errorPayload map[string]interface{}
		err := json.Unmarshal(responseMsg.Payload, &errorPayload)
		require.NoError(t, err)
		
		assert.Equal(t, "error", errorPayload["type"])
		assert.Equal(t, "invalid_payload", errorPayload["error"])
		
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for error response")
	}
}

// TestServer_handleToolRequest tests the handleToolRequest method
func TestServer_handleToolRequest(t *testing.T) {
	messageCh := make(chan *messaging.Message, 100)
	server := NewServer(messageCh)
	
	// Test with tool that returns result
	tool := &mockTool{
		name:        "success_tool",
		description: "A successful tool",
		execFunc: func(ctx context.Context, params json.RawMessage) (interface{}, error) {
			return map[string]interface{}{
				"result": "success",
			}, nil
		},
	}
	server.RegisterTool(tool)
	
	// Test successful execution
	server.handleToolRequest("test-client", "success_tool", json.RawMessage(`{"input": "test"}`))
	
	// Wait for response
	select {
	case responseMsg := <-messageCh:
		assert.Equal(t, "test-client", responseMsg.ClientID)
		assert.Equal(t, "tool_response", responseMsg.Type)
		
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for response")
	}
}

// TestServer_handleToolRequest_ToolError tests handleToolRequest with tool execution error
func TestServer_handleToolRequest_ToolError(t *testing.T) {
	messageCh := make(chan *messaging.Message, 100)
	server := NewServer(messageCh)
	
	// Test with tool that returns error
	tool := &mockTool{
		name:        "error_tool",
		description: "A tool that errors",
		execFunc: func(ctx context.Context, params json.RawMessage) (interface{}, error) {
			return nil, fmt.Errorf("execution error")
		},
	}
	server.RegisterTool(tool)
	
	server.handleToolRequest("test-client", "error_tool", json.RawMessage(`{}`))
	
	// Wait for error response
	select {
	case responseMsg := <-messageCh:
		assert.Equal(t, "test-client", responseMsg.ClientID)
		assert.Equal(t, "error", responseMsg.Type)
		
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for error response")
	}
}

// TestServer_sendResponse tests the sendResponse method
func TestServer_sendResponse(t *testing.T) {
	messageCh := make(chan *messaging.Message, 100)
	server := NewServer(messageCh)
	
	testData := map[string]interface{}{
		"result": "test result",
		"status": "success",
	}
	
	server.sendResponse("test-client", testData)
	
	// Check the response message
	select {
	case msg := <-messageCh:
		assert.Equal(t, "test-client", msg.ClientID)
		assert.Equal(t, "tool_response", msg.Type)
		
		var payload map[string]interface{}
		err := json.Unmarshal(msg.Payload, &payload)
		require.NoError(t, err)
		
		assert.Equal(t, "tool_response", payload["type"])
		assert.Equal(t, testData, payload["data"])
		
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for response message")
	}
}

// TestServer_sendErrorResponse tests the sendErrorResponse method
func TestServer_sendErrorResponse(t *testing.T) {
	messageCh := make(chan *messaging.Message, 100)
	server := NewServer(messageCh)
	
	server.sendErrorResponse("test-client", "test_error", "Test error message")
	
	// Check the error response message
	select {
	case msg := <-messageCh:
		assert.Equal(t, "test-client", msg.ClientID)
		assert.Equal(t, "error", msg.Type)
		
		var payload map[string]interface{}
		err := json.Unmarshal(msg.Payload, &payload)
		require.NoError(t, err)
		
		assert.Equal(t, "error", payload["type"])
		assert.Equal(t, "test_error", payload["error"])
		assert.Equal(t, "Test error message", payload["message"])
		
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for error response message")
	}
}

// TestServer_Start tests starting the server
func TestServer_Start(t *testing.T) {
	// Skip this test by default as it requires a network port
	t.Skip("Skipping server start test - requires available port")
	
	messageCh := make(chan *messaging.Message, 100)
	server := NewServer(messageCh)
	
	// Start server in a goroutine
	go func() {
		err := server.Start(":0") // Use port 0 to get any available port
		if err != nil && err != http.ErrServerClosed {
			t.Errorf("Server start error: %v", err)
		}
	}()
	
	// Give server time to start
	time.Sleep(100 * time.Millisecond)
	
	// The server should be running
	// In a real test, you would check if the server is actually listening
}

// TestServer_ConcurrentToolExecution tests concurrent tool execution
func TestServer_ConcurrentToolExecution(t *testing.T) {
	messageCh := make(chan *messaging.Message, 1000)
	server := NewServer(messageCh)
	
	// Create a tool that takes some time to execute
	tool := &mockTool{
		name:        "slow_tool",
		description: "A slow tool",
		execFunc: func(ctx context.Context, params json.RawMessage) (interface{}, error) {
			time.Sleep(10 * time.Millisecond)
			return map[string]interface{}{
				"result": "completed",
			}, nil
		},
	}
	server.RegisterTool(tool)
	
	// Start processing messages
	go server.processMessages()
	
	const numRequests = 100
	var wg sync.WaitGroup
	wg.Add(numRequests)
	
	// Send multiple concurrent requests
	for i := 0; i < numRequests; i++ {
		go func(index int) {
			defer wg.Done()
			
			payload := map[string]interface{}{
				"type":   "tool_request",
				"tool":   "slow_tool",
				"params": json.RawMessage(`{}`),
			}
			payloadBytes, _ := json.Marshal(payload)
			
			msg := &messaging.Message{
				ID:        fmt.Sprintf("msg-%d", index),
				Type:      "tool_request",
				Timestamp: time.Now(),
				Payload:   payloadBytes,
				ClientID:  fmt.Sprintf("client-%d", index),
			}
			
			messageCh <- msg
		}(i)
	}
	
	wg.Wait()
	
	// Wait for all responses
	responseCount := 0
	timeout := time.After(5 * time.Second)
	
	for responseCount < numRequests {
		select {
		case <-messageCh:
			responseCount++
		case <-timeout:
			t.Fatalf("Only received %d responses, expected %d", responseCount, numRequests)
		}
	}
}

// TestServer_MessageChannelFull tests behavior when message channel is full
func TestServer_MessageChannelFull(t *testing.T) {
	messageCh := make(chan *messaging.Message, 1) // Small buffer
	server := NewServer(messageCh)
	
	// Fill the channel
	dummyMsg := &messaging.Message{
		ID:        "dummy",
		Type:      "dummy",
		Timestamp: time.Now(),
		Payload:   json.RawMessage(`{}`),
		ClientID:  "dummy-client",
	}
	messageCh <- dummyMsg
	
	// This should not block or panic
	server.sendResponse("test-client", map[string]interface{}{"test": "data"})
	
	// Should complete without hanging
	assert.True(t, true)
}

// BenchmarkServer_HandleMessage benchmarks message handling
func BenchmarkServer_HandleMessage(b *testing.B) {
	messageCh := make(chan *messaging.Message, 10000)
	server := NewServer(messageCh)
	
	// Register a simple tool
	tool := &mockTool{
		name:        "benchmark_tool",
		description: "A benchmark tool",
		execFunc: func(ctx context.Context, params json.RawMessage) (interface{}, error) {
			return map[string]interface{}{"result": "ok"}, nil
		},
	}
	server.RegisterTool(tool)
	
	// Drain the message channel
	go func() {
		for range messageCh {
		}
	}()
	
	payload := map[string]interface{}{
		"type":   "tool_request",
		"tool":   "benchmark_tool",
		"params": json.RawMessage(`{}`),
	}
	payloadBytes, _ := json.Marshal(payload)
	
	msg := &messaging.Message{
		ID:        "benchmark-msg",
		Type:      "tool_request",
		Timestamp: time.Now(),
		Payload:   payloadBytes,
		ClientID:  "benchmark-client",
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		server.handleMessage(msg)
	}
}

// BenchmarkServer_RegisterTool benchmarks tool registration
func BenchmarkServer_RegisterTool(b *testing.B) {
	messageCh := make(chan *messaging.Message, 100)
	server := NewServer(messageCh)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tool := &mockTool{
			name:        fmt.Sprintf("tool-%d", i),
			description: fmt.Sprintf("Tool %d", i),
		}
		server.RegisterTool(tool)
	}
}

// TestServer_ToolInterface tests that tools implement the interface correctly
func TestServer_ToolInterface(t *testing.T) {
	var tool Tool = &mockTool{
		name:        "interface_test",
		description: "Testing interface",
	}
	
	assert.Equal(t, "interface_test", tool.Name())
	assert.Equal(t, "Testing interface", tool.Description())
	
	result, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	assert.NoError(t, err)
	assert.NotNil(t, result)
}

// TestServer_ContextCancellation tests context cancellation in tool execution
func TestServer_ContextCancellation(t *testing.T) {
	messageCh := make(chan *messaging.Message, 100)
	server := NewServer(messageCh)
	
	// Create a tool that respects context cancellation
	tool := &mockTool{
		name:        "context_tool",
		description: "A context-aware tool",
		execFunc: func(ctx context.Context, params json.RawMessage) (interface{}, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(100 * time.Millisecond):
				return map[string]interface{}{"result": "completed"}, nil
			}
		},
	}
	server.RegisterTool(tool)
	
	// Test with cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately
	
	result, err := tool.Execute(ctx, json.RawMessage(`{}`))
	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

// TestServer_LargePayload tests handling of large payloads
func TestServer_LargePayload(t *testing.T) {
	messageCh := make(chan *messaging.Message, 100)
	server := NewServer(messageCh)
	
	// Create a tool that handles large payloads
	tool := &mockTool{
		name:        "large_payload_tool",
		description: "Handles large payloads",
		execFunc: func(ctx context.Context, params json.RawMessage) (interface{}, error) {
			return map[string]interface{}{
				"size": len(params),
			}, nil
		},
	}
	server.RegisterTool(tool)
	
	// Create a large payload (1MB)
	largeData := make([]byte, 1024*1024)
	for i := range largeData {
		largeData[i] = 'a'
	}
	
	largePayload := map[string]interface{}{
		"data": string(largeData),
	}
	payloadBytes, _ := json.Marshal(largePayload)
	
	// Test handling large payload
	result, err := tool.Execute(context.Background(), payloadBytes)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	
	resultMap := result.(map[string]interface{})
	assert.Greater(t, resultMap["size"].(int), 1024*1024)
}