package wsserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jcronq/frontend_router_mcp/pkg/messaging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestClient_SendResponse tests the SendResponse method implementation
func TestClient_SendResponse(t *testing.T) {
	messageCh := make(chan *messaging.Message, 100)
	server := NewServer(messageCh)
	
	// Start the server processing loop
	go server.run()
	defer server.Shutdown(context.Background())
	
	// Create a test HTTP server
	s := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	defer s.Close()
	
	wsURL := "ws" + s.URL[4:]
	
	// Connect to the WebSocket server
	dialer := &websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}
	
	conn, _, err := dialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()
	
	// Give time for connection to be established
	time.Sleep(100 * time.Millisecond)
	
	// Get the client by sending a message and capturing the response sender
	testMessage := map[string]interface{}{
		"type": "test_message",
	}
	
	err = conn.WriteJSON(testMessage)
	require.NoError(t, err)
	
	// Receive the message to get the client's response sender
	var clientResponseSender messaging.ResponseSender
	select {
	case msg := <-messageCh:
		clientResponseSender = msg.ResponseSender
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for message")
	}
	
	require.NotNil(t, clientResponseSender)
	
	// Test sending a response back to the client
	responseMsg := &messaging.Message{
		ID:        "response-123",
		Type:      "response",
		Timestamp: time.Now(),
		Payload:   json.RawMessage(`{"status": "success", "data": "hello from server"}`),
	}
	
	err = clientResponseSender.SendResponse(responseMsg)
	assert.NoError(t, err)
	
	// Set read deadline and read the response from the WebSocket
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, message, err := conn.ReadMessage()
	require.NoError(t, err)
	
	// Verify the response message
	var receivedResponse map[string]interface{}
	err = json.Unmarshal(message, &receivedResponse)
	require.NoError(t, err)
	
	assert.Equal(t, "success", receivedResponse["status"])
	assert.Equal(t, "hello from server", receivedResponse["data"])
}

// TestClient_SendResponse_ChannelFull tests SendResponse when send channel is full
func TestClient_SendResponse_ChannelFull(t *testing.T) {
	messageCh := make(chan *messaging.Message, 100)
	server := NewServer(messageCh)
	
	// Create a mock client with a small send channel
	client := &Client{
		ID:     "test-client",
		conn:   nil, // Don't need actual connection for this test
		send:   make(chan *messaging.Message, 1), // Small buffer
		server: server,
	}
	
	// Fill the send channel
	msg1 := &messaging.Message{
		ID:        "msg-1",
		Type:      "test",
		Timestamp: time.Now(),
		Payload:   json.RawMessage(`"test message 1"`),
	}
	
	client.send <- msg1
	
	// Try to send another message (should not block and should not return error)
	msg2 := &messaging.Message{
		ID:        "msg-2",
		Type:      "test",
		Timestamp: time.Now(),
		Payload:   json.RawMessage(`"test message 2"`),
	}
	
	err := client.SendResponse(msg2)
	assert.NoError(t, err) // Should not return error even if channel is full
	
	// Verify the channel still has the first message
	select {
	case receivedMsg := <-client.send:
		assert.Equal(t, msg1.ID, receivedMsg.ID)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Expected to receive the first message")
	}
	
	// Verify the channel is now empty (second message was dropped)
	select {
	case <-client.send:
		t.Fatal("Channel should be empty after receiving the first message")
	case <-time.After(100 * time.Millisecond):
		// Expected - channel should be empty
	}
}

// TestClient_SendResponse_MultipleConcurrentSends tests concurrent SendResponse calls
func TestClient_SendResponse_MultipleConcurrentSends(t *testing.T) {
	messageCh := make(chan *messaging.Message, 100)
	server := NewServer(messageCh)
	
	// Start the server processing loop
	go server.run()
	defer server.Shutdown(context.Background())
	
	// Create a test HTTP server
	s := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	defer s.Close()
	
	wsURL := "ws" + s.URL[4:]
	
	// Connect to the WebSocket server
	dialer := &websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}
	
	conn, _, err := dialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()
	
	// Give time for connection to be established
	time.Sleep(100 * time.Millisecond)
	
	// Get the client's response sender
	testMessage := map[string]interface{}{
		"type": "test_message",
	}
	
	err = conn.WriteJSON(testMessage)
	require.NoError(t, err)
	
	var clientResponseSender messaging.ResponseSender
	select {
	case msg := <-messageCh:
		clientResponseSender = msg.ResponseSender
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for message")
	}
	
	require.NotNil(t, clientResponseSender)
	
	// Send multiple responses concurrently
	const numResponses = 100
	done := make(chan bool, numResponses)
	
	for i := 0; i < numResponses; i++ {
		go func(index int) {
			msg := &messaging.Message{
				ID:        string(rune('a' + index)),
				Type:      "concurrent_response",
				Timestamp: time.Now(),
				Payload:   json.RawMessage(`{"index": ` + string(rune('0'+index)) + `}`),
			}
			
			err := clientResponseSender.SendResponse(msg)
			assert.NoError(t, err)
			done <- true
		}(i)
	}
	
	// Wait for all goroutines to complete
	for i := 0; i < numResponses; i++ {
		select {
		case <-done:
			// Success
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for concurrent sends to complete")
		}
	}
	
	// Read messages from the WebSocket to verify they were sent
	receivedCount := 0
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	
	for receivedCount < numResponses {
		_, _, err := conn.ReadMessage()
		if err != nil {
			// Some messages might be dropped if the channel is full, which is expected
			break
		}
		receivedCount++
	}
	
	// We should have received at least some messages
	assert.Greater(t, receivedCount, 0, "Should have received at least some messages")
}

