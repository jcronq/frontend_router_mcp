package wsserver

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jcronq/frontend_router_mcp/internal/logger"
	"github.com/jcronq/frontend_router_mcp/pkg/messaging"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// Client represents a connected WebSocket client
type Client struct {
	ID       string
	conn     *websocket.Conn
	send     chan *messaging.Message
	server   *Server
}

// Server manages WebSocket connections and message routing
type Server struct {
	clients    map[string]*Client
	register   chan *Client
	unregister chan *Client
	broadcast  chan *messaging.Message
	messageCh  chan *messaging.Message
	mu         sync.RWMutex
	httpServer *http.Server
	shutdown   chan struct{}
	logger     *logger.Logger
}

// NewServer creates a new WebSocket server
func NewServer(messageCh chan *messaging.Message) *Server {
	return &Server{
		clients:    make(map[string]*Client),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan *messaging.Message),
		messageCh:  messageCh,
		shutdown:   make(chan struct{}),
		logger:     logger.WithComponent("websocket-server"),
	}
}

// Start starts the WebSocket server
func (s *Server) Start(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.HandleWebSocket)
	
	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: mux,
	}
	
	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.logger.LogPanic(context.Background(), r, "WebSocket server run goroutine panic")
			}
		}()
		s.run()
	}()
	
	s.logger.Info("WebSocket server starting", "address", addr)
	return s.httpServer.ListenAndServe()
}

// HandleWebSocket handles websocket requests from the peer.
func (s *Server) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.LogError(context.Background(), err, "Failed to upgrade connection to WebSocket")
		return
	}

	clientID := generateClientID()
	s.logger.Info("New WebSocket connection established", 
		"client_id", clientID,
		"remote_addr", r.RemoteAddr,
		"user_agent", r.Header.Get("User-Agent"))

	client := &Client{
		ID:     clientID,
		conn:   conn,
		send:   make(chan *messaging.Message, 256),
		server: s,
	}

	// Register the client
	select {
	case s.register <- client:
		// Successfully registered
	default:
		s.logger.Error("Failed to register client - register channel full", "client_id", clientID)
		conn.Close()
		return
	}

	// Set up ping/pong handlers
	conn.SetPingHandler(func(appData string) error {
		s.logger.Debug("Received ping from client", "client_id", clientID)
		err := conn.WriteControl(websocket.PongMessage, []byte{}, time.Now().Add(10*time.Second))
		if err == websocket.ErrCloseSent {
			return nil
		} else if e, ok := err.(net.Error); ok && e.Timeout() {
			return nil
		}
		return err
	})

	// Start the read and write pumps with panic recovery
	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.logger.Error("WebSocket client write pump panic", "panic", r, "client_id", clientID)
			}
		}()
		client.writePump()
	}()
	
	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.logger.Error("WebSocket client read pump panic", "panic", r, "client_id", clientID)
			}
		}()
		client.readPump()
	}()
}

// Shutdown gracefully shuts down the WebSocket server
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down WebSocket server")
	
	// Signal the run goroutine to stop
	close(s.shutdown)
	
	// Close all client connections
	s.mu.Lock()
	for clientID, client := range s.clients {
		s.logger.Debug("Closing client connection", "client_id", clientID)
		client.conn.Close()
	}
	s.mu.Unlock()
	
	// Shutdown the HTTP server if it exists
	if s.httpServer != nil {
		if err := s.httpServer.Shutdown(ctx); err != nil {
			s.logger.LogError(ctx, err, "Error shutting down HTTP server")
			return err
		}
	}
	
	s.logger.Info("WebSocket server shutdown completed")
	return nil
}


// GetClientCount returns the number of connected clients
func (s *Server) GetClientCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.clients)
}

// IsRunning returns whether the WebSocket server is currently running
func (s *Server) IsRunning() bool {
	select {
	case <-s.shutdown:
		return false
	default:
		return s.httpServer != nil
	}
}

// GetServerStats returns server statistics
func (s *Server) GetServerStats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	return map[string]interface{}{
		"client_count":       len(s.clients),
		"register_buffer":    len(s.register),
		"unregister_buffer":  len(s.unregister),
		"broadcast_buffer":   len(s.broadcast),
	}
}

// run handles client registration, unregistration, and message broadcasting
func (s *Server) run() {
	for {
		select {
		case <-s.shutdown:
			// Server is shutting down
			return
		case client := <-s.register:
			s.mu.Lock()
			s.clients[client.ID] = client
			s.mu.Unlock()
			s.logger.Info("Client connected", "client_id", client.ID)

		case client := <-s.unregister:
			s.mu.Lock()
			if _, ok := s.clients[client.ID]; ok {
				delete(s.clients, client.ID)
				close(client.send)
				s.logger.Info("Client disconnected", "client_id", client.ID)
			}
			s.mu.Unlock()

		case message := <-s.broadcast:
			s.mu.RLock()
			if message.ClientID == "" {
				// Broadcast to all clients if no specific client ID is set
				for _, client := range s.clients {
					select {
					case client.send <- message:
					default:
						close(client.send)
						delete(s.clients, client.ID)
					}
				}
			} else {
				// Send to specific client
				if client, ok := s.clients[message.ClientID]; ok {
					select {
					case client.send <- message:
					default:
						close(client.send)
						delete(s.clients, client.ID)
					}
				} else {
					s.logger.Warn("Client not found", "client_id", message.ClientID)
				}
			}
			s.mu.RUnlock()

		// Forward messages from the message channel to the broadcast channel
		case msg := <-s.messageCh:
			s.broadcast <- msg
		}
	}
}

// generateClientID creates a unique ID for each client
func generateClientID() string {
	// In a production environment, you might want to use a more robust ID generation
	return "client-" + messaging.RandString(8)
}
