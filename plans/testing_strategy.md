# Testing Strategy

## Overview
This document outlines the comprehensive testing strategy for the Frontend Router MCP service, covering unit testing, integration testing, load testing, security testing, and end-to-end testing.

## Testing Pyramid

### 1. Unit Tests (70% of test coverage)
- **Scope**: Individual functions and methods
- **Coverage Target**: 90%+
- **Execution Time**: <10 seconds total
- **Isolation**: No external dependencies

### 2. Integration Tests (20% of test coverage)
- **Scope**: Component interactions
- **Coverage Target**: 80%+
- **Execution Time**: <2 minutes total
- **Dependencies**: Test databases, mock services

### 3. End-to-End Tests (10% of test coverage)
- **Scope**: Complete user workflows
- **Coverage Target**: Critical paths only
- **Execution Time**: <10 minutes total
- **Dependencies**: Full environment

## Unit Testing Strategy

### 1. Test Structure and Organization

**Test File Structure:**
```
internal/
├── tools/
│   ├── ask_user.go
│   └── ask_user_test.go
├── wsserver/
│   ├── server.go
│   ├── server_test.go
│   ├── client.go
│   └── client_test.go
└── messaging/
    ├── message.go
    └── message_test.go
```

### 2. AskUser Tool Testing

**Test Implementation:**
```go
// internal/tools/ask_user_test.go
package tools

import (
    "context"
    "encoding/json"
    "testing"
    "time"
    
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
)

type MockQuestionHandler struct {
    mock.Mock
}

func (m *MockQuestionHandler) HandleQuestion(requestID, question string) error {
    args := m.Called(requestID, question)
    return args.Error(0)
}

func TestAskUserTool_Execute_Success(t *testing.T) {
    // Setup
    handler := &MockQuestionHandler{}
    tool := NewAskUserTool(handler.HandleQuestion)
    
    // Mock expectations
    handler.On("HandleQuestion", mock.AnythingOfType("string"), "What is your name?").Return(nil)
    
    // Test data
    params := map[string]interface{}{
        "question": "What is your name?",
        "timeout_seconds": 10,
    }
    paramsJSON, _ := json.Marshal(params)
    
    // Execute in goroutine to simulate async response
    var result interface{}
    var err error
    
    go func() {
        result, err = tool.Execute(context.Background(), paramsJSON)
    }()
    
    // Simulate user response after 100ms
    time.Sleep(100 * time.Millisecond)
    tool.HandleUserResponse("req-123", "John Doe")
    
    // Wait for completion
    time.Sleep(100 * time.Millisecond)
    
    // Assertions
    assert.NoError(t, err)
    assert.NotNil(t, result)
    
    resultMap := result.(map[string]interface{})
    assert.Equal(t, "John Doe", resultMap["answer"])
    
    handler.AssertExpectations(t)
}

func TestAskUserTool_Execute_Timeout(t *testing.T) {
    // Setup
    handler := &MockQuestionHandler{}
    tool := NewAskUserTool(handler.HandleQuestion)
    
    handler.On("HandleQuestion", mock.AnythingOfType("string"), "What is your name?").Return(nil)
    
    // Test data with short timeout
    params := map[string]interface{}{
        "question": "What is your name?",
        "timeout_seconds": 0.1, // 100ms timeout
    }
    paramsJSON, _ := json.Marshal(params)
    
    // Execute
    result, err := tool.Execute(context.Background(), paramsJSON)
    
    // Assertions
    assert.Error(t, err)
    assert.Nil(t, result)
    assert.Contains(t, err.Error(), "timeout")
    
    handler.AssertExpectations(t)
}

func TestAskUserTool_Execute_InvalidParameters(t *testing.T) {
    // Setup
    handler := &MockQuestionHandler{}
    tool := NewAskUserTool(handler.HandleQuestion)
    
    testCases := []struct {
        name     string
        params   map[string]interface{}
        expected string
    }{
        {
            name:     "missing question",
            params:   map[string]interface{}{},
            expected: "question parameter is required",
        },
        {
            name:     "empty question",
            params:   map[string]interface{}{"question": ""},
            expected: "question parameter is required",
        },
        {
            name:     "invalid timeout",
            params:   map[string]interface{}{"question": "test", "timeout_seconds": "invalid"},
            expected: "failed to parse parameters",
        },
    }
    
    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            paramsJSON, _ := json.Marshal(tc.params)
            
            result, err := tool.Execute(context.Background(), paramsJSON)
            
            assert.Error(t, err)
            assert.Nil(t, result)
            assert.Contains(t, err.Error(), tc.expected)
        })
    }
}

func TestAskUserTool_ConcurrentRequests(t *testing.T) {
    // Setup
    handler := &MockQuestionHandler{}
    tool := NewAskUserTool(handler.HandleQuestion)
    
    handler.On("HandleQuestion", mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(nil)
    
    // Test concurrent requests
    const numRequests = 10
    results := make(chan interface{}, numRequests)
    errors := make(chan error, numRequests)
    
    for i := 0; i < numRequests; i++ {
        go func(id int) {
            params := map[string]interface{}{
                "question": fmt.Sprintf("Question %d", id),
                "timeout_seconds": 5,
            }
            paramsJSON, _ := json.Marshal(params)
            
            result, err := tool.Execute(context.Background(), paramsJSON)
            results <- result
            errors <- err
        }(i)
    }
    
    // Simulate responses
    time.Sleep(100 * time.Millisecond)
    for i := 0; i < numRequests; i++ {
        tool.HandleUserResponse(fmt.Sprintf("req-%d", i), fmt.Sprintf("Answer %d", i))
    }
    
    // Collect results
    for i := 0; i < numRequests; i++ {
        select {
        case result := <-results:
            assert.NotNil(t, result)
        case err := <-errors:
            assert.NoError(t, err)
        case <-time.After(2 * time.Second):
            t.Fatal("Timeout waiting for results")
        }
    }
    
    handler.AssertExpectations(t)
}
```

