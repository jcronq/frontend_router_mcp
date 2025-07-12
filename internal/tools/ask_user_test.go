package tools

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewAskUserTool tests the constructor
func TestNewAskUserTool(t *testing.T) {
	handler := func(requestID string, question string) error {
		return nil
	}
	
	tool := NewAskUserTool(handler)
	
	assert.NotNil(t, tool)
	assert.NotNil(t, tool.pendingRequests)
	assert.NotNil(t, tool.questionHandler)
	assert.Equal(t, 0, len(tool.pendingRequests))
}

// TestAskUserTool_Name tests the Name method
func TestAskUserTool_Name(t *testing.T) {
	handler := func(requestID string, question string) error {
		return nil
	}
	
	tool := NewAskUserTool(handler)
	assert.Equal(t, "ask_user", tool.Name())
}

// TestAskUserTool_Description tests the Description method
func TestAskUserTool_Description(t *testing.T) {
	handler := func(requestID string, question string) error {
		return nil
	}
	
	tool := NewAskUserTool(handler)
	assert.Equal(t, "Ask a question to the user and receive their response", tool.Description())
}

// TestAskUserTool_GetSchema tests the GetSchema method
func TestAskUserTool_GetSchema(t *testing.T) {
	handler := func(requestID string, question string) error {
		return nil
	}
	
	tool := NewAskUserTool(handler)
	schema := tool.GetSchema()
	
	assert.NotNil(t, schema)
	assert.Equal(t, "object", schema["type"])
	
	properties, ok := schema["properties"].(map[string]interface{})
	assert.True(t, ok)
	assert.Contains(t, properties, "question")
	assert.Contains(t, properties, "timeout_seconds")
	
	required, ok := schema["required"].([]string)
	assert.True(t, ok)
	assert.Contains(t, required, "question")
}

// TestAskUserTool_Execute_Success tests successful execution
func TestAskUserTool_Execute_Success(t *testing.T) {
	var receivedRequestID, receivedQuestion string
	handler := func(requestID string, question string) error {
		receivedRequestID = requestID
		receivedQuestion = question
		return nil
	}
	
	tool := NewAskUserTool(handler)
	
	params := map[string]interface{}{
		"question":        "What is your name?",
		"timeout_seconds": 1.0,
	}
	paramsJSON, _ := json.Marshal(params)
	
	// Start execution in a goroutine
	resultCh := make(chan interface{}, 1)
	errCh := make(chan error, 1)
	
	go func() {
		result, err := tool.Execute(context.Background(), paramsJSON)
		resultCh <- result
		errCh <- err
	}()
	
	// Wait for the handler to be called
	time.Sleep(10 * time.Millisecond)
	
	// Verify the handler was called with correct parameters
	assert.Equal(t, "What is your name?", receivedQuestion)
	assert.NotEmpty(t, receivedRequestID)
	
	// Simulate user response
	err := tool.HandleUserResponse(receivedRequestID, "John Doe")
	require.NoError(t, err)
	
	// Check the result
	select {
	case result := <-resultCh:
		err := <-errCh
		assert.NoError(t, err)
		
		resultMap, ok := result.(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, "John Doe", resultMap["answer"])
		
	case <-time.After(2 * time.Second):
		t.Fatal("Execute did not complete within timeout")
	}
}

// TestAskUserTool_Execute_Timeout tests timeout behavior
func TestAskUserTool_Execute_Timeout(t *testing.T) {
	handler := func(requestID string, question string) error {
		return nil
	}
	
	tool := NewAskUserTool(handler)
	
	params := map[string]interface{}{
		"question":        "What is your name?",
		"timeout_seconds": 0.1, // Very short timeout
	}
	paramsJSON, _ := json.Marshal(params)
	
	ctx := context.Background()
	result, err := tool.Execute(ctx, paramsJSON)
	
	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timeout waiting for user response")
}

// TestAskUserTool_Execute_ContextCanceled tests context cancellation
func TestAskUserTool_Execute_ContextCanceled(t *testing.T) {
	handler := func(requestID string, question string) error {
		return nil
	}
	
	tool := NewAskUserTool(handler)
	
	params := map[string]interface{}{
		"question":        "What is your name?",
		"timeout_seconds": 10.0,
	}
	paramsJSON, _ := json.Marshal(params)
	
	ctx, cancel := context.WithCancel(context.Background())
	
	// Start execution in a goroutine
	resultCh := make(chan interface{}, 1)
	errCh := make(chan error, 1)
	
	go func() {
		result, err := tool.Execute(ctx, paramsJSON)
		resultCh <- result
		errCh <- err
	}()
	
	// Cancel the context after a short delay
	time.Sleep(10 * time.Millisecond)
	cancel()
	
	// Check the result
	select {
	case result := <-resultCh:
		err := <-errCh
		assert.Nil(t, result)
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
		
	case <-time.After(2 * time.Second):
		t.Fatal("Execute did not complete within timeout")
	}
}

