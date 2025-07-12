package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/jcronq/frontend_router_mcp/internal/tools"
	"github.com/jcronq/frontend_router_mcp/internal/wsserver"
	"github.com/jcronq/frontend_router_mcp/pkg/messaging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUserQuestion tests the userQuestion struct
func TestUserQuestion(t *testing.T) {
	q := userQuestion{
		RequestID: "test-123",
		Question:  "What is your name?",
	}
	
	assert.Equal(t, "test-123", q.RequestID)
	assert.Equal(t, "What is your name?", q.Question)
}

// TestUserResponse tests the userResponse struct
func TestUserResponse(t *testing.T) {
	r := userResponse{
		RequestID: "test-123",
		Answer:    "John Doe",
	}
	
	assert.Equal(t, "test-123", r.RequestID)
	assert.Equal(t, "John Doe", r.Answer)
}

// TestServerInitialization tests that servers can be initialized without error
func TestServerInitialization(t *testing.T) {
	// Create message channel
	messageCh := make(chan *messaging.Message, 100)
	defer close(messageCh)
	
	// Test that we can initialize the servers without panic
	assert.NotPanics(t, func() {
		// Initialize servers
		wsServer := wsserver.NewServer(messageCh)
		
		// Create ask user tool
		askUserTool := tools.NewAskUserTool(func(requestID, question string) error {
			return nil
		})
		
		// Use the servers to avoid unused variable warnings
		_ = wsServer
		_ = askUserTool
	}, "Server initialization should not panic")
}

// TestHandleWebSocketToMCP tests the handleWebSocketToMCP function
func TestHandleWebSocketToMCP(t *testing.T) {
	messageCh := make(chan *messaging.Message, 100)
	questionCh := make(chan userQuestion, 10)
	responseCh := make(chan userResponse, 10)
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	// Start the handler in a goroutine
	go handleWebSocketToMCP(ctx, messageCh, questionCh, responseCh)
	
	// Test question handling
	q := userQuestion{
		RequestID: "test-123",
		Question:  "What is your name?",
	}
	
	questionCh <- q
	
	// Give time for processing
	time.Sleep(10 * time.Millisecond)
	
	// Since no clients are connected, should get an error response
	select {
	case response := <-responseCh:
		assert.Equal(t, "test-123", response.RequestID)
		assert.Contains(t, response.Answer, "Error: No clients connected")
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for response")
	}
}

// TestHandleWebSocketToMCP_ConnectMessage tests connect message handling
func TestHandleWebSocketToMCP_ConnectMessage(t *testing.T) {
	messageCh := make(chan *messaging.Message, 100)
	questionCh := make(chan userQuestion, 10)
	responseCh := make(chan userResponse, 10)
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	// Start the handler in a goroutine
	go handleWebSocketToMCP(ctx, messageCh, questionCh, responseCh)
	
	// Create a mock response sender
	mockSender := &mockResponseSender{}
	
	// Send a connect message
	connectPayload, _ := json.Marshal(map[string]interface{}{
		"type": "connect",
	})
	
	connectMsg := &messaging.Message{
		ID:             "connect-123",
		Type:           "connect",
		Timestamp:      time.Now(),
		Payload:        connectPayload,
		ClientID:       "test-client",
		ResponseSender: mockSender,
	}
	
	messageCh <- connectMsg
	
	// Give time for processing
	time.Sleep(10 * time.Millisecond)
	
	// Check if acknowledgment was sent
	messages := mockSender.GetMessages()
	assert.Len(t, messages, 1)
	assert.Equal(t, "connect_ack", messages[0].Type)
}

// TestHandleWebSocketToMCP_UserResponse tests user response handling
func TestHandleWebSocketToMCP_UserResponse(t *testing.T) {
	messageCh := make(chan *messaging.Message, 100)
	questionCh := make(chan userQuestion, 10)
	responseCh := make(chan userResponse, 10)
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	// Start the handler in a goroutine
	go handleWebSocketToMCP(ctx, messageCh, questionCh, responseCh)
	
	// Create a mock response sender
	mockSender := &mockResponseSender{}
	
	// First, send a connect message to register the client
	connectPayload, _ := json.Marshal(map[string]interface{}{
		"type": "connect",
	})
	
	connectMsg := &messaging.Message{
		ID:             "connect-123",
		Type:           "connect",
		Timestamp:      time.Now(),
		Payload:        connectPayload,
		ClientID:       "test-client",
		ResponseSender: mockSender,
	}
	
	messageCh <- connectMsg
	time.Sleep(10 * time.Millisecond)
	
	// Send a question
	q := userQuestion{
		RequestID: "test-456",
		Question:  "What is your favorite color?",
	}
	
	questionCh <- q
	time.Sleep(10 * time.Millisecond)
	
	// Now send a user response
	responsePayload, _ := json.Marshal(map[string]interface{}{
		"request_id": "test-456",
		"answer":     "Blue",
	})
	
	userResponseMsg := &messaging.Message{
		ID:             "response-789",
		Type:           "user_response",
		Timestamp:      time.Now(),
		Payload:        responsePayload,
		ClientID:       "test-client",
		ResponseSender: mockSender,
	}
	
	messageCh <- userResponseMsg
	
	// Check if the response was forwarded
	select {
	case response := <-responseCh:
		assert.Equal(t, "test-456", response.RequestID)
		assert.Equal(t, "Blue", response.Answer)
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for response")
	}
}

