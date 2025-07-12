package wsserver

import (
	"encoding/json"
	"log"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jcronq/frontend_router_mcp/pkg/messaging"
)

// readPump pumps messages from the websocket connection to the hub.
func (c *Client) readPump() {
	defer func() {
		c.server.unregister <- c
		c.conn.Close()
	}()

	// Set read deadline to detect client disconnections
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error { 
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil 
	})

	for {
		// Read the raw message as bytes first
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		// Parse the message to determine its type
		var msgData struct {
			Type string `json:"type"`
		}

		messageType := "unknown"
		if err := json.Unmarshal(message, &msgData); err == nil {
			messageType = msgData.Type
		}

		// Create a new message with the raw JSON payload and response sender
		msg := &messaging.Message{
			ID:             messaging.RandString(8),
			Type:           messageType,
			Timestamp:      time.Now().UTC(),
			Payload:        message,
			ClientID:       c.ID,
			ResponseSender: c, // Client implements ResponseSender
		}

		log.Printf("Forwarding message type %s from client %s", messageType, c.ID)

		// Forward the message to the message channel
		select {
		case c.server.messageCh <- msg:
		default:
			log.Printf("Message channel full, dropping message from client %s", c.ID)
		}
	}
}

// writePump pumps messages from the hub to the websocket connection.
func (c *Client) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				// The hub closed the channel.
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// Write the message as raw JSON bytes
			err := c.conn.WriteMessage(websocket.TextMessage, message.Payload)
			if err != nil {
				log.Printf("Error writing message to client %s: %v", c.ID, err)
				return
			}

		case <-ticker.C:
			// Send a ping to keep the connection alive
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("Error sending ping to client %s: %v", c.ID, err)
				return
			}
		}
	}
}
