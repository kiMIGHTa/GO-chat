package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestStaticFileServing tests that static files are served correctly
func TestStaticFileServing(t *testing.T) {
	// Create a test server
	hub := NewHub()
	go hub.Run()

	// Set up routes like in main
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		handleWebSocket(hub, w, r)
	})
	fs := http.FileServer(http.Dir("./static/"))
	mux.Handle("/", fs)

	server := httptest.NewServer(mux)
	defer server.Close()

	// Test root path serves static files
	resp, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Should return 200 since we have an index.html file
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 for static file serving, got %d", resp.StatusCode)
	}
}

// TestWebSocketEndpoint tests that the /ws endpoint handles WebSocket upgrades
func TestWebSocketEndpoint(t *testing.T) {
	// Create a test server
	hub := NewHub()
	go hub.Run()

	// Set up routes
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		handleWebSocket(hub, w, r)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	// Convert HTTP URL to WebSocket URL
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	// Test WebSocket connection
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer conn.Close()

	// Connection should be established successfully
	// Send a test message to verify connection works
	testMsg := Message{
		Type:    MessageTypeJoin,
		Content: "TestUser",
	}
	testMsg.SetTimestamp()

	jsonData, err := testMsg.ToJSON()
	if err != nil {
		t.Fatalf("Failed to marshal test message: %v", err)
	}

	err = conn.WriteMessage(websocket.TextMessage, jsonData)
	if err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Read response to verify server processed the message
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, _, err = conn.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}
}

// TestWebSocketUpgradeError tests error handling for invalid WebSocket upgrade requests
func TestWebSocketUpgradeError(t *testing.T) {
	// Create a test server
	hub := NewHub()
	go hub.Run()

	// Set up routes
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		handleWebSocket(hub, w, r)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	// Make a regular HTTP request to /ws (should fail WebSocket upgrade)
	resp, err := http.Get(server.URL + "/ws")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Should return 400 Bad Request for invalid WebSocket upgrade
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400 for invalid WebSocket upgrade, got %d", resp.StatusCode)
	}
}

// TestServerRouting tests that different routes are handled correctly
func TestServerRouting(t *testing.T) {
	// Create a test server
	hub := NewHub()
	go hub.Run()

	// Set up routes like in main
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		handleWebSocket(hub, w, r)
	})
	fs := http.FileServer(http.Dir("./static/"))
	mux.Handle("/", fs)

	server := httptest.NewServer(mux)
	defer server.Close()

	// Test that /ws endpoint exists
	resp, err := http.Get(server.URL + "/ws")
	if err != nil {
		t.Fatalf("Failed to make request to /ws: %v", err)
	}
	resp.Body.Close()

	// Should get 400 (bad request) not 404 (not found)
	if resp.StatusCode == http.StatusNotFound {
		t.Error("/ws endpoint not found - routing not set up correctly")
	}

	// Test that root path is handled by static file server
	resp2, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("Failed to make request to /: %v", err)
	}
	resp2.Body.Close()

	// Should be handled by file server (200 since we have index.html)
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 for static file serving, got %d", resp2.StatusCode)
	}
}

// TestMultipleWebSocketConnections tests handling multiple concurrent WebSocket connections
func TestMultipleWebSocketConnections(t *testing.T) {
	// Create a test server
	hub := NewHub()
	go hub.Run()

	// Set up routes
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		handleWebSocket(hub, w, r)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	// Convert HTTP URL to WebSocket URL
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	// Create multiple connections
	var connections []*websocket.Conn
	numConnections := 3

	for i := 0; i < numConnections; i++ {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("Failed to connect WebSocket %d: %v", i, err)
		}
		connections = append(connections, conn)
	}

	// Clean up connections
	for i, conn := range connections {
		if err := conn.Close(); err != nil {
			t.Errorf("Failed to close connection %d: %v", i, err)
		}
	}

	// Verify hub handled all connections
	if hub.GetClientCount() > 0 {
		t.Errorf("Expected 0 clients after closing connections, got %d", hub.GetClientCount())
	}
}