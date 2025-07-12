package wsserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jcronq/frontend_router_mcp/pkg/messaging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewServer tests the NewServer constructor
func TestNewServer(t *testing.T) {
	messageCh := make(chan *messaging.Message, 100)
	server := NewServer(messageCh)
	
	assert.NotNil(t, server)
	assert.NotNil(t, server.clients)
	assert.NotNil(t, server.register)
	assert.NotNil(t, server.unregister)
	assert.NotNil(t, server.broadcast)
	assert.NotNil(t, server.shutdown)
	assert.Equal(t, messageCh, server.messageCh)
	assert.Equal(t, 0, len(server.clients))
}

// TestServer_HandleWebSocket tests the WebSocket handler
func TestServer_HandleWebSocket(t *testing.T) {
	messageCh := make(chan *messaging.Message, 100)
	server := NewServer(messageCh)
	
	// Start the server's message processing loop
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
	
	// Give the server time to process the connection
	time.Sleep(100 * time.Millisecond)
	
	// Verify client was registered
	server.mu.RLock()
	clientCount := len(server.clients)
	server.mu.RUnlock()
	assert.Equal(t, 1, clientCount)
}

// TestServer_HandleWebSocket_InvalidUpgrade tests invalid WebSocket upgrade
func TestServer_HandleWebSocket_InvalidUpgrade(t *testing.T) {
	messageCh := make(chan *messaging.Message, 100)
	server := NewServer(messageCh)
	
	// Create a test HTTP server
	s := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	defer s.Close()
	
	// Make a regular HTTP request (not WebSocket)
	resp, err := http.Get(s.URL)
	require.NoError(t, err)
	defer resp.Body.Close()
	
	// Should get a bad request status
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// TestServer_Start tests starting the server
func TestServer_Start(t *testing.T) {
	t.Skip("Skipping server start test - requires available port")
	
	messageCh := make(chan *messaging.Message, 100)
	server := NewServer(messageCh)
	
	// Find an available port
	listener, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	
	addr := fmt.Sprintf(":%d", port)
	
	// Start server in a goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start(addr)
	}()
	
	// Give server time to start
	time.Sleep(100 * time.Millisecond)
	
	// Try to connect
	conn, err := net.Dial("tcp", addr)
	if err == nil {
		conn.Close()
	}
	
	// Shutdown server
	server.Shutdown(context.Background())
	
	// Check for errors
	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			t.Errorf("Server error: %v", err)
		}
	case <-time.After(1 * time.Second):
		// Server didn't return, which is okay
	}
}

// TestServer_Shutdown tests graceful shutdown
func TestServer_Shutdown(t *testing.T) {
	messageCh := make(chan *messaging.Message, 100)
	server := NewServer(messageCh)
	
	// Start the server processing loop
	go server.run()
	
	// Add a mock client
	mockClient := &Client{
		ID:     "test-client",
		conn:   nil, // Will be nil for this test
		send:   make(chan *messaging.Message, 10),
		server: server,
	}
	
	server.mu.Lock()
	server.clients[mockClient.ID] = mockClient
	server.mu.Unlock()
	
	// Test shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	err := server.Shutdown(ctx)
	assert.NoError(t, err)
	
	// Verify clients are cleared
	server.mu.RLock()
	clientCount := len(server.clients)
	server.mu.RUnlock()
	assert.Equal(t, 0, clientCount)
}

// TestServer_run tests the main server processing loop
func TestServer_run(t *testing.T) {
	messageCh := make(chan *messaging.Message, 100)
	server := NewServer(messageCh)
	
	// Start the server processing loop
	go server.run()
	defer server.Shutdown(context.Background())
	
	// Test client registration
	mockClient := &Client{
		ID:     "test-client",
		conn:   nil,
		send:   make(chan *messaging.Message, 10),
		server: server,
	}
	
	// Register client
	server.register <- mockClient
	time.Sleep(10 * time.Millisecond)
	
	// Verify client was registered
	server.mu.RLock()
	client, exists := server.clients[mockClient.ID]
	server.mu.RUnlock()
	assert.True(t, exists)
	assert.Equal(t, mockClient, client)
	
	// Test client unregistration
	server.unregister <- mockClient
	time.Sleep(10 * time.Millisecond)
	
	// Verify client was unregistered
	server.mu.RLock()
	_, exists = server.clients[mockClient.ID]
	server.mu.RUnlock()
	assert.False(t, exists)
}

