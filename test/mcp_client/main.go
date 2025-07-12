package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"
)

func main() {
	ctx := context.Background()
	
	// Test 1: List available tools
	log.Println("Testing MCP server tool listing...")
	resp, err := http.Get("http://localhost:8081/tools")
	if err != nil {
		log.Printf("Failed to connect to MCP server: %v", err)
		log.Println("Make sure the server is running with: ./frontend-router-mcp")
		return
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Failed to read response: %v", err)
	}
	
	log.Printf("Tools response status: %d", resp.StatusCode)
	log.Printf("Tools response: %s", string(body))
	
	// Test 2: Call ask_user tool
	log.Println("\nTesting ask_user tool execution...")
	
	toolRequest := map[string]interface{}{
		"tool":   "ask_user",
		"params": map[string]interface{}{
			"question":        "What is your favorite color?",
			"timeout_seconds": 5,
		},
	}
	
	jsonData, err := json.Marshal(toolRequest)
	if err != nil {
		log.Fatalf("Failed to marshal request: %v", err)
	}
	
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	
	req, err := http.NewRequestWithContext(ctx, "POST", "http://localhost:8081/execute", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	
	client := &http.Client{}
	resp, err = client.Do(req)
	if err != nil {
		log.Printf("Tool call failed (expected if no user connected): %v", err)
		return
	}
	defer resp.Body.Close()
	
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Failed to read response: %v", err)
	}
	
	log.Printf("Tool execution status: %d", resp.StatusCode)
	log.Printf("Tool execution response: %s", string(body))
	
	if resp.StatusCode == 200 {
		log.Println("✅ MCP ask_user tool executed successfully!")
	} else {
		log.Printf("❌ Tool execution failed with status %d", resp.StatusCode)
	}
}