### 3. WebSocket Server Testing

**Test Implementation:**
```go
// internal/wsserver/server_test.go
package wsserver

import (
    "context"
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"
    "time"
    
    "github.com/gorilla/websocket"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    
    "github.com/jcronq/frontend_router_mcp/pkg/messaging"
)

func TestWebSocketServer_HandleConnection(t *testing.T) {
    // Setup
    messageCh := make(chan *messaging.Message, 10)
    server := NewServer(messageCh)
    
    // Create test server
    testServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
    defer testServer.Close()
    
    // Convert URL to WebSocket URL
    wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http") + "/ws"
    
    // Start server goroutine
    go server.run()
    
    // Connect WebSocket client
    ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
    require.NoError(t, err)
    defer ws.Close()
    
    // Send connect message
    connectMsg := map[string]interface{}{
        "type": "connect",
    }
    err = ws.WriteJSON(connectMsg)
    require.NoError(t, err)
    
    // Verify message received
    select {
    case msg := <-messageCh:
        assert.Equal(t, "connect", msg.Type)
        assert.NotEmpty(t, msg.ClientID)
    case <-time.After(time.Second):
        t.Fatal("Timeout waiting for connect message")
    }
}

func TestWebSocketServer_MessageBroadcast(t *testing.T) {
    // Setup
    messageCh := make(chan *messaging.Message, 10)
    server := NewServer(messageCh)
    
    // Create test server
    testServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
    defer testServer.Close()
    
    wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http") + "/ws"
    
    // Start server
    go server.run()
    
    // Connect multiple clients
    const numClients = 3
    clients := make([]*websocket.Conn, numClients)
    
    for i := 0; i < numClients; i++ {
        ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
        require.NoError(t, err)
        defer ws.Close()
        
        clients[i] = ws
        
        // Send connect message
        connectMsg := map[string]interface{}{"type": "connect"}
        err = ws.WriteJSON(connectMsg)
        require.NoError(t, err)
    }
    
    // Wait for connections to be established
    time.Sleep(100 * time.Millisecond)
    
    // Broadcast a message
    broadcastMsg := &messaging.Message{
        ID:        "test-broadcast",
        Type:      "test_message",
        Payload:   []byte(`{"content": "Hello everyone!"}`),
        Timestamp: time.Now(),
    }
    
    server.broadcast <- broadcastMsg
    
    // Verify all clients receive the message
    for i, client := range clients {
        var received map[string]interface{}
        err := client.ReadJSON(&received)
        require.NoError(t, err, "Client %d should receive message", i)
        assert.Equal(t, "Hello everyone!", received["content"])
    }
}

func TestWebSocketServer_ClientDisconnection(t *testing.T) {
    // Setup
    messageCh := make(chan *messaging.Message, 10)
    server := NewServer(messageCh)
    
    testServer := httptest.NewServer(http.HandlerFunc(server.HandleWebSocket))
    defer testServer.Close()
    
    wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http") + "/ws"
    
    go server.run()
    
    // Connect client
    ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
    require.NoError(t, err)
    
    // Send connect message
    connectMsg := map[string]interface{}{"type": "connect"}
    err = ws.WriteJSON(connectMsg)
    require.NoError(t, err)
    
    // Wait for connection
    time.Sleep(100 * time.Millisecond)
    
    // Verify client is connected
    server.mu.RLock()
    clientCount := len(server.clients)
    server.mu.RUnlock()
    assert.Equal(t, 1, clientCount)
    
    // Disconnect client
    ws.Close()
    
    // Wait for cleanup
    time.Sleep(100 * time.Millisecond)
    
    // Verify client is removed
    server.mu.RLock()
    clientCount = len(server.clients)
    server.mu.RUnlock()
    assert.Equal(t, 0, clientCount)
}
```