// TestClient_SendResponse_NilMessage tests SendResponse with nil message
func TestClient_SendResponse_NilMessage(t *testing.T) {
	client := &Client{
		ID:     "test-client",
		conn:   nil,
		send:   make(chan *messaging.Message, 10),
		server: nil,
	}
	
	// This should not panic
	err := client.SendResponse(nil)
	assert.NoError(t, err)
	
	// Verify nil message was sent to channel
	select {
	case msg := <-client.send:
		assert.Nil(t, msg)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Expected to receive nil message")
	}
}

// TestClient_SendResponse_LargeMessage tests SendResponse with large message
func TestClient_SendResponse_LargeMessage(t *testing.T) {
	messageCh := make(chan *messaging.Message, 100)
	server := NewServer(messageCh)
	
	// Start the server processing loop
	go server.run()
	defer server.Shutdown(context.Background())
	
	// Create a test HTTP server
	s := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	defer s.Close()
	
	wsURL := "ws" + s.URL[4:]
	
	// Connect to the WebSocket server
	dialer := &websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
	}
	
	conn, _, err := dialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()
	
	// Give time for connection to be established
	time.Sleep(100 * time.Millisecond)
	
	// Get the client's response sender
	testMessage := map[string]interface{}{
		"type": "test_message",
	}
	
	err = conn.WriteJSON(testMessage)
	require.NoError(t, err)
	
	var clientResponseSender messaging.ResponseSender
	select {
	case msg := <-messageCh:
		clientResponseSender = msg.ResponseSender
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for message")
	}
	
	require.NotNil(t, clientResponseSender)
	
	// Create a large message (1MB of data)
	largeData := make([]byte, 1024*1024)
	for i := range largeData {
		largeData[i] = byte('a' + (i % 26))
	}
	
	largeMessage := &messaging.Message{
		ID:        "large-msg",
		Type:      "large_response",
		Timestamp: time.Now(),
		Payload:   json.RawMessage(`{"data": "` + string(largeData) + `"}`),
	}
	
	err = clientResponseSender.SendResponse(largeMessage)
	assert.NoError(t, err)
	
	// Try to read the large message
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, message, err := conn.ReadMessage()
	require.NoError(t, err)
	
	// Verify the message is the expected size
	assert.Greater(t, len(message), 1024*1024, "Message should be at least 1MB")
	
	// Verify the message structure
	var receivedResponse map[string]interface{}
	err = json.Unmarshal(message, &receivedResponse)
	require.NoError(t, err)
	
	assert.Contains(t, receivedResponse, "data")
	receivedData, ok := receivedResponse["data"].(string)
	assert.True(t, ok)
	assert.Equal(t, len(largeData), len(receivedData))
}

// TestClient_SendResponse_InterfaceCompliance tests that Client implements ResponseSender interface
func TestClient_SendResponse_InterfaceCompliance(t *testing.T) {
	client := &Client{
		ID:     "test-client",
		conn:   nil,
		send:   make(chan *messaging.Message, 10),
		server: nil,
	}
	
	// This should compile if Client implements ResponseSender
	var responseSender messaging.ResponseSender = client
	assert.NotNil(t, responseSender)
	
	// Test that the interface method works
	msg := &messaging.Message{
		ID:        "test-msg",
		Type:      "test",
		Timestamp: time.Now(),
		Payload:   json.RawMessage(`"test"`),
	}
	
	err := responseSender.SendResponse(msg)
	assert.NoError(t, err)
	
	// Verify message was sent
	select {
	case receivedMsg := <-client.send:
		assert.Equal(t, msg.ID, receivedMsg.ID)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Expected to receive message")
	}
}

// TestClient_SendResponse_EmptyMessage tests SendResponse with empty message fields
func TestClient_SendResponse_EmptyMessage(t *testing.T) {
	client := &Client{
		ID:     "test-client",
		conn:   nil,
		send:   make(chan *messaging.Message, 10),
		server: nil,
	}
	
	// Test with empty message
	emptyMsg := &messaging.Message{}
	
	err := client.SendResponse(emptyMsg)
	assert.NoError(t, err)
	
	// Verify message was sent
	select {
	case receivedMsg := <-client.send:
		assert.Equal(t, emptyMsg, receivedMsg)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Expected to receive message")
	}
}

// BenchmarkClient_SendResponse benchmarks the SendResponse method
func BenchmarkClient_SendResponse(b *testing.B) {
	client := &Client{
		ID:     "test-client",
		conn:   nil,
		send:   make(chan *messaging.Message, 10000), // Large buffer to avoid blocking
		server: nil,
	}
	
	// Drain the channel in a goroutine to prevent blocking
	go func() {
		for range client.send {
		}
	}()
	
	msg := &messaging.Message{
		ID:        "benchmark-msg",
		Type:      "benchmark",
		Timestamp: time.Now(),
		Payload:   json.RawMessage(`{"data": "benchmark test"}`),
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = client.SendResponse(msg)
	}
}

// TestClient_SendResponse_ChannelClosed tests SendResponse when send channel is closed
func TestClient_SendResponse_ChannelClosed(t *testing.T) {
	client := &Client{
		ID:     "test-client",
		conn:   nil,
		send:   make(chan *messaging.Message, 10),
		server: nil,
	}
	
	// Close the send channel
	close(client.send)
	
	msg := &messaging.Message{
		ID:        "test-msg",
		Type:      "test",
		Timestamp: time.Now(),
		Payload:   json.RawMessage(`"test"`),
	}
	
	// This should not panic even with closed channel
	err := client.SendResponse(msg)
	assert.NoError(t, err) // Current implementation doesn't return error for closed channel
}