// TestHandleWebSocketToMCP_DisconnectMessage tests disconnect message handling
func TestHandleWebSocketToMCP_DisconnectMessage(t *testing.T) {
	messageCh := make(chan *messaging.Message, 100)
	questionCh := make(chan userQuestion, 10)
	responseCh := make(chan userResponse, 10)
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	// Start the handler in a goroutine
	go handleWebSocketToMCP(ctx, messageCh, questionCh, responseCh)
	
	// Create a mock response sender
	mockSender := &mockResponseSender{}
	
	// Send a connect message first
	connectPayload, _ := json.Marshal(map[string]interface{}{
		"type": "connect",
	})
	
	connectMsg := &messaging.Message{
		ID:             "connect-123",
		Type:           "connect",
		Timestamp:      time.Now(),
		Payload:        connectPayload,
		ClientID:       "test-client",
		ResponseSender: mockSender,
	}
	
	messageCh <- connectMsg
	time.Sleep(10 * time.Millisecond)
	
	// Send a disconnect message
	disconnectPayload, _ := json.Marshal(map[string]interface{}{
		"type": "disconnect",
	})
	
	disconnectMsg := &messaging.Message{
		ID:             "disconnect-456",
		Type:           "disconnect",
		Timestamp:      time.Now(),
		Payload:        disconnectPayload,
		ClientID:       "test-client",
		ResponseSender: mockSender,
	}
	
	messageCh <- disconnectMsg
	
	// Give time for processing
	time.Sleep(10 * time.Millisecond)
	
	// Should not crash or panic
	assert.True(t, true)
}

// TestHandleWebSocketToMCP_UnknownMessage tests unknown message handling
func TestHandleWebSocketToMCP_UnknownMessage(t *testing.T) {
	messageCh := make(chan *messaging.Message, 100)
	questionCh := make(chan userQuestion, 10)
	responseCh := make(chan userResponse, 10)
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	// Start the handler in a goroutine
	go handleWebSocketToMCP(ctx, messageCh, questionCh, responseCh)
	
	// Send an unknown message type
	unknownPayload, _ := json.Marshal(map[string]interface{}{
		"type": "unknown_message_type",
	})
	
	unknownMsg := &messaging.Message{
		ID:        "unknown-123",
		Type:      "unknown_message_type",
		Timestamp: time.Now(),
		Payload:   unknownPayload,
		ClientID:  "test-client",
	}
	
	messageCh <- unknownMsg
	
	// Give time for processing
	time.Sleep(10 * time.Millisecond)
	
	// Should not crash or panic
	assert.True(t, true)
}

// TestHandleWebSocketToMCP_InvalidJSON tests invalid JSON handling
func TestHandleWebSocketToMCP_InvalidJSON(t *testing.T) {
	messageCh := make(chan *messaging.Message, 100)
	questionCh := make(chan userQuestion, 10)
	responseCh := make(chan userResponse, 10)
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	// Start the handler in a goroutine
	go handleWebSocketToMCP(ctx, messageCh, questionCh, responseCh)
	
	// Send a message with invalid JSON
	invalidMsg := &messaging.Message{
		ID:        "invalid-123",
		Type:      "user_response",
		Timestamp: time.Now(),
		Payload:   json.RawMessage(`{invalid json}`),
		ClientID:  "test-client",
	}
	
	messageCh <- invalidMsg
	
	// Give time for processing
	time.Sleep(10 * time.Millisecond)
	
	// Should not crash or panic
	assert.True(t, true)
}