### 4. Message System Testing

**Test Implementation:**
```go
// pkg/messaging/message_test.go
package messaging

import (
    "testing"
    "time"
    
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestNewMessage(t *testing.T) {
    // Test data
    payload := map[string]interface{}{
        "question": "What is your name?",
        "timeout":  300,
    }
    
    // Create message
    msg, err := NewMessage("ask_user", payload)
    
    // Assertions
    require.NoError(t, err)
    assert.NotEmpty(t, msg.ID)
    assert.Equal(t, "ask_user", msg.Type)
    assert.WithinDuration(t, time.Now(), msg.Timestamp, time.Second)
    assert.NotNil(t, msg.Payload)
    
    // Test payload parsing
    var parsedPayload map[string]interface{}
    err = msg.ParsePayload(&parsedPayload)
    require.NoError(t, err)
    assert.Equal(t, payload, parsedPayload)
}

func TestMessage_ParsePayload(t *testing.T) {
    testCases := []struct {
        name        string
        payload     interface{}
        parseTarget interface{}
        expectError bool
    }{
        {
            name:        "valid object",
            payload:     map[string]interface{}{"key": "value"},
            parseTarget: &map[string]interface{}{},
            expectError: false,
        },
        {
            name:        "valid array",
            payload:     []string{"item1", "item2"},
            parseTarget: &[]string{},
            expectError: false,
        },
        {
            name:        "invalid target type",
            payload:     map[string]interface{}{"key": "value"},
            parseTarget: &[]string{},
            expectError: true,
        },
    }
    
    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            msg, err := NewMessage("test", tc.payload)
            require.NoError(t, err)
            
            err = msg.ParsePayload(tc.parseTarget)
            
            if tc.expectError {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}

func TestGenerateID(t *testing.T) {
    // Generate multiple IDs
    ids := make(map[string]bool)
    const numIDs = 1000
    
    for i := 0; i < numIDs; i++ {
        id := GenerateID()
        
        // Check format (UUID)
        assert.Len(t, id, 36)
        assert.Contains(t, id, "-")
        
        // Check uniqueness
        assert.False(t, ids[id], "ID %s should be unique", id)
        ids[id] = true
    }
}

func TestRandString(t *testing.T) {
    testCases := []int{1, 5, 10, 50, 100}
    
    for _, length := range testCases {
        t.Run(fmt.Sprintf("length_%d", length), func(t *testing.T) {
            str := RandString(length)
            assert.Len(t, str, length)
            
            // Check character set
            for _, char := range str {
                assert.True(t, 
                    (char >= 'a' && char <= 'z') ||
                    (char >= 'A' && char <= 'Z') ||
                    (char >= '0' && char <= '9'),
                    "Character %c should be alphanumeric", char)
            }
        })
    }
}
```

## Integration Testing Strategy

### 1. End-to-End Message Flow Testing