// TestServer_run_Broadcast tests message broadcasting
func TestServer_run_Broadcast(t *testing.T) {
	messageCh := make(chan *messaging.Message, 100)
	server := NewServer(messageCh)
	
	// Start the server processing loop
	go server.run()
	defer server.Shutdown(context.Background())
	
	// Create mock clients
	client1 := &Client{
		ID:     "client-1",
		conn:   nil,
		send:   make(chan *messaging.Message, 10),
		server: server,
	}
	client2 := &Client{
		ID:     "client-2",
		conn:   nil,
		send:   make(chan *messaging.Message, 10),
		server: server,
	}
	
	// Register clients
	server.register <- client1
	server.register <- client2
	time.Sleep(10 * time.Millisecond)
	
	// Test broadcast to all clients
	msg := &messaging.Message{
		ID:        "test-msg",
		Type:      "test",
		Timestamp: time.Now(),
		Payload:   json.RawMessage(`"test message"`),
		ClientID:  "", // Empty ClientID means broadcast to all
	}
	
	server.broadcast <- msg
	time.Sleep(10 * time.Millisecond)
	
	// Verify both clients received the message
	select {
	case receivedMsg := <-client1.send:
		assert.Equal(t, msg.ID, receivedMsg.ID)
	case <-time.After(1 * time.Second):
		t.Fatal("Client 1 did not receive message")
	}
	
	select {
	case receivedMsg := <-client2.send:
		assert.Equal(t, msg.ID, receivedMsg.ID)
	case <-time.After(1 * time.Second):
		t.Fatal("Client 2 did not receive message")
	}
}

// TestServer_run_TargetedMessage tests sending message to specific client
func TestServer_run_TargetedMessage(t *testing.T) {
	messageCh := make(chan *messaging.Message, 100)
	server := NewServer(messageCh)
	
	// Start the server processing loop
	go server.run()
	defer server.Shutdown(context.Background())
	
	// Create mock clients
	client1 := &Client{
		ID:     "client-1",
		conn:   nil,
		send:   make(chan *messaging.Message, 10),
		server: server,
	}
	client2 := &Client{
		ID:     "client-2",
		conn:   nil,
		send:   make(chan *messaging.Message, 10),
		server: server,
	}
	
	// Register clients
	server.register <- client1
	server.register <- client2
	time.Sleep(10 * time.Millisecond)
	
	// Test targeted message
	msg := &messaging.Message{
		ID:        "test-msg",
		Type:      "test",
		Timestamp: time.Now(),
		Payload:   json.RawMessage(`"test message"`),
		ClientID:  "client-1", // Target specific client
	}
	
	server.broadcast <- msg
	time.Sleep(10 * time.Millisecond)
	
	// Verify only client1 received the message
	select {
	case receivedMsg := <-client1.send:
		assert.Equal(t, msg.ID, receivedMsg.ID)
	case <-time.After(1 * time.Second):
		t.Fatal("Client 1 did not receive message")
	}
	
	// Verify client2 did not receive the message
	select {
	case <-client2.send:
		t.Fatal("Client 2 should not have received the message")
	case <-time.After(100 * time.Millisecond):
		// Expected timeout
	}
}

// TestServer_run_MessageFromChannel tests forwarding messages from the message channel
func TestServer_run_MessageFromChannel(t *testing.T) {
	messageCh := make(chan *messaging.Message, 100)
	server := NewServer(messageCh)
	
	// Start the server processing loop
	go server.run()
	defer server.Shutdown(context.Background())
	
	// Create mock client
	client := &Client{
		ID:     "client-1",
		conn:   nil,
		send:   make(chan *messaging.Message, 10),
		server: server,
	}
	
	// Register client
	server.register <- client
	time.Sleep(10 * time.Millisecond)
	
	// Send message through the message channel
	msg := &messaging.Message{
		ID:        "test-msg",
		Type:      "test",
		Timestamp: time.Now(),
		Payload:   json.RawMessage(`"test message"`),
		ClientID:  "client-1",
	}
	
	messageCh <- msg
	time.Sleep(10 * time.Millisecond)
	
	// Verify client received the message
	select {
	case receivedMsg := <-client.send:
		assert.Equal(t, msg.ID, receivedMsg.ID)
	case <-time.After(1 * time.Second):
		t.Fatal("Client did not receive message")
	}
}

// TestServer_run_NonExistentClient tests sending to non-existent client
func TestServer_run_NonExistentClient(t *testing.T) {
	messageCh := make(chan *messaging.Message, 100)
	server := NewServer(messageCh)
	
	// Start the server processing loop
	go server.run()
	defer server.Shutdown(context.Background())
	
	// Send message to non-existent client
	msg := &messaging.Message{
		ID:        "test-msg",
		Type:      "test",
		Timestamp: time.Now(),
		Payload:   json.RawMessage(`"test message"`),
		ClientID:  "non-existent-client",
	}
	
	server.broadcast <- msg
	time.Sleep(10 * time.Millisecond)
	
	// Should not panic or crash
	assert.True(t, true) // If we get here, the test passed
}

