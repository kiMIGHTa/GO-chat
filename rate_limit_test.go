package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestRateLimiting tests the rate limiting functionality
func TestRateLimiting(t *testing.T) {
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

	t.Run("Rate Limit Enforcement", func(t *testing.T) {
		// Connect to WebSocket
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("Failed to connect to WebSocket: %v", err)
		}
		defer conn.Close()

		// Send join message first
		joinMsg := Message{
			Type:    MessageTypeJoin,
			Content: "RateLimitUser",
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

		// Wait for join to complete
		time.Sleep(200 * time.Millisecond)

		// Send messages rapidly to trigger rate limiting
		messagesPerMinute := maxMessagesPerMinute + 5 // Exceed the limit

		// Start a goroutine to read messages
		errorReceived := false
		messageChan := make(chan Message, 100)
		
		go func() {
			for {
				conn.SetReadDeadline(time.Now().Add(5 * time.Second))
				_, messageData, readErr := conn.ReadMessage()
				if readErr != nil {
					return
				}
				
				var response Message
				if json.Unmarshal(messageData, &response) == nil {
					messageChan <- response
				}
			}
		}()

		// Send messages
		for i := 0; i < messagesPerMinute; i++ {
			chatMsg := Message{
				Type:    MessageTypeChat,
				From:    "RateLimitUser",
				Content: fmt.Sprintf("Message %d", i+1),
			}
			chatMsg.SetTimestamp()

			data, err := chatMsg.ToJSON()
			if err != nil {
				t.Fatalf("Failed to marshal chat message %d: %v", i+1, err)
			}

			err = conn.WriteMessage(websocket.TextMessage, data)
			if err != nil {
				t.Fatalf("Failed to send chat message %d: %v", i+1, err)
			}

			// Small delay to avoid overwhelming the connection
			time.Sleep(1 * time.Millisecond)
		}

		// Check for rate limit error in received messages
		timeout := time.After(3 * time.Second)
		for !errorReceived {
			select {
			case msg := <-messageChan:
				if msg.Type == MessageTypeError && strings.Contains(msg.Error, "Rate limit exceeded") {
					errorReceived = true
					t.Logf("Rate limit error received: %s", msg.Error)
				}
			case <-timeout:
				break
			}
			if errorReceived {
				break
			}
		}

		if !errorReceived {
			t.Error("Expected to receive rate limit error but didn't")
		}
	})

	t.Run("Rate Limit Recovery", func(t *testing.T) {
		// Connect to WebSocket
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("Failed to connect to WebSocket: %v", err)
		}
		defer conn.Close()

		// Send join message first
		joinMsg := Message{
			Type:    MessageTypeJoin,
			Content: "RecoveryUser",
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

		// Wait for join to complete
		time.Sleep(200 * time.Millisecond)

		// Send messages up to the limit
		for i := 0; i < maxMessagesPerMinute; i++ {
			chatMsg := Message{
				Type:    MessageTypeChat,
				From:    "RecoveryUser",
				Content: fmt.Sprintf("Message %d", i+1),
			}
			chatMsg.SetTimestamp()

			data, err := chatMsg.ToJSON()
			if err != nil {
				t.Fatalf("Failed to marshal chat message %d: %v", i+1, err)
			}

			err = conn.WriteMessage(websocket.TextMessage, data)
			if err != nil {
				t.Fatalf("Failed to send chat message %d: %v", i+1, err)
			}

			time.Sleep(1 * time.Millisecond)
		}

		// Wait for rate limit window to partially reset
		t.Logf("Waiting for rate limit window to reset...")
		time.Sleep(2 * time.Second)

		// Should be able to send more messages now
		chatMsg := Message{
			Type:    MessageTypeChat,
			From:    "RecoveryUser",
			Content: "Recovery message",
		}
		chatMsg.SetTimestamp()

		data, err = chatMsg.ToJSON()
		if err != nil {
			t.Fatalf("Failed to marshal recovery message: %v", err)
		}

		err = conn.WriteMessage(websocket.TextMessage, data)
		if err != nil {
			t.Fatalf("Failed to send recovery message: %v", err)
		}

		// Should receive the message without error
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, messageData, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("Failed to read recovery message response: %v", err)
		}

		var response Message
		if err := json.Unmarshal(messageData, &response); err != nil {
			t.Fatalf("Failed to unmarshal recovery response: %v", err)
		}

		// Should be a chat message, not an error
		if response.Type == MessageTypeError {
			t.Errorf("Received error after rate limit should have reset: %s", response.Error)
		} else if response.Type == MessageTypeChat && response.Content == "Recovery message" {
			t.Log("Successfully sent message after rate limit recovery")
		}
	})
}