**Test Implementation:**
```go
// test/integration/message_flow_test.go
package integration

import (
    "context"
    "encoding/json"
    "net/http"
    "testing"
    "time"
    
    "github.com/gorilla/websocket"
    "github.com/mark3labs/mcp-go/client"
    "github.com/mark3labs/mcp-go/mcp"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestCompleteMessageFlow(t *testing.T) {
    // Start test server
    server := startTestServer(t)
    defer server.Close()
    
    // Connect WebSocket client
    ws, err := connectWebSocketClient(server.WebSocketURL())
    require.NoError(t, err)
    defer ws.Close()
    
    // Connect MCP client
    mcpClient := client.NewHTTPClient(server.MCPUrl())
    err = mcpClient.Initialize(context.Background())
    require.NoError(t, err)
    
    // Test complete flow
    testQuestion := "What is your favorite programming language?"
    testAnswer := "Go"
    
    // Start MCP request in goroutine
    var mcpResult *mcp.CallToolResult
    var mcpError error
    
    go func() {
        request := mcp.CallToolRequest{
            Name: "ask_user",
            Arguments: map[string]interface{}{
                "question": testQuestion,
                "timeout_seconds": 10,
            },
        }
        
        mcpResult, mcpError = mcpClient.CallTool(context.Background(), request)
    }()
    
    // Wait for WebSocket message
    var wsMessage map[string]interface{}
    err = ws.ReadJSON(&wsMessage)
    require.NoError(t, err)
    
    // Verify WebSocket message
    assert.Equal(t, "ask_user", wsMessage["type"])
    assert.Equal(t, testQuestion, wsMessage["question"])
    assert.NotEmpty(t, wsMessage["request_id"])
    
    requestID := wsMessage["request_id"].(string)
    
    // Send response via WebSocket
    response := map[string]interface{}{
        "type":       "user_response",
        "request_id": requestID,
        "answer":     testAnswer,
    }
    
    err = ws.WriteJSON(response)
    require.NoError(t, err)
    
    // Wait for MCP response
    time.Sleep(100 * time.Millisecond)
    
    // Verify MCP result
    require.NoError(t, mcpError)
    require.NotNil(t, mcpResult)
    assert.Equal(t, testAnswer, mcpResult.Content[0].Text)
}

func TestMultipleClientsFlow(t *testing.T) {
    // Start test server
    server := startTestServer(t)
    defer server.Close()
    
    // Connect multiple WebSocket clients
    const numClients = 3
    clients := make([]*websocket.Conn, numClients)
    
    for i := 0; i < numClients; i++ {
        ws, err := connectWebSocketClient(server.WebSocketURL())
        require.NoError(t, err)
        defer ws.Close()
        
        clients[i] = ws
    }
    
    // Connect MCP client
    mcpClient := client.NewHTTPClient(server.MCPUrl())
    err := mcpClient.Initialize(context.Background())
    require.NoError(t, err)
    
    // Send question via MCP
    testQuestion := "Choose a number from 1 to 10"
    
    go func() {
        request := mcp.CallToolRequest{
            Name: "ask_user",
            Arguments: map[string]interface{}{
                "question": testQuestion,
                "timeout_seconds": 10,
            },
        }
        
        mcpClient.CallTool(context.Background(), request)
    }()
    
    // Verify all clients receive the message
    for i, client := range clients {
        var wsMessage map[string]interface{}
        err = client.ReadJSON(&wsMessage)
        require.NoError(t, err, "Client %d should receive message", i)
        
        assert.Equal(t, "ask_user", wsMessage["type"])
        assert.Equal(t, testQuestion, wsMessage["question"])
    }
    
    // First client responds
    requestID := "" // Extract from first client's message
    response := map[string]interface{}{
        "type":       "user_response",
        "request_id": requestID,
        "answer":     "7",
    }
    
    err = clients[0].WriteJSON(response)
    require.NoError(t, err)
    
    // Verify MCP receives the response
    time.Sleep(100 * time.Millisecond)
}
```

### 2. Database Integration Testing

**Test Implementation:**
```go
// test/integration/database_test.go
package integration

import (
    "context"
    "database/sql"
    "testing"
    
    _ "github.com/lib/pq"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestDatabaseIntegration(t *testing.T) {
    // Setup test database
    db, err := sql.Open("postgres", "postgres://test:test@localhost/test_db?sslmode=disable")
    require.NoError(t, err)
    defer db.Close()
    
    // Run migrations
    err = runMigrations(db)
    require.NoError(t, err)
    
    // Test conversation storage
    conversationID := "test-conversation-123"
    userID := "user-456"
    question := "What is your favorite color?"
    answer := "Blue"
    
    // Insert conversation
    _, err = db.Exec(`
        INSERT INTO conversations (id, user_id, question, answer, created_at)
        VALUES ($1, $2, $3, $4, NOW())
    `, conversationID, userID, question, answer)
    require.NoError(t, err)
    
    // Retrieve conversation
    var retrieved struct {
        ID       string
        UserID   string
        Question string
        Answer   string
    }
    
    err = db.QueryRow(`
        SELECT id, user_id, question, answer
        FROM conversations
        WHERE id = $1
    `, conversationID).Scan(&retrieved.ID, &retrieved.UserID, &retrieved.Question, &retrieved.Answer)
    require.NoError(t, err)
    
    // Verify data
    assert.Equal(t, conversationID, retrieved.ID)
    assert.Equal(t, userID, retrieved.UserID)
    assert.Equal(t, question, retrieved.Question)
    assert.Equal(t, answer, retrieved.Answer)
}

func TestRedisIntegration(t *testing.T) {
    // Setup Redis client
    client := redis.NewClient(&redis.Options{
        Addr: "localhost:6379",
        DB:   1, // Use test database
    })
    defer client.Close()
    
    // Test session storage
    sessionID := "session-123"
    sessionData := map[string]interface{}{
        "user_id": "user-456",
        "role":    "user",
        "created": time.Now().Unix(),
    }
    
    // Store session
    data, err := json.Marshal(sessionData)
    require.NoError(t, err)
    
    err = client.Set(context.Background(), sessionID, data, time.Hour).Err()
    require.NoError(t, err)
    
    // Retrieve session
    retrieved, err := client.Get(context.Background(), sessionID).Result()
    require.NoError(t, err)
    
    var retrievedData map[string]interface{}
    err = json.Unmarshal([]byte(retrieved), &retrievedData)
    require.NoError(t, err)
    
    // Verify data
    assert.Equal(t, sessionData["user_id"], retrievedData["user_id"])
    assert.Equal(t, sessionData["role"], retrievedData["role"])
}
```

