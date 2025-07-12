package wsserver

import (
	"log"

	"github.com/jcronq/frontend_router_mcp/pkg/messaging"
)

// SendResponse implements the messaging.ResponseSender interface
// It sends a message to the client through the WebSocket connection
func (c *Client) SendResponse(msg *messaging.Message) error {
	select {
	case c.send <- msg:
		return nil
	default:
		log.Printf("Send channel full for client %s, dropping message", c.ID)
		return nil
	}
}