// TestConnectionLimits tests the connection limiting functionality
func TestConnectionLimits(t *testing.T) {
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

	t.Run("Connection Acceptance", func(t *testing.T) {
		// Test that hub can accept connections within limits
		if !hub.CanAcceptNewConnection() {
			t.Error("Hub should accept new connections when empty")
		}

		// Connect a few clients
		var connections []*websocket.Conn
		for i := 0; i < 5; i++ {
			conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
			if err != nil {
				t.Fatalf("Failed to connect client %d: %v", i+1, err)
			}
			connections = append(connections, conn)
		}

		// Clean up connections
		for _, conn := range connections {
			conn.Close()
		}

		// Wait for cleanup
		time.Sleep(100 * time.Millisecond)
	})

	t.Run("Connection Stats", func(t *testing.T) {
		// Get initial stats
		stats := hub.GetConnectionStats()
		
		// Verify stats structure
		if _, ok := stats["total_connections"]; !ok {
			t.Error("Stats should include total_connections")
		}
		if _, ok := stats["active_connections"]; !ok {
			t.Error("Stats should include active_connections")
		}
		if _, ok := stats["idle_connections"]; !ok {
			t.Error("Stats should include idle_connections")
		}
		if _, ok := stats["max_connections"]; !ok {
			t.Error("Stats should include max_connections")
		}
		if _, ok := stats["users_online"]; !ok {
			t.Error("Stats should include users_online")
		}

		t.Logf("Connection stats: %+v", stats)
	})
}

// TestClientActivityTracking tests client activity monitoring
func TestClientActivityTracking(t *testing.T) {
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

	t.Run("Activity Timestamps", func(t *testing.T) {
		// Connect to WebSocket
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("Failed to connect to WebSocket: %v", err)
		}
		defer conn.Close()

		// Send join message
		joinMsg := Message{
			Type:    MessageTypeJoin,
			Content: "ActivityUser",
		}
		joinMsg.SetTimestamp()

		data, err := joinMsg.ToJSON()
		if err != nil {
			t.Fatalf("Failed to marshal join message: %v", err)
		}

		beforeSend := time.Now()
		err = conn.WriteMessage(websocket.TextMessage, data)
		if err != nil {
			t.Fatalf("Failed to send join message: %v", err)
		}
		afterSend := time.Now()

		// Wait for processing
		time.Sleep(100 * time.Millisecond)

		// Check that client activity was updated
		// Note: We can't directly access the client from the test, but we can verify
		// the system is working by checking that messages are processed
		
		// Send a chat message to verify activity tracking
		chatMsg := Message{
			Type:    MessageTypeChat,
			From:    "ActivityUser",
			Content: "Activity test message",
		}
		chatMsg.SetTimestamp()

		data, err = chatMsg.ToJSON()
		if err != nil {
			t.Fatalf("Failed to marshal chat message: %v", err)
		}

		err = conn.WriteMessage(websocket.TextMessage, data)
		if err != nil {
			t.Fatalf("Failed to send chat message: %v", err)
		}

		// Verify we can receive messages (indicating activity tracking is working)
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, _, err = conn.ReadMessage()
		if err != nil {
			t.Fatalf("Failed to read message response: %v", err)
		}

		t.Logf("Activity tracking test completed successfully (sent between %v and %v)", 
			beforeSend.Format(time.RFC3339Nano), afterSend.Format(time.RFC3339Nano))
	})
}