## Load Testing Strategy

### 1. WebSocket Load Testing

**Test Implementation:**
```go
// test/load/websocket_load_test.go
package load

import (
    "context"
    "fmt"
    "log"
    "sync"
    "testing"
    "time"
    
    "github.com/gorilla/websocket"
    "github.com/stretchr/testify/assert"
)

func TestWebSocketLoadTest(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping load test in short mode")
    }
    
    const (
        numClients      = 1000
        messagesPerClient = 100
        testDuration    = 5 * time.Minute
    )
    
    // Metrics
    var (
        successfulConnections int64
        failedConnections    int64
        messagesSent         int64
        messagesReceived     int64
        errors               int64
    )
    
    var wg sync.WaitGroup
    ctx, cancel := context.WithTimeout(context.Background(), testDuration)
    defer cancel()
    
    // Start clients
    for i := 0; i < numClients; i++ {
        wg.Add(1)
        go func(clientID int) {
            defer wg.Done()
            
            err := runLoadTestClient(ctx, clientID, messagesPerClient, &messagesSent, &messagesReceived, &errors)
            if err != nil {
                atomic.AddInt64(&failedConnections, 1)
                log.Printf("Client %d failed: %v", clientID, err)
            } else {
                atomic.AddInt64(&successfulConnections, 1)
            }
        }(i)
    }
    
    // Wait for completion
    wg.Wait()
    
    // Report results
    t.Logf("Load test results:")
    t.Logf("  Successful connections: %d", successfulConnections)
    t.Logf("  Failed connections: %d", failedConnections)
    t.Logf("  Messages sent: %d", messagesSent)
    t.Logf("  Messages received: %d", messagesReceived)
    t.Logf("  Errors: %d", errors)
    
    // Assertions
    assert.Greater(t, successfulConnections, int64(numClients*0.95)) // 95% success rate
    assert.Less(t, failedConnections, int64(numClients*0.05))       // <5% failure rate
    assert.Greater(t, messagesSent, int64(numClients*messagesPerClient*0.9)) // 90% messages sent
}

func runLoadTestClient(ctx context.Context, clientID int, numMessages int, 
                      messagesSent, messagesReceived, errors *int64) error {
    
    // Connect to WebSocket
    conn, _, err := websocket.DefaultDialer.Dial("ws://localhost:8080/ws", nil)
    if err != nil {
        return err
    }
    defer conn.Close()
    
    // Send connect message
    connectMsg := map[string]interface{}{
        "type": "connect",
    }
    
    if err := conn.WriteJSON(connectMsg); err != nil {
        return err
    }
    
    // Start message receiver
    go func() {
        for {
            select {
            case <-ctx.Done():
                return
            default:
                var msg map[string]interface{}
                if err := conn.ReadJSON(&msg); err != nil {
                    atomic.AddInt64(errors, 1)
                    return
                }
                atomic.AddInt64(messagesReceived, 1)
            }
        }
    }()
    
    // Send messages
    ticker := time.NewTicker(100 * time.Millisecond)
    defer ticker.Stop()
    
    messageCount := 0
    for {
        select {
        case <-ctx.Done():
            return nil
        case <-ticker.C:
            if messageCount >= numMessages {
                return nil
            }
            
            msg := map[string]interface{}{
                "type":    "test_message",
                "client":  clientID,
                "message": messageCount,
                "data":    fmt.Sprintf("Test message %d from client %d", messageCount, clientID),
            }
            
            if err := conn.WriteJSON(msg); err != nil {
                atomic.AddInt64(errors, 1)
                return err
            }
            
            atomic.AddInt64(messagesSent, 1)
            messageCount++
        }
    }
}
```

