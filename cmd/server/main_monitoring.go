package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/jcronq/frontend_router_mcp/internal/health"
	"github.com/jcronq/frontend_router_mcp/internal/logging"
	"github.com/jcronq/frontend_router_mcp/internal/metrics"
	"github.com/jcronq/frontend_router_mcp/internal/middleware"
	"github.com/jcronq/frontend_router_mcp/internal/tools"
	"github.com/jcronq/frontend_router_mcp/internal/wsserver"
	"github.com/jcronq/frontend_router_mcp/pkg/messaging"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const (
	serviceName    = "frontend-router-mcp"
	serviceVersion = "1.0.0"
)

// userQuestion represents a question from an agent to a user
type userQuestion struct {
	RequestID string
	Question  string
}

// userResponse represents a response from a user to an agent
type userResponse struct {
	RequestID string
	Answer    string
}

// channelWrapper wraps a channel to provide Len/Cap methods for health checking
type channelWrapper struct {
	ch chan *messaging.Message
}

func (cw *channelWrapper) Len() int { return len(cw.ch) }
func (cw *channelWrapper) Cap() int { return cap(cw.ch) }

// MonitoredMCPServer wraps the MCP server with monitoring
type MonitoredMCPServer struct {
	server  *server.StreamableHTTPServer
	running bool
	mu      sync.RWMutex
}

func (m *MonitoredMCPServer) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.running
}

func (m *MonitoredMCPServer) Start(addr string) error {
	m.mu.Lock()
	m.running = true
	m.mu.Unlock()
	
	return m.server.Start(addr)
}

func (m *MonitoredMCPServer) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if m.server != nil {
		err := m.server.Shutdown(ctx)
		m.running = false
		return err
	}
	return nil
}

