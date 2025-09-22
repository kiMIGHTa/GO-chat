package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestSimpleIntegration tests basic functionality step by step
func TestSimpleIntegration(t *testing.T) {
	// Create hub and start it
	hub := NewHub()
	go hub.Run()

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ws" {
			handleWebSocket(hub, w, r)
		} else {
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	// Convert HTTP URL to WebSocket URL
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	t.Run("Basic Connection and Join", func(t *testing.T) {
		// Connect to WebSocket
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("Failed to connect to WebSocket: %v", err)
		}
		defer conn.Close()

		// Send join message
		joinMsg := Message{
			Type:    MessageTypeJoin,
			Content: "TestUser",
		}
		joinMsg.SetTimestamp()

		data, err := joinMsg.ToJSON()
		if err != nil {
			t.Fatalf("Failed to marshal join message: %v", err)
		}

		err = conn.WriteMessage(websocket.TextMessage, data)
		if err != nil {
			t.Fatalf("Failed to send join message: %v", err)
		}

		// Read messages for a few seconds
		messages := make([]Message, 0)
		conn.SetReadDeadline(time.Now().Add(3 * time.Second))

		for i := 0; i < 10; i++ { // Try to read up to 10 messages
			_, messageData, err := conn.ReadMessage()
			if err != nil {
				break // Timeout or connection closed
			}

			var message Message
			if err := json.Unmarshal(messageData, &message); err != nil {
				t.Errorf("Failed to unmarshal message: %v", err)
				continue
			}

			messages = append(messages, message)
			t.Logf("Received message: Type=%s, Content=%s, From=%s, Users=%v", 
				message.Type, message.Content, message.From, message.Users)
		}

		// Verify we received some messages
		if len(messages) == 0 {
			t.Fatal("No messages received")
		}

		// Look for system message
		foundSystemMsg := false
		foundUserList := false

		for _, msg := range messages {
			if msg.Type == MessageTypeSystem && strings.Contains(msg.Content, "TestUser has joined") {
				foundSystemMsg = true
			}
			if msg.Type == MessageTypeUserList && len(msg.Users) > 0 {
				foundUserList = true
			}
		}

		if !foundSystemMsg {
			t.Error("Did not receive system join message")
		}
		if !foundUserList {
			t.Error("Did not receive user list message")
		}
	})

	t.Run("Chat Message Flow", func(t *testing.T) {
		// Connect to WebSocket
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("Failed to connect to WebSocket: %v", err)
		}
		defer conn.Close()

		// Send join message first
		joinMsg := Message{
			Type:    MessageTypeJoin,
			Content: "ChatUser",
		}
		joinMsg.SetTimestamp()

		data, err := joinMsg.ToJSON()
		if err != nil {
			t.Fatalf("Failed to marshal join message: %v", err)
		}

		err = conn.WriteMessage(websocket.TextMessage, data)
		if err != nil {
			t.Fatalf("Failed to send join message: %v", err)
		}

		// Wait for join to complete - read the join messages first
		joinMessages := make([]Message, 0)
		conn.SetReadDeadline(time.Now().Add(3 * time.Second))

		// Read join messages
		for i := 0; i < 5; i++ { // Try to read up to 5 messages
			_, messageData, readErr := conn.ReadMessage()
			if readErr != nil {
				break // Timeout or connection closed
			}

			var message Message
			if unmarshalErr := json.Unmarshal(messageData, &message); unmarshalErr != nil {
				t.Errorf("Failed to unmarshal message: %v", unmarshalErr)
				continue
			}

			joinMessages = append(joinMessages, message)
			t.Logf("Join phase - Received message: Type=%s, Content=%s, From=%s, Users=%v", 
				message.Type, message.Content, message.From, message.Users)
			
			// Stop when we get both system message and user list
			if len(joinMessages) >= 2 {
				break
			}
		}

		// Send chat message
		chatMsg := Message{
			Type:    MessageTypeChat,
			From:    "ChatUser",
			Content: "Hello, World!",
		}
		chatMsg.SetTimestamp()

		data, err = chatMsg.ToJSON()
		if err != nil {
			t.Fatalf("Failed to marshal chat message: %v", err)
		}

		t.Logf("Sending chat message: %s", string(data))
		err = conn.WriteMessage(websocket.TextMessage, data)
		if err != nil {
			t.Fatalf("Failed to send chat message: %v", err)
		}

		// Read messages
		messages := make([]Message, 0)
		conn.SetReadDeadline(time.Now().Add(3 * time.Second))

		for i := 0; i < 15; i++ { // Try to read more messages
			_, messageData, err := conn.ReadMessage()
			if err != nil {
				break
			}

			var message Message
			if err := json.Unmarshal(messageData, &message); err != nil {
				t.Errorf("Failed to unmarshal message: %v", err)
				continue
			}

			messages = append(messages, message)
			t.Logf("Received message: Type=%s, Content=%s, From=%s", 
				message.Type, message.Content, message.From)
		}

		// Look for chat message echo
		foundChatMsg := false
		for _, msg := range messages {
			if msg.Type == MessageTypeChat && msg.Content == "Hello, World!" && msg.From == "ChatUser" {
				foundChatMsg = true
				break
			}
		}

		if !foundChatMsg {
			t.Error("Did not receive chat message echo")
		}
	})
}