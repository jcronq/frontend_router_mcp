package main

import (
	"encoding/json"
	"log"
	"net/url"
	"os"
	"os/signal"
	"time"

	"github.com/gorilla/websocket"
)

func main() {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	u := url.URL{Scheme: "ws", Host: "localhost:8080", Path: "/ws"}
	log.Printf("connecting to %s", u.String())

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer c.Close()

	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				log.Println("read:", err)
				return
			}
			log.Printf("recv: %s", message)
		}
	}()

	// Send a connect message
	connectMsg := map[string]interface{}{
		"type": "connect",
	}
	connectJSON, _ := json.Marshal(connectMsg)
	err = c.WriteMessage(websocket.TextMessage, connectJSON)
	if err != nil {
		log.Println("write:", err)
		return
	}

	// Wait a bit for connection to be established
	time.Sleep(1 * time.Second)

	// Send a user response to simulate answering a question
	responseMsg := map[string]interface{}{
		"type":       "user_response",
		"request_id": "test-request-123",
		"answer":     "This is a test answer from the user",
	}
	responseJSON, _ := json.Marshal(responseMsg)
	err = c.WriteMessage(websocket.TextMessage, responseJSON)
	if err != nil {
		log.Println("write:", err)
		return
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			// Keep connection alive
		case <-interrupt:
			log.Println("interrupt")

			// Cleanly close the connection by sending a close message and then
			// waiting (with timeout) for the server to close the connection.
			err := c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			if err != nil {
				log.Println("write close:", err)
				return
			}
			select {
			case <-done:
			case <-time.After(time.Second):
			}
			return
		}
	}
}