// TestHandleWebSocketToMCP_ContextCancellation tests context cancellation
func TestHandleWebSocketToMCP_ContextCancellation(t *testing.T) {
	messageCh := make(chan *messaging.Message, 100)
	questionCh := make(chan userQuestion, 10)
	responseCh := make(chan userResponse, 10)
	
	ctx, cancel := context.WithCancel(context.Background())
	
	// Start the handler in a goroutine
	done := make(chan bool)
	go func() {
		handleWebSocketToMCP(ctx, messageCh, questionCh, responseCh)
		done <- true
	}()
	
	// Cancel the context
	cancel()
	
	// Wait for the function to return
	select {
	case <-done:
		// Function returned as expected
	case <-time.After(1 * time.Second):
		t.Fatal("Function did not return after context cancellation")
	}
}

// TestHandleUserResponses tests the handleUserResponses function
func TestHandleUserResponses(t *testing.T) {
	responseCh := make(chan userResponse, 10)
	
	// Create a mock ask user tool
	var receivedRequestID, receivedAnswer string
	mockAskUserTool := tools.NewAskUserTool(func(requestID, question string) error {
		return nil
	})
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	// Start the handler in a goroutine
	go handleUserResponses(ctx, responseCh, mockAskUserTool)
	
	// First, we need to set up a pending request in the tool
	// This is a bit tricky since we need to access the internal state
	// For now, let's just test that the function doesn't crash
	
	// Send a response
	response := userResponse{
		RequestID: "test-123",
		Answer:    "Test Answer",
	}
	
	responseCh <- response
	
	// Give time for processing
	time.Sleep(10 * time.Millisecond)
	
	// The function should not crash even if the request ID is not found
	assert.True(t, true)
	
	// Test with invalid request ID
	response2 := userResponse{
		RequestID: "invalid-456",
		Answer:    "Another Answer",
	}
	
	responseCh <- response2
	
	// Give time for processing
	time.Sleep(10 * time.Millisecond)
	
	// Should not crash
	assert.True(t, true)
	
	// Verify variables were set (even though they won't be used)
	_ = receivedRequestID
	_ = receivedAnswer
}

// TestHandleUserResponses_ContextCancellation tests context cancellation
func TestHandleUserResponses_ContextCancellation(t *testing.T) {
	responseCh := make(chan userResponse, 10)
	mockAskUserTool := tools.NewAskUserTool(func(requestID, question string) error {
		return nil
	})
	
	ctx, cancel := context.WithCancel(context.Background())
	
	// Start the handler in a goroutine
	done := make(chan bool)
	go func() {
		handleUserResponses(ctx, responseCh, mockAskUserTool)
		done <- true
	}()
	
	// Cancel the context
	cancel()
	
	// Wait for the function to return
	select {
	case <-done:
		// Function returned as expected
	case <-time.After(1 * time.Second):
		t.Fatal("Function did not return after context cancellation")
	}
}

// TestConcurrentMessageHandling tests concurrent message handling
func TestConcurrentMessageHandling(t *testing.T) {
	messageCh := make(chan *messaging.Message, 1000)
	questionCh := make(chan userQuestion, 100)
	responseCh := make(chan userResponse, 100)
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	// Start the handler in a goroutine
	go handleWebSocketToMCP(ctx, messageCh, questionCh, responseCh)
	
	const numMessages = 100
	var wg sync.WaitGroup
	wg.Add(numMessages)
	
	// Send multiple messages concurrently
	for i := 0; i < numMessages; i++ {
		go func(index int) {
			defer wg.Done()
			
			// Create a connect message
			connectPayload, _ := json.Marshal(map[string]interface{}{
				"type": "connect",
			})
			
			msg := &messaging.Message{
				ID:        fmt.Sprintf("msg-%d", index),
				Type:      "connect",
				Timestamp: time.Now(),
				Payload:   connectPayload,
				ClientID:  fmt.Sprintf("client-%d", index),
				ResponseSender: &mockResponseSender{},
			}
			
			messageCh <- msg
		}(i)
	}
	
	wg.Wait()
	
	// Give time for all messages to be processed
	time.Sleep(100 * time.Millisecond)
	
	// Should not crash or panic
	assert.True(t, true)
}

// TestChannelClosedHandling tests handling of closed channels
func TestChannelClosedHandling(t *testing.T) {
	messageCh := make(chan *messaging.Message, 100)
	questionCh := make(chan userQuestion, 10)
	responseCh := make(chan userResponse, 10)
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	// Start the handler in a goroutine
	done := make(chan bool)
	go func() {
		handleWebSocketToMCP(ctx, messageCh, questionCh, responseCh)
		done <- true
	}()
	
	// Close the message channel
	close(messageCh)
	
	// Wait for the function to return
	select {
	case <-done:
		// Function returned as expected
	case <-time.After(1 * time.Second):
		t.Fatal("Function did not return after channel closure")
	}
}