### 2. MCP Load Testing

**Test Implementation:**
```go
// test/load/mcp_load_test.go
package load

import (
    "context"
    "sync"
    "testing"
    "time"
    
    "github.com/mark3labs/mcp-go/client"
    "github.com/mark3labs/mcp-go/mcp"
    "github.com/stretchr/testify/assert"
)

func TestMCPLoadTest(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping load test in short mode")
    }
    
    const (
        numClients = 100
        requestsPerClient = 10
        testDuration = 2 * time.Minute
    )
    
    var (
        successfulRequests int64
        failedRequests     int64
        totalLatency       time.Duration
    )
    
    var wg sync.WaitGroup
    ctx, cancel := context.WithTimeout(context.Background(), testDuration)
    defer cancel()
    
    // Start MCP clients
    for i := 0; i < numClients; i++ {
        wg.Add(1)
        go func(clientID int) {
            defer wg.Done()
            
            mcpClient := client.NewHTTPClient("http://localhost:8081")
            if err := mcpClient.Initialize(ctx); err != nil {
                atomic.AddInt64(&failedRequests, int64(requestsPerClient))
                return
            }
            
            for j := 0; j < requestsPerClient; j++ {
                start := time.Now()
                
                request := mcp.CallToolRequest{
                    Name: "ask_user",
                    Arguments: map[string]interface{}{
                        "question": fmt.Sprintf("Load test question %d from client %d", j, clientID),
                        "timeout_seconds": 1, // Short timeout for load testing
                    },
                }
                
                _, err := mcpClient.CallTool(ctx, request)
                latency := time.Since(start)
                
                if err != nil {
                    atomic.AddInt64(&failedRequests, 1)
                } else {
                    atomic.AddInt64(&successfulRequests, 1)
                }
                
                // Add to total latency (thread-safe addition)
                atomic.AddInt64((*int64)(&totalLatency), int64(latency))
            }
        }(i)
    }
    
    wg.Wait()
    
    // Calculate metrics
    totalRequests := successfulRequests + failedRequests
    successRate := float64(successfulRequests) / float64(totalRequests) * 100
    avgLatency := time.Duration(int64(totalLatency) / successfulRequests)
    
    // Report results
    t.Logf("MCP load test results:")
    t.Logf("  Total requests: %d", totalRequests)
    t.Logf("  Successful requests: %d (%.2f%%)", successfulRequests, successRate)
    t.Logf("  Failed requests: %d", failedRequests)
    t.Logf("  Average latency: %v", avgLatency)
    
    // Assertions
    assert.Greater(t, successRate, 95.0) // 95% success rate
    assert.Less(t, avgLatency, 100*time.Millisecond) // <100ms average latency
}
```

## Security Testing Strategy

### 1. Authentication Testing

**Test Implementation:**
```go
// test/security/auth_test.go
package security

import (
    "testing"
    "time"
    
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestJWTAuthentication(t *testing.T) {
    // Setup JWT manager
    jwtManager := NewJWTManager("test-secret", time.Hour)
    
    testCases := []struct {
        name        string
        userID      string
        role        string
        expectError bool
    }{
        {
            name:        "valid user token",
            userID:      "user-123",
            role:        "user",
            expectError: false,
        },
        {
            name:        "valid admin token",
            userID:      "admin-456",
            role:        "admin",
            expectError: false,
        },
        {
            name:        "empty user ID",
            userID:      "",
            role:        "user",
            expectError: true,
        },
        {
            name:        "invalid role",
            userID:      "user-123",
            role:        "invalid",
            expectError: true,
        },
    }
    
    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            // Generate token
            token, err := jwtManager.Generate(tc.userID, tc.role)
            
            if tc.expectError {
                assert.Error(t, err)
                return
            }
            
            require.NoError(t, err)
            assert.NotEmpty(t, token)
            
            // Validate token
            claims, err := jwtManager.Validate(token)
            require.NoError(t, err)
            
            assert.Equal(t, tc.userID, claims.UserID)
            assert.Equal(t, tc.role, claims.Role)
        })
    }
}

func TestTokenExpiration(t *testing.T) {
    // Setup JWT manager with short expiry
    jwtManager := NewJWTManager("test-secret", time.Millisecond)
    
    // Generate token
    token, err := jwtManager.Generate("user-123", "user")
    require.NoError(t, err)
    
    // Wait for expiration
    time.Sleep(10 * time.Millisecond)
    
    // Validate expired token
    _, err = jwtManager.Validate(token)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "token is expired")
}

func TestInvalidTokens(t *testing.T) {
    jwtManager := NewJWTManager("test-secret", time.Hour)
    
    testCases := []struct {
        name  string
        token string
    }{
        {
            name:  "malformed token",
            token: "invalid.token.format",
        },
        {
            name:  "empty token",
            token: "",
        },
        {
            name:  "random string",
            token: "random-string-not-jwt",
        },
    }
    
    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            _, err := jwtManager.Validate(tc.token)
            assert.Error(t, err)
        })
    }
}
```