func main() {
	// Parse command line flags
	wsPort := flag.Int("ws-port", 8080, "WebSocket server port")
	mcpPort := flag.Int("mcp-port", 8081, "MCP server port")
	monitoringPort := flag.Int("monitoring-port", 8082, "Monitoring endpoints port")
	logLevel := flag.String("log-level", "info", "Log level (debug, info, warn, error, fatal)")
	flag.Parse()

	// Set up logging
	var level logging.LogLevel
	switch *logLevel {
	case "debug":
		level = logging.DEBUG
	case "info":
		level = logging.INFO
	case "warn":
		level = logging.WARN
	case "error":
		level = logging.ERROR
	case "fatal":
		level = logging.FATAL
	default:
		level = logging.INFO
	}

	logger := logging.NewLogger(logging.Config{
		Service:   serviceName,
		Component: "main",
		Level:     level,
	})

	// Set up metrics
	metricsInstance := metrics.NewMetrics()

	// Set up health manager
	healthManager := health.NewManager(serviceVersion)

	// Create a context that can be canceled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create channels for communication
	messageCh := make(chan *messaging.Message, 100)
	questionCh := make(chan userQuestion, 10)
	responseCh := make(chan userResponse, 10)

	// Create a wait group to coordinate shutdown
	wg := sync.WaitGroup{}

	// Initialize WebSocket server
	wsServer := wsserver.NewServer(messageCh)
	
	// Register WebSocket server health check
	healthManager.RegisterComponent("websocket_server", health.NewWebSocketHealthChecker(wsServer))

	// Start WebSocket server
	wg.Add(1)
	go func() {
		defer wg.Done()
		addr := fmt.Sprintf(":%d", *wsPort)
		logger.WithField("port", *wsPort).Info("Starting WebSocket server")
		if err := wsServer.Start(addr); err != nil && err != http.ErrServerClosed {
			logger.WithError(err).Error("WebSocket server error")
		}
	}()

	// Create the ask_user tool with a question handler
	askUserTool := tools.NewAskUserTool(func(requestID, question string) error {
		questionCh <- userQuestion{RequestID: requestID, Question: question}
		return nil
	})

	// Initialize MCP server
	mcpServer := server.NewMCPServer(
		serviceName,
		serviceVersion,
		server.WithToolCapabilities(true),
	)

	// Create and register the ask_user tool
	mcpTool := mcp.NewTool("ask_user",
		mcp.WithDescription("Ask a question to the user and receive their response"),
		mcp.WithString("question",
			mcp.Required(),
			mcp.Description("The question to ask the user"),
		),
		mcp.WithNumber("timeout_seconds",
			mcp.Description("Maximum time to wait for a response in seconds (default: 300)"),
		),
	)

	// Register the tool with the MCP server
	mcpServer.AddTool(mcpTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		requestLogger := logging.FromContext(ctx)
		
		// Extract arguments from the request
		question, err := request.RequireString("question")
		if err != nil {
			metricsInstance.RecordMCPToolExecution("ask_user", "error")
			requestLogger.WithError(err).Error("Failed to extract question from request")
			return mcp.NewToolResultError(err.Error()), nil
		}
		
		timeoutSeconds := 300.0
		
		// Create a JSON representation of the parameters for our AskUserTool
		paramsObj := map[string]interface{}{
			"question":        question,
			"timeout_seconds": timeoutSeconds,
		}
		
		paramsJSON, err := json.Marshal(paramsObj)
		if err != nil {
			metricsInstance.RecordMCPToolExecution("ask_user", "error")
			requestLogger.WithError(err).Error("Failed to marshal parameters")
			return mcp.NewToolResultError(fmt.Sprintf("failed to marshal parameters: %v", err)), nil
		}

		// Execute the tool
		result, err := askUserTool.Execute(ctx, paramsJSON)
		if err != nil {
			metricsInstance.RecordMCPToolExecution("ask_user", "error")
			requestLogger.WithError(err).Error("Tool execution failed")
			return mcp.NewToolResultError(err.Error()), nil
		}

		// Record successful execution
		metricsInstance.RecordMCPToolExecution("ask_user", "success")
		requestLogger.WithField("duration", time.Since(start)).Info("Tool execution completed")

		// Return the result as text
		if resultMap, ok := result.(map[string]interface{}); ok {
			if answer, ok := resultMap["answer"].(string); ok {
				return mcp.NewToolResultText(answer), nil
			}
		}

		return mcp.NewToolResultError("unexpected result format"), nil
	})

	// Start MCP server with monitoring
	monitoredMCPServer := &MonitoredMCPServer{
		server: server.NewStreamableHTTPServer(mcpServer),
	}
	
	// Register MCP server health check
	healthManager.RegisterComponent("mcp_server", health.NewMCPHealthChecker(monitoredMCPServer))

	wg.Add(1)
	go func() {
		defer wg.Done()
		logger.WithField("port", *mcpPort).Info("Starting MCP server")
		
		if err := monitoredMCPServer.Start(fmt.Sprintf(":%d", *mcpPort)); err != nil {
			logger.WithError(err).Fatal("MCP server error")
		}
	}()

	// Register channel health check
	channelChecker := health.NewChannelHealthChecker()
	channelChecker.RegisterChannel("message_channel", &channelWrapper{ch: messageCh})
	healthManager.RegisterComponent("communication_channels", channelChecker)

	// Start monitoring server
	wg.Add(1)
	go func() {
		defer wg.Done()
		startMonitoringServer(*monitoringPort, logger, metricsInstance, healthManager)
	}()

	// Start system metrics collection
	systemInfo := metrics.SystemInfo{
		Version:      serviceVersion,
		GoVersion:    runtime.Version(),
		OS:           runtime.GOOS,
		Architecture: runtime.GOARCH,
		Goroutines:   runtime.NumGoroutine(),
	}
	
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	systemInfo.MemoryUsage = int64(memStats.Alloc)
	
	metricsInstance.StartMetricsCollector(30*time.Second, systemInfo)

	// Start WebSocket to MCP bridge
	wg.Add(1)
	go func() {
		defer wg.Done()
		handleWebSocketToMCP(ctx, messageCh, questionCh, responseCh, logger, metricsInstance)
	}()

	// Start the user response handler
	wg.Add(1)
	go func() {
		defer wg.Done()
		handleUserResponses(ctx, responseCh, askUserTool, logger, metricsInstance)
	}()

	logger.WithFields(map[string]interface{}{
		"websocket_port":  *wsPort,
		"mcp_port":        *mcpPort,
		"monitoring_port": *monitoringPort,
	}).Info("All servers started successfully")

	// Wait for interrupt signal to gracefully shut down
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	logger.Info("Shutting down servers...")
	
	// Create a timeout context for shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	
	// First cancel the main context to stop all operations
	cancel()
	
	// Explicitly shut down the MCP server
	if err := monitoredMCPServer.Shutdown(shutdownCtx); err != nil {
		logger.WithError(err).Error("Error shutting down MCP server")
	}
	
	// Explicitly shut down the WebSocket server
	if err := wsServer.Shutdown(shutdownCtx); err != nil {
		logger.WithError(err).Error("Error shutting down WebSocket server")
	}
	
	// Wait for all goroutines to finish
	wg.Wait()
	logger.Info("All servers shut down successfully")
}