// TestAskUserTool_Execute_InvalidJSON tests invalid JSON parameter handling
func TestAskUserTool_Execute_InvalidJSON(t *testing.T) {
	handler := func(requestID string, question string) error {
		return nil
	}
	
	tool := NewAskUserTool(handler)
	
	invalidJSON := json.RawMessage(`{"question": "test", "timeout_seconds": invalid}`)
	
	result, err := tool.Execute(context.Background(), invalidJSON)
	
	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse parameters")
}

// TestAskUserTool_Execute_EmptyQuestion tests empty question handling
func TestAskUserTool_Execute_EmptyQuestion(t *testing.T) {
	handler := func(requestID string, question string) error {
		return nil
	}
	
	tool := NewAskUserTool(handler)
	
	params := map[string]interface{}{
		"question":        "",
		"timeout_seconds": 1.0,
	}
	paramsJSON, _ := json.Marshal(params)
	
	result, err := tool.Execute(context.Background(), paramsJSON)
	
	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Equal(t, "question parameter is required", err.Error())
}

// TestAskUserTool_Execute_DefaultTimeout tests default timeout behavior
func TestAskUserTool_Execute_DefaultTimeout(t *testing.T) {
	var receivedRequestID, receivedQuestion string
	handler := func(requestID string, question string) error {
		receivedRequestID = requestID
		receivedQuestion = question
		return nil
	}
	
	tool := NewAskUserTool(handler)
	
	params := map[string]interface{}{
		"question": "What is your name?",
		// No timeout_seconds specified
	}
	paramsJSON, _ := json.Marshal(params)
	
	// Start execution in a goroutine
	resultCh := make(chan interface{}, 1)
	errCh := make(chan error, 1)
	
	go func() {
		result, err := tool.Execute(context.Background(), paramsJSON)
		resultCh <- result
		errCh <- err
	}()
	
	// Wait for the handler to be called
	time.Sleep(10 * time.Millisecond)
	
	// Verify the handler was called
	assert.Equal(t, "What is your name?", receivedQuestion)
	assert.NotEmpty(t, receivedRequestID)
	
	// Simulate user response
	err := tool.HandleUserResponse(receivedRequestID, "John Doe")
	require.NoError(t, err)
	
	// Check the result
	select {
	case result := <-resultCh:
		err := <-errCh
		assert.NoError(t, err)
		
		resultMap, ok := result.(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, "John Doe", resultMap["answer"])
		
	case <-time.After(2 * time.Second):
		t.Fatal("Execute did not complete within timeout")
	}
}

// TestAskUserTool_Execute_HandlerError tests question handler error
func TestAskUserTool_Execute_HandlerError(t *testing.T) {
	handler := func(requestID string, question string) error {
		return assert.AnError
	}
	
	tool := NewAskUserTool(handler)
	
	params := map[string]interface{}{
		"question":        "What is your name?",
		"timeout_seconds": 1.0,
	}
	paramsJSON, _ := json.Marshal(params)
	
	result, err := tool.Execute(context.Background(), paramsJSON)
	
	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to send question to user")
}

// TestAskUserTool_HandleUserResponse_Success tests successful response handling
func TestAskUserTool_HandleUserResponse_Success(t *testing.T) {
	handler := func(requestID string, question string) error {
		return nil
	}
	
	tool := NewAskUserTool(handler)
	
	// Manually add a pending request
	requestID := "test-request-123"
	responseCh := make(chan string, 1)
	
	tool.pendingRequestsLock.Lock()
	tool.pendingRequests[requestID] = responseCh
	tool.pendingRequestsLock.Unlock()
	
	// Handle user response
	err := tool.HandleUserResponse(requestID, "Test Answer")
	assert.NoError(t, err)
	
	// Verify the response was sent
	select {
	case response := <-responseCh:
		assert.Equal(t, "Test Answer", response)
	case <-time.After(1 * time.Second):
		t.Fatal("Response was not received")
	}
}

// TestAskUserTool_HandleUserResponse_NonExistentRequest tests non-existent request handling
func TestAskUserTool_HandleUserResponse_NonExistentRequest(t *testing.T) {
	handler := func(requestID string, question string) error {
		return nil
	}
	
	tool := NewAskUserTool(handler)
	
	err := tool.HandleUserResponse("non-existent-request", "Test Answer")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no pending request found with ID")
}