### 2. Input Validation Testing

**Test Implementation:**
```go
// test/security/validation_test.go
package security

import (
    "testing"
    
    "github.com/stretchr/testify/assert"
)

func TestInputValidation(t *testing.T) {
    validator := NewValidator()
    
    testCases := []struct {
        name        string
        input       string
        expectError bool
        expected    string
    }{
        {
            name:        "normal text",
            input:       "What is your name?",
            expectError: false,
            expected:    "What is your name?",
        },
        {
            name:        "text with HTML",
            input:       "Hello <script>alert('xss')</script>",
            expectError: false,
            expected:    "Hello ",
        },
        {
            name:        "text with SQL injection",
            input:       "'; DROP TABLE users; --",
            expectError: false,
            expected:    "'; DROP TABLE users; --", // SQL injection should be handled at query level
        },
        {
            name:        "very long text",
            input:       strings.Repeat("a", 10000),
            expectError: true,
            expected:    "",
        },
        {
            name:        "empty input",
            input:       "",
            expectError: true,
            expected:    "",
        },
    }
    
    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            result, err := validator.ValidateAndSanitize(tc.input)
            
            if tc.expectError {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
                assert.Equal(t, tc.expected, result)
            }
        })
    }
}

func TestRateLimiting(t *testing.T) {
    rateLimiter := NewRateLimiter(5, time.Minute) // 5 requests per minute
    
    clientID := "test-client"
    
    // First 5 requests should succeed
    for i := 0; i < 5; i++ {
        allowed := rateLimiter.Allow(clientID)
        assert.True(t, allowed, "Request %d should be allowed", i+1)
    }
    
    // 6th request should be denied
    allowed := rateLimiter.Allow(clientID)
    assert.False(t, allowed, "6th request should be denied")
    
    // Wait for window to reset
    time.Sleep(61 * time.Second)
    
    // Request should be allowed again
    allowed = rateLimiter.Allow(clientID)
    assert.True(t, allowed, "Request after window reset should be allowed")
}
```

## Performance Testing Strategy

### 1. Benchmark Testing

**Test Implementation:**
```go
// test/performance/benchmark_test.go
package performance

import (
    "testing"
    
    "github.com/jcronq/frontend_router_mcp/internal/tools"
    "github.com/jcronq/frontend_router_mcp/pkg/messaging"
)

func BenchmarkAskUserTool_Execute(b *testing.B) {
    handler := func(requestID, question string) error {
        return nil
    }
    
    tool := tools.NewAskUserTool(handler)
    
    params := map[string]interface{}{
        "question": "What is your name?",
        "timeout_seconds": 1,
    }
    
    paramsJSON, _ := json.Marshal(params)
    
    b.ResetTimer()
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
            tool.Execute(ctx, paramsJSON)
            cancel()
        }
    })
}

func BenchmarkMessageSerialization(b *testing.B) {
    payload := map[string]interface{}{
        "question": "What is your favorite programming language?",
        "timeout":  300,
        "metadata": map[string]interface{}{
            "source": "benchmark",
            "timestamp": time.Now().Unix(),
        },
    }
    
    b.ResetTimer()
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            _, err := messaging.NewMessage("ask_user", payload)
            if err != nil {
                b.Fatal(err)
            }
        }
    })
}

func BenchmarkWebSocketMessageProcessing(b *testing.B) {
    // Setup test server
    messageCh := make(chan *messaging.Message, 1000)
    server := NewServer(messageCh)
    
    // Create test message
    msg := &messaging.Message{
        ID:        "test-message",
        Type:      "test",
        Payload:   []byte(`{"content": "test message"}`),
        Timestamp: time.Now(),
    }
    
    b.ResetTimer()
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            server.broadcast <- msg
        }
    })
}
```

### 2. Memory Usage Testing