// startMonitoringServer starts the monitoring endpoints server
func startMonitoringServer(port int, logger *logging.Logger, metricsInstance *metrics.Metrics, healthManager *health.Manager) {
	mux := http.NewServeMux()
	
	// Health endpoints
	mux.Handle("/health", healthManager.HTTPHandler())
	mux.Handle("/ready", healthManager.HTTPHandler())
	mux.Handle("/live", healthManager.HTTPHandler())
	
	// Metrics endpoint
	mux.Handle("/metrics", metricsInstance.Handler())
	
	// Apply middleware
	handler := middleware.ChainMiddleware(
		middleware.SecurityMiddleware,
		middleware.CORSMiddleware,
		middleware.RequestIDMiddleware,
		middleware.TracingMiddleware,
		middleware.LoggingMiddleware(logger),
		middleware.MetricsMiddleware(metricsInstance),
		middleware.RecoveryMiddleware(logger),
	)(mux)
	
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: handler,
	}
	
	logger.WithField("port", port).Info("Starting monitoring server")
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.WithError(err).Error("Monitoring server error")
	}
}

// handleWebSocketToMCP bridges WebSocket messages to MCP server and handles user questions
func handleWebSocketToMCP(ctx context.Context, messageCh <-chan *messaging.Message, questionCh <-chan userQuestion, responseCh chan<- userResponse, logger *logging.Logger, metricsInstance *metrics.Metrics) {
	bridgeLogger := logger.WithComponent("websocket-mcp-bridge")
	
	// Create a map to track client connections by ID
	clients := make(map[string]messaging.ResponseSender)

	// Create a map to track active questions
	activeQuestions := make(map[string]string) // requestID -> clientID

	for {
		select {
		case <-ctx.Done():
			return

		case question, ok := <-questionCh:
			if !ok {
				return
			}

			bridgeLogger.WithFields(map[string]interface{}{
				"request_id": question.RequestID,
				"question":   question.Question,
			}).Info("Received question for user")

			// Record user question metric
			metricsInstance.UserQuestionsTotal.WithLabelValues("sent").Inc()

			// If no clients are connected, send an error response
			if len(clients) == 0 {
				bridgeLogger.Warn("No clients connected to answer question")
				responseCh <- userResponse{
					RequestID: question.RequestID,
					Answer:    "Error: No clients connected to answer the question",
				}
				metricsInstance.UserQuestionsTotal.WithLabelValues("no_clients").Inc()
				continue
			}

			// Send to all clients
			for clientID, sender := range clients {
				// Track which client received this question
				activeQuestions[question.RequestID] = clientID

				// Create the message payload
				payload, err := json.Marshal(map[string]interface{}{
					"request_id": question.RequestID,
					"question":   question.Question,
				})
				if err != nil {
					bridgeLogger.WithError(err).Error("Error marshaling question")
					continue
				}

				// Send the message
				msg := &messaging.Message{
					ID:        messaging.GenerateID(),
					Type:      "ask_user",
					Timestamp: time.Now().UTC(),
					Payload:   payload,
				}

				if err := sender.SendResponse(msg); err != nil {
					bridgeLogger.WithError(err).WithField("client_id", clientID).Error("Error sending question to client")
				} else {
					metricsInstance.RecordWebSocketMessage("outbound", "ask_user", len(payload))
				}
			}

		case msg, ok := <-messageCh:
			if !ok || msg == nil {
				return
			}

			bridgeLogger.WithFields(map[string]interface{}{
				"message_type": msg.Type,
				"client_id":    msg.ClientID,
				"message_id":   msg.ID,
			}).Info("Received WebSocket message")

			// Record WebSocket message metric
			metricsInstance.RecordWebSocketMessage("inbound", msg.Type, len(msg.Payload))

			// Store the client's response sender if available
			if msg.ResponseSender != nil {
				clients[msg.ClientID] = msg.ResponseSender
			}

			switch msg.Type {
			case "user_response":
				// Handle user response to a question
				var response struct {
					RequestID string `json:"request_id"`
					Answer    string `json:"answer"`
				}

				if err := json.Unmarshal(msg.Payload, &response); err != nil {
					bridgeLogger.WithError(err).Error("Error unmarshaling user response")
					continue
				}

				// Check if this is a response to an active question
				if _, exists := activeQuestions[response.RequestID]; exists {
					bridgeLogger.WithFields(map[string]interface{}{
						"client_id":  msg.ClientID,
						"request_id": response.RequestID,
					}).Info("Received user response")

					responseCh <- userResponse{
						RequestID: response.RequestID,
						Answer:    response.Answer,
					}

					// Remove from active questions
					delete(activeQuestions, response.RequestID)
					metricsInstance.UserQuestionsTotal.WithLabelValues("answered").Inc()
				} else {
					bridgeLogger.WithField("request_id", response.RequestID).Warn("Received response for unknown request ID")
				}

			case "connect":
				// New client connected
				bridgeLogger.WithField("client_id", msg.ClientID).Info("Client connected")
				metricsInstance.SetActiveUserSessions(len(clients))

				// Send acknowledgement
				if sender := clients[msg.ClientID]; sender != nil {
					payload, _ := json.Marshal(map[string]interface{}{
						"status": "connected",
					})

					responseMsg := &messaging.Message{
						ID:        messaging.GenerateID(),
						Type:      "connect_ack",
						Timestamp: time.Now().UTC(),
						Payload:   payload,
					}

					sender.SendResponse(responseMsg)
				}

			case "disconnect":
				// Client disconnected
				bridgeLogger.WithField("client_id", msg.ClientID).Info("Client disconnected")
				delete(clients, msg.ClientID)
				metricsInstance.SetActiveUserSessions(len(clients))

			default:
				bridgeLogger.WithField("message_type", msg.Type).Warn("Unknown message type")
			}
		}
	}
}

// handleUserResponses forwards user responses to the ask_user tool
func handleUserResponses(ctx context.Context, responseCh <-chan userResponse, askUserTool *tools.AskUserTool, logger *logging.Logger, metricsInstance *metrics.Metrics) {
	responseLogger := logger.WithComponent("user-response-handler")
	
	for {
		select {
		case <-ctx.Done():
			return
		case response, ok := <-responseCh:
			if !ok {
				return
			}

			start := time.Now()
			responseLogger.WithField("request_id", response.RequestID).Info("Forwarding user response to ask_user tool")
			
			if err := askUserTool.HandleUserResponse(response.RequestID, response.Answer); err != nil {
				responseLogger.WithError(err).WithField("request_id", response.RequestID).Error("Error handling user response")
				metricsInstance.RecordUserQuestion("error", time.Since(start))
			} else {
				metricsInstance.RecordUserQuestion("success", time.Since(start))
			}
		}
	}
}