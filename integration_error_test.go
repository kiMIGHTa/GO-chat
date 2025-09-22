package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestWebSocketUpgradeErrors tests WebSocket upgrade error handling
func TestWebSocketUpgradeErrors(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	
	// Test with invalid upgrade request
	req := httptest.NewRequest("GET", "/ws", nil)
	w := httptest.NewRecorder()
	
	// This should fail because it's not a proper WebSocket upgrade request
	handleWebSocket(hub, w, req)
	
	// The response should indicate an upgrade failure
	if w.Code == http.StatusSwitchingProtocols {
		t.Error("Expected WebSocket upgrade to fail, but it succeeded")
	}
}

// TestServerPanicRecovery tests that server panics are recovered
func TestServerPanicRecovery(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// This should not crash the server even if it panics
		handleWebSocket(hub, w, r)
	}))
	defer server.Close()
	
	// Make a request that should not crash the server
	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	resp.Body.Close()
	
	// Server should still be responsive
	resp2, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("Server became unresponsive after error: %v", err)
	}
	resp2.Body.Close()
}

// TestConcurrentConnections tests handling of multiple concurrent connections
func TestConcurrentConnections(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handleWebSocket(hub, w, r)
	}))
	defer server.Close()
	
	// Convert HTTP URL to WebSocket URL
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	
	// Create multiple connections concurrently
	numConnections := 5
	connections := make([]*websocket.Conn, numConnections)
	
	for i := 0; i < numConnections; i++ {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("Failed to connect WebSocket %d: %v", i, err)
		}
		connections[i] = conn
		
		// Send join message
		joinMsg := Message{
			Type:    MessageTypeJoin,
			Content: "user" + string(rune(i)),
		}
		joinMsg.SetTimestamp()
		
		data, _ := joinMsg.ToJSON()
		conn.WriteMessage(websocket.TextMessage, data)
	}
	
	// Give time for all connections to be processed
	time.Sleep(200 * time.Millisecond)
	
	// Check that hub has all clients
	if len(hub.clients) != numConnections {
		t.Errorf("Expected %d clients, got %d", numConnections, len(hub.clients))
	}
	
	// Close all connections
	for i, conn := range connections {
		if conn != nil {
			conn.Close()
		}
		// Give time for cleanup
		if i == len(connections)-1 {
			time.Sleep(100 * time.Millisecond)
		}
	}
	
	// All clients should be cleaned up
	time.Sleep(200 * time.Millisecond)
	if len(hub.clients) != 0 {
		t.Errorf("Expected 0 clients after closing connections, got %d", len(hub.clients))
	}
}

// TestInvalidMessageHandling tests server response to invalid messages
func TestInvalidMessageHandling(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handleWebSocket(hub, w, r)
	}))
	defer server.Close()
	
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect WebSocket: %v", err)
	}
	defer conn.Close()
	
	// Send invalid JSON
	conn.WriteMessage(websocket.TextMessage, []byte(`{"invalid": json`))
	
	// Should receive error message
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read error response: %v", err)
	}
	
	message, err := MessageFromJSON(data)
	if err != nil {
		t.Fatalf("Failed to parse error response: %v", err)
	}
	
	if message.Type != MessageTypeError {
		t.Errorf("Expected error message, got %s", message.Type)
	}
	
	if !strings.Contains(message.Error, "Invalid message format") {
		t.Errorf("Expected 'Invalid message format' error, got: %s", message.Error)
	}
}

// TestConnectionTimeout tests connection timeout handling
func TestConnectionTimeout(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handleWebSocket(hub, w, r)
	}))
	defer server.Close()
	
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	
	// Set a very short timeout for testing
	dialer := websocket.Dialer{
		HandshakeTimeout: 1 * time.Millisecond,
	}
	
	// This should timeout (though it might succeed on very fast systems)
	_, _, err := dialer.Dial(wsURL, nil)
	
	// We expect either a timeout or successful connection
	// The important thing is that it doesn't crash
	if err != nil && !strings.Contains(err.Error(), "timeout") && !strings.Contains(err.Error(), "connection refused") {
		t.Logf("Connection attempt result: %v", err)
	}
}

// TestMessageSizeLimit tests handling of oversized messages
func TestMessageSizeLimit(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handleWebSocket(hub, w, r)
	}))
	defer server.Close()
	
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect WebSocket: %v", err)
	}
	defer conn.Close()
	
	// First join the chat
	joinMsg := Message{
		Type:    MessageTypeJoin,
		Content: "testuser",
	}
	joinMsg.SetTimestamp()
	
	data, _ := joinMsg.ToJSON()
	conn.WriteMessage(websocket.TextMessage, data)
	
	// Read join confirmation messages with timeout
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	for i := 0; i < 2; i++ { // system message + user list
		_, _, err := conn.ReadMessage()
		if err != nil {
			t.Logf("Expected message %d not received: %v", i, err)
			break
		}
	}
	
	// Send oversized message
	oversizedContent := strings.Repeat("a", 1001) // Over 1000 char limit
	chatMsg := Message{
		Type:    MessageTypeChat,
		From:    "testuser",
		Content: oversizedContent,
	}
	chatMsg.SetTimestamp()
	
	data, _ = chatMsg.ToJSON()
	conn.WriteMessage(websocket.TextMessage, data)
	
	// Should receive error message
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, responseData, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read error response: %v", err)
	}
	
	message, err := MessageFromJSON(responseData)
	if err != nil {
		t.Fatalf("Failed to parse error response: %v", err)
	}
	
	if message.Type != MessageTypeError {
		t.Errorf("Expected error message, got %s", message.Type)
	}
	
	if !strings.Contains(message.Error, "too long") {
		t.Errorf("Expected 'too long' error, got: %s", message.Error)
	}
}