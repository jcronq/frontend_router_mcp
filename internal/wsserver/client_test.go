package wsserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jcronq/frontend_router_mcp/pkg/messaging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestClient_Connection tests basic WebSocket client connection
func TestClient_Connection(t *testing.T) {
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
	
	// Verify client is registered
	server.mu.RLock()
	clientCount := len(server.clients)
	server.mu.RUnlock()
	assert.Equal(t, 1, clientCount)
}

// TestClient_MessageSending tests sending messages from client to server
func TestClient_MessageSending(t *testing.T) {
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
	
	// Send a test message
	testMessage := map[string]interface{}{
		"type": "test_message",
		"data": "hello world",
	}
	
	err = conn.WriteJSON(testMessage)
	require.NoError(t, err)
	
	// Verify the message was received
	select {
	case msg := <-messageCh:
		assert.Equal(t, "test_message", msg.Type)
		assert.NotEmpty(t, msg.ClientID)
		assert.NotEmpty(t, msg.ID)
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for message")
	}
}

// TestClient_ConnectionClose tests client connection closure
func TestClient_ConnectionClose(t *testing.T) {
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
	
	// Give time for connection to be established
	time.Sleep(100 * time.Millisecond)
	
	// Verify client is registered
	server.mu.RLock()
	initialClientCount := len(server.clients)
	server.mu.RUnlock()
	assert.Equal(t, 1, initialClientCount)
	
	// Close the connection
	err = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	require.NoError(t, err)
	conn.Close()
	
	// Give time for disconnection to be processed
	time.Sleep(100 * time.Millisecond)
	
	// Verify client is unregistered
	server.mu.RLock()
	finalClientCount := len(server.clients)
	server.mu.RUnlock()
	assert.Equal(t, 0, finalClientCount)
}