**Test Implementation:**
```go
// test/performance/memory_test.go
package performance

import (
    "runtime"
    "testing"
    
    "github.com/stretchr/testify/assert"
)

func TestMemoryUsage(t *testing.T) {
    // Baseline memory usage
    runtime.GC()
    var baseline runtime.MemStats
    runtime.ReadMemStats(&baseline)
    
    // Create many connections
    const numConnections = 1000
    connections := make([]*Connection, numConnections)
    
    for i := 0; i < numConnections; i++ {
        connections[i] = &Connection{
            ID:       fmt.Sprintf("conn-%d", i),
            Messages: make(chan *Message, 100),
        }
    }
    
    // Measure memory usage
    runtime.GC()
    var afterCreate runtime.MemStats
    runtime.ReadMemStats(&afterCreate)
    
    memoryPerConnection := (afterCreate.Alloc - baseline.Alloc) / numConnections
    
    t.Logf("Memory usage per connection: %d bytes", memoryPerConnection)
    
    // Assert memory usage is reasonable
    assert.Less(t, memoryPerConnection, uint64(10*1024), "Memory per connection should be less than 10KB")
    
    // Cleanup
    for i := 0; i < numConnections; i++ {
        close(connections[i].Messages)
    }
    
    runtime.GC()
    var afterCleanup runtime.MemStats
    runtime.ReadMemStats(&afterCleanup)
    
    // Verify memory is released
    assert.Less(t, afterCleanup.Alloc, afterCreate.Alloc, "Memory should be released after cleanup")
}
```

## Test Execution Strategy

### 1. Test Automation

**GitHub Actions Configuration:**
```yaml
# .github/workflows/test.yml
name: Test

on:
  push:
    branches: [ main, develop ]
  pull_request:
    branches: [ main ]

jobs:
  test:
    runs-on: ubuntu-latest
    
    services:
      postgres:
        image: postgres:15
        env:
          POSTGRES_PASSWORD: postgres
          POSTGRES_DB: test_db
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
      
      redis:
        image: redis:7
        options: >-
          --health-cmd "redis-cli ping"
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
    
    steps:
    - uses: actions/checkout@v3
    
    - name: Set up Go
      uses: actions/setup-go@v3
      with:
        go-version: 1.21
    
    - name: Install dependencies
      run: go mod download
    
    - name: Run unit tests
      run: go test -v -race -coverprofile=coverage.out ./...
    
    - name: Run integration tests
      run: go test -v -tags=integration ./test/integration/...
      env:
        DATABASE_URL: postgres://postgres:postgres@localhost:5432/test_db?sslmode=disable
        REDIS_URL: redis://localhost:6379
    
    - name: Upload coverage reports
      uses: codecov/codecov-action@v3
      with:
        file: ./coverage.out
    
    - name: Run security tests
      run: go test -v ./test/security/...
    
    - name: Run performance tests
      run: go test -v -bench=. ./test/performance/...

  load-test:
    runs-on: ubuntu-latest
    if: github.event_name == 'push' && github.ref == 'refs/heads/main'
    
    steps:
    - uses: actions/checkout@v3
    
    - name: Set up Go
      uses: actions/setup-go@v3
      with:
        go-version: 1.21
    
    - name: Build application
      run: go build -o frontend-router-mcp ./cmd/server
    
    - name: Start application
      run: |
        ./frontend-router-mcp &
        sleep 5
    
    - name: Run load tests
      run: go test -v -timeout=10m ./test/load/...
      env:
        LOAD_TEST_DURATION: 5m
        LOAD_TEST_CLIENTS: 1000
```

### 2. Test Coverage Requirements

**Coverage Targets:**
- **Unit Tests**: 90% line coverage
- **Integration Tests**: 80% critical path coverage
- **End-to-End Tests**: 100% happy path coverage

**Coverage Reporting:**
```bash
# Generate coverage report
go test -coverprofile=coverage.out ./...

# View coverage in browser
go tool cover -html=coverage.out

# Check coverage percentage
go tool cover -func=coverage.out | grep total
```

### 3. Test Environment Management

**Test Configuration:**
```yaml
# test/config/test.yml
test:
  database:
    host: localhost
    port: 5432
    name: test_db
    user: postgres
    password: postgres
    
  redis:
    host: localhost
    port: 6379
    db: 1
    
  server:
    ws_port: 8080
    mcp_port: 8081
    log_level: debug
    
  load_test:
    duration: 5m
    clients: 1000
    messages_per_client: 100
```

This comprehensive testing strategy ensures the Frontend Router MCP service is thoroughly tested across all layers, from individual units to complete system integration, with proper load testing and security validation.