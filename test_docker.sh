#!/bin/bash

# Test script for Docker deployment
set -e

echo "🚀 Testing Frontend Router MCP Docker deployment..."

# Clean up any existing containers
echo "🧹 Cleaning up existing containers..."
docker-compose down 2>/dev/null || true
docker rmi frontend-router-mcp 2>/dev/null || true

# Build the Docker image
echo "🏗️  Building Docker image..."
docker build -t frontend-router-mcp .

# Start the application with Docker Compose
echo "🚀 Starting application with Docker Compose..."
docker-compose up -d

# Wait for services to be ready
echo "⏳ Waiting for services to start..."
sleep 10

# Test WebSocket server
echo "🔌 Testing WebSocket server..."
if curl -s --connect-timeout 5 http://localhost:8080/ws | grep -q "Bad Request"; then
    echo "✅ WebSocket server is responding correctly"
else
    echo "❌ WebSocket server is not responding"
    exit 1
fi

# Test MCP server (it should respond to HTTP requests, even if not with tools endpoint)
echo "🔧 Testing MCP server..."
if curl -s --connect-timeout 5 http://localhost:8081/ >/dev/null; then
    echo "✅ MCP server is responding"
else
    echo "❌ MCP server is not responding"
    exit 1
fi

# Show logs
echo "📋 Recent logs:"
docker-compose logs --tail=20

# Show container status
echo "📊 Container status:"
docker-compose ps

echo ""
echo "🎉 Docker deployment test completed successfully!"
echo ""
echo "To test manually:"
echo "1. Open test_frontend.html in a browser"
echo "2. Click 'Connect to WebSocket Server'"
echo "3. Run 'go run test_mcp.go' in another terminal"
echo "4. Answer the question in the browser"
echo ""
echo "To stop the application:"
echo "docker-compose down"