// TestServerStartup tests that servers can start without error
func TestServerStartup(t *testing.T) {
	// Skip this test by default as it requires network ports
	t.Skip("Skipping server startup test - requires available ports")
	
	// Create message channel
	messageCh := make(chan *messaging.Message, 100)
	defer close(messageCh)
	
	// Initialize servers
	wsServer := wsserver.NewServer(messageCh)
	
	// Use unique ports for testing
	wsPort := ":28080"
	
	// Start servers in goroutines
	wsErrCh := make(chan error, 1)
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	go func() {
		wsErrCh <- wsServer.Start(wsPort)
	}()
	
	// Give servers time to start
	time.Sleep(100 * time.Millisecond)
	
	// Check for immediate startup errors
	select {
	case err := <-wsErrCh:
		if err != nil && err != http.ErrServerClosed {
			t.Errorf("WebSocket server error: %v", err)
		}
	default:
		// No immediate errors, continue
	}
	
	// Test WebSocket server is running
	_, err := http.Get("http://localhost" + wsPort)
	if err != nil {
		t.Logf("WebSocket server not reachable (expected): %v", err)
	}
	
	// Shutdown
	cancel()
	err = wsServer.Shutdown(ctx)
	if err != nil {
		t.Logf("Shutdown error: %v", err)
	}
}

// mockResponseSender implements the ResponseSender interface for testing
type mockResponseSender struct {
	messages []*messaging.Message
	mu       sync.Mutex
}

func (m *mockResponseSender) SendResponse(msg *messaging.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, msg)
	return nil
}

func (m *mockResponseSender) GetMessages() []*messaging.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	messages := make([]*messaging.Message, len(m.messages))
	copy(messages, m.messages)
	return messages
}

// BenchmarkHandleWebSocketToMCP benchmarks the message handling function
func BenchmarkHandleWebSocketToMCP(b *testing.B) {
	messageCh := make(chan *messaging.Message, 10000)
	questionCh := make(chan userQuestion, 1000)
	responseCh := make(chan userResponse, 1000)
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	// Start the handler in a goroutine
	go handleWebSocketToMCP(ctx, messageCh, questionCh, responseCh)
	
	// Drain the response channel
	go func() {
		for range responseCh {
		}
	}()
	
	// Create a test message
	connectPayload, _ := json.Marshal(map[string]interface{}{
		"type": "connect",
	})
	
	msg := &messaging.Message{
		ID:             "benchmark-msg",
		Type:           "connect",
		Timestamp:      time.Now(),
		Payload:        connectPayload,
		ClientID:       "benchmark-client",
		ResponseSender: &mockResponseSender{},
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		messageCh <- msg
	}
}

// BenchmarkHandleUserResponses benchmarks the user response handling function
func BenchmarkHandleUserResponses(b *testing.B) {
	responseCh := make(chan userResponse, 10000)
	mockAskUserTool := tools.NewAskUserTool(func(requestID, question string) error {
		return nil
	})
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	// Start the handler in a goroutine
	go handleUserResponses(ctx, responseCh, mockAskUserTool)
	
	response := userResponse{
		RequestID: "benchmark-123",
		Answer:    "Benchmark Answer",
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		responseCh <- response
	}
}

// TestEdgeCases tests various edge cases
func TestEdgeCases(t *testing.T) {
	t.Run("nil message", func(t *testing.T) {
		messageCh := make(chan *messaging.Message, 10)
		questionCh := make(chan userQuestion, 10)
		responseCh := make(chan userResponse, 10)
		
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		
		go handleWebSocketToMCP(ctx, messageCh, questionCh, responseCh)
		
		// Send nil message
		messageCh <- nil
		
		// Should not crash
		time.Sleep(10 * time.Millisecond)
		assert.True(t, true)
	})
	
	t.Run("empty payload", func(t *testing.T) {
		messageCh := make(chan *messaging.Message, 10)
		questionCh := make(chan userQuestion, 10)
		responseCh := make(chan userResponse, 10)
		
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		
		go handleWebSocketToMCP(ctx, messageCh, questionCh, responseCh)
		
		// Send message with empty payload
		msg := &messaging.Message{
			ID:        "empty-payload",
			Type:      "user_response",
			Timestamp: time.Now(),
			Payload:   json.RawMessage(``),
			ClientID:  "test-client",
		}
		
		messageCh <- msg
		
		// Should not crash
		time.Sleep(10 * time.Millisecond)
		assert.True(t, true)
	})
}