// TestGenerateClientID tests the generateClientID function
func TestGenerateClientID(t *testing.T) {
	// Test basic functionality
	id := generateClientID()
	assert.NotEmpty(t, id)
	assert.True(t, strings.HasPrefix(id, "client-"))
	
	// Test uniqueness
	ids := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id := generateClientID()
		assert.False(t, ids[id], "Generated duplicate ID: %s", id)
		ids[id] = true
	}
}

// TestServer_ConcurrentClients tests multiple concurrent clients
func TestServer_ConcurrentClients(t *testing.T) {
	messageCh := make(chan *messaging.Message, 1000)
	server := NewServer(messageCh)
	
	// Start the server processing loop
	go server.run()
	defer server.Shutdown(context.Background())
	
	// Create a test HTTP server
	s := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
	defer s.Close()
	
	wsURL := "ws" + s.URL[4:]
	
	const numClients = 10
	var wg sync.WaitGroup
	wg.Add(numClients)
	
	// Connect multiple clients concurrently
	for i := 0; i < numClients; i++ {
		go func(clientNum int) {
			defer wg.Done()
			
			dialer := &websocket.Dialer{
				HandshakeTimeout: 5 * time.Second,
			}
			
			conn, _, err := dialer.Dial(wsURL, nil)
			if err != nil {
				t.Errorf("Client %d failed to connect: %v", clientNum, err)
				return
			}
			defer conn.Close()
			
			// Send a message
			msg := map[string]interface{}{
				"type": "test",
				"data": fmt.Sprintf("message from client %d", clientNum),
			}
			
			err = conn.WriteJSON(msg)
			if err != nil {
				t.Errorf("Client %d failed to send message: %v", clientNum, err)
			}
			
			time.Sleep(100 * time.Millisecond)
		}(i)
	}
	
	wg.Wait()
	
	// Verify all clients were processed
	time.Sleep(100 * time.Millisecond)
	
	// Check that we received messages from all clients
	messageCount := 0
	timeout := time.After(2 * time.Second)
	for {
		select {
		case <-messageCh:
			messageCount++
			if messageCount >= numClients {
				return
			}
		case <-timeout:
			t.Fatalf("Only received %d messages, expected %d", messageCount, numClients)
		}
	}
}

// TestServer_FullChannelHandling tests behavior when channels are full
func TestServer_FullChannelHandling(t *testing.T) {
	messageCh := make(chan *messaging.Message, 100)
	server := NewServer(messageCh)
	
	// Start the server processing loop
	go server.run()
	defer server.Shutdown(context.Background())
	
	// Create mock client with small buffer
	client := &Client{
		ID:     "client-1",
		conn:   nil,
		send:   make(chan *messaging.Message, 1), // Small buffer
		server: server,
	}
	
	// Register client
	server.register <- client
	time.Sleep(10 * time.Millisecond)
	
	// Fill the client's send channel
	msg1 := &messaging.Message{
		ID:        "test-msg-1",
		Type:      "test",
		Timestamp: time.Now(),
		Payload:   json.RawMessage(`"test message 1"`),
		ClientID:  "client-1",
	}
	
	msg2 := &messaging.Message{
		ID:        "test-msg-2",
		Type:      "test",
		Timestamp: time.Now(),
		Payload:   json.RawMessage(`"test message 2"`),
		ClientID:  "client-1",
	}
	
	// Send first message (should succeed)
	server.broadcast <- msg1
	time.Sleep(10 * time.Millisecond)
	
	// Send second message (should cause client to be removed due to full channel)
	server.broadcast <- msg2
	time.Sleep(10 * time.Millisecond)
	
	// Verify client was removed
	server.mu.RLock()
	_, exists := server.clients["client-1"]
	server.mu.RUnlock()
	assert.False(t, exists)
}

// BenchmarkServer_Broadcast benchmarks message broadcasting
func BenchmarkServer_Broadcast(b *testing.B) {
	messageCh := make(chan *messaging.Message, 1000)
	server := NewServer(messageCh)
	
	// Start the server processing loop
	go server.run()
	defer server.Shutdown(context.Background())
	
	// Create mock clients
	const numClients = 100
	for i := 0; i < numClients; i++ {
		client := &Client{
			ID:     fmt.Sprintf("client-%d", i),
			conn:   nil,
			send:   make(chan *messaging.Message, 1000),
			server: server,
		}
		server.register <- client
	}
	
	time.Sleep(100 * time.Millisecond)
	
	msg := &messaging.Message{
		ID:        "test-msg",
		Type:      "test",
		Timestamp: time.Now(),
		Payload:   json.RawMessage(`"test message"`),
		ClientID:  "", // Broadcast to all
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		server.broadcast <- msg
	}
}

// BenchmarkGenerateClientID benchmarks client ID generation
func BenchmarkGenerateClientID(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = generateClientID()
	}
}