// TestAskUserTool_HandleUserResponse_ChannelFull tests channel full condition
func TestAskUserTool_HandleUserResponse_ChannelFull(t *testing.T) {
	handler := func(requestID string, question string) error {
		return nil
	}
	
	tool := NewAskUserTool(handler)
	
	// Create a buffered channel and fill it
	requestID := "test-request-123"
	responseCh := make(chan string, 1)
	responseCh <- "first response" // Fill the channel
	
	tool.pendingRequestsLock.Lock()
	tool.pendingRequests[requestID] = responseCh
	tool.pendingRequestsLock.Unlock()
	
	// Try to handle another response
	err := tool.HandleUserResponse(requestID, "Test Answer")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to send response")
}

// TestAskUserTool_ConcurrentAccess tests concurrent access to pending requests
func TestAskUserTool_ConcurrentAccess(t *testing.T) {
	handler := func(requestID string, question string) error {
		return nil
	}
	
	tool := NewAskUserTool(handler)
	
	const numGoroutines = 100
	const numRequests = 10
	
	var wg sync.WaitGroup
	wg.Add(numGoroutines)
	
	// Launch multiple goroutines to simulate concurrent access
	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer wg.Done()
			
			for j := 0; j < numRequests; j++ {
				requestID := generateRequestID()
				responseCh := make(chan string, 1)
				
				// Add request
				tool.pendingRequestsLock.Lock()
				tool.pendingRequests[requestID] = responseCh
				tool.pendingRequestsLock.Unlock()
				
				// Handle response
				err := tool.HandleUserResponse(requestID, "Test Answer")
				assert.NoError(t, err)
				
				// Remove request
				tool.pendingRequestsLock.Lock()
				delete(tool.pendingRequests, requestID)
				tool.pendingRequestsLock.Unlock()
			}
		}(i)
	}
	
	wg.Wait()
	
	// Verify all requests were cleaned up
	tool.pendingRequestsLock.Lock()
	assert.Equal(t, 0, len(tool.pendingRequests))
	tool.pendingRequestsLock.Unlock()
}

// TestGenerateRequestID tests the generateRequestID function
func TestGenerateRequestID(t *testing.T) {
	// Generate multiple IDs to test uniqueness
	ids := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id := generateRequestID()
		
		// Check format
		assert.Contains(t, id, "req-")
		assert.True(t, len(id) > 10) // Should be reasonably long
		
		// Check uniqueness
		assert.False(t, ids[id], "Generated duplicate ID: %s", id)
		ids[id] = true
	}
}

// BenchmarkAskUserTool_Execute benchmarks the Execute method
func BenchmarkAskUserTool_Execute(b *testing.B) {
	handler := func(requestID string, question string) error {
		return nil
	}
	
	tool := NewAskUserTool(handler)
	
	params := map[string]interface{}{
		"question":        "What is your name?",
		"timeout_seconds": 0.001, // Very short timeout to complete quickly
	}
	paramsJSON, _ := json.Marshal(params)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = tool.Execute(context.Background(), paramsJSON)
	}
}

// BenchmarkGenerateRequestID benchmarks the generateRequestID function
func BenchmarkGenerateRequestID(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = generateRequestID()
	}
}

// TestAskUserTool_Execute_RaceCondition tests for race conditions
func TestAskUserTool_Execute_RaceCondition(t *testing.T) {
	handler := func(requestID string, question string) error {
		return nil
	}
	
	tool := NewAskUserTool(handler)
	
	params := map[string]interface{}{
		"question":        "What is your name?",
		"timeout_seconds": 1.0,
	}
	paramsJSON, _ := json.Marshal(params)
	
	const numGoroutines = 50
	var wg sync.WaitGroup
	wg.Add(numGoroutines)
	
	// Launch multiple executions concurrently
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			
			// Start execution
			resultCh := make(chan interface{}, 1)
			errCh := make(chan error, 1)
			
			go func() {
				result, err := tool.Execute(context.Background(), paramsJSON)
				resultCh <- result
				errCh <- err
			}()
			
			// Wait for completion (timeout or response)
			select {
			case <-resultCh:
				<-errCh
			case <-time.After(2 * time.Second):
				// Timeout is expected for most of these since we're not providing responses
			}
		}()
	}
	
	wg.Wait()
	
	// Verify no pending requests remain
	tool.pendingRequestsLock.Lock()
	pendingCount := len(tool.pendingRequests)
	tool.pendingRequestsLock.Unlock()
	
	assert.Equal(t, 0, pendingCount, "All pending requests should be cleaned up")
}