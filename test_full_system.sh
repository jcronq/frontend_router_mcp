#!/bin/bash

# Full system test for Frontend Router MCP
set -e

echo "🔍 Running comprehensive system test..."

# Build the application
echo "🏗️  Building application..."
go build -o frontend-router-mcp ./cmd/server

# Start the server in background
echo "🚀 Starting server..."
./frontend-router-mcp > server_test.log 2>&1 &
SERVER_PID=$!

# Function to cleanup
cleanup() {
    echo "🧹 Cleaning up..."
    kill $SERVER_PID 2>/dev/null || true
    rm -f server_test.log
    exit $1
}

# Set up cleanup on exit
trap 'cleanup 0' EXIT
trap 'cleanup 1' INT TERM

# Wait for server to start
echo "⏳ Waiting for server to start..."
sleep 5

# Test 1: Check if WebSocket server is responding
echo "🔌 Test 1: WebSocket server availability..."
if curl -s --connect-timeout 5 http://localhost:8080/ws | grep -q "Bad Request"; then
    echo "✅ WebSocket server is responding correctly"
else
    echo "❌ WebSocket server test failed"
    echo "Server logs:"
    cat server_test.log
    cleanup 1
fi

# Test 2: Check server logs for startup messages
echo "📋 Test 2: Server startup logs..."
if grep -q "Servers started" server_test.log && grep -q "WebSocket server starting" server_test.log; then
    echo "✅ Server started successfully"
else
    echo "❌ Server startup test failed"
    echo "Server logs:"
    cat server_test.log
    cleanup 1
fi

# Test 3: Test WebSocket connection
echo "🔗 Test 3: WebSocket connection test..."
timeout 10 go run test_client.go > client_test.log 2>&1 &
CLIENT_PID=$!

# Wait for client to run
sleep 3

# Kill the client
kill $CLIENT_PID 2>/dev/null || true

# Check if client connected successfully
if grep -q "recv:" client_test.log; then
    echo "✅ WebSocket connection test passed"
else
    echo "❌ WebSocket connection test failed"
    echo "Client logs:"
    cat client_test.log 2>/dev/null || echo "No client logs"
    echo "Server logs:"
    tail -10 server_test.log
    cleanup 1
fi

# Test 4: Check server logs for client connection
echo "🤝 Test 4: Client connection logging..."
if grep -q "Client connected" server_test.log; then
    echo "✅ Client connection logged successfully"
else
    echo "❌ Client connection logging test failed"
    echo "Server logs:"
    tail -10 server_test.log
    cleanup 1
fi

# Clean up test files
rm -f client_test.log

echo ""
echo "🎉 All tests passed! System is working correctly."
echo ""
echo "📊 Test Summary:"
echo "✅ WebSocket server availability"
echo "✅ Server startup process"
echo "✅ WebSocket connection functionality"
echo "✅ Client connection logging"
echo ""
echo "🎯 To test the full ask_user functionality:"
echo "1. Keep the server running"
echo "2. Open test_frontend.html in a browser"
echo "3. Click 'Connect to WebSocket Server'"
echo "4. In another terminal, run: go run test_mcp.go"
echo "5. Answer the question in the browser"
echo ""
echo "Server is still running in the background (PID: $SERVER_PID)"
echo "Press Ctrl+C to stop the test and cleanup"

# Keep the server running for manual testing
wait $SERVER_PID