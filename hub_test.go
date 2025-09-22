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

// MockClient creates a mock client for testing
type MockClient struct {
	*Client
	receivedMessages [][]byte
}

func (mc *MockClient) mockSend(data []byte) {
	mc.receivedMessages = append(mc.receivedMessages, data)
}

func TestNewHub(t *testing.T) {
	hub := NewHub()
	
	if hub == nil {
		t.Fatal("NewHub() returned nil")
	}
	
	if hub.clients == nil {
		t.Error("Hub clients map not initialized")
	}
	
	if hub.broadcast == nil {
		t.Error("Hub broadcast channel not initialized")
	}
	
	if hub.register == nil {
		t.Error("Hub register channel not initialized")
	}
	
	if hub.unregister == nil {
		t.Error("Hub unregister channel not initialized")
	}
	
	if hub.userList == nil {
		t.Error("Hub userList map not initialized")
	}
}

func TestHubRegisterClient(t *testing.T) {
	hub := NewHub()
	
	// Start hub in goroutine
	go hub.Run()
	
	// Create mock WebSocket connection
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatal("Failed to upgrade connection:", err)
		}
		defer conn.Close()
		
		client := NewClient(hub, conn)
		client.SetDisplayName("TestUser")
		
		// Register client
		hub.RegisterClient(client, "TestUser")
		
		// Give some time for processing
		time.Sleep(100 * time.Millisecond)
		
		// Check if client is registered
		if !hub.clients[client] {
			t.Error("Client not registered in hub")
		}
		
		// Check if user is in user list
		users := hub.GetConnectedUsers()
		found := false
		for _, user := range users {
			if user == "TestUser" {
				found = true
				break
			}
		}
		if !found {
			t.Error("User not found in connected users list")
		}
	}))
	defer server.Close()
	
	// Connect to test server
	url := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal("Failed to connect:", err)
	}
	defer conn.Close()
	
	// Wait for test to complete
	time.Sleep(200 * time.Millisecond)
}

func TestHubUnregisterClient(t *testing.T) {
	hub := NewHub()
	
	// Start hub in goroutine
	go hub.Run()
	
	// Create mock WebSocket connection
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatal("Failed to upgrade connection:", err)
		}
		defer conn.Close()
		
		client := NewClient(hub, conn)
		client.SetDisplayName("TestUser")
		
		// Register client first
		hub.RegisterClient(client, "TestUser")
		time.Sleep(50 * time.Millisecond)
		
		// Verify client is registered
		if !hub.clients[client] {
			t.Error("Client should be registered")
		}
		
		// Unregister client
		hub.UnregisterClient(client)
		time.Sleep(50 * time.Millisecond)
		
		// Check if client is unregistered
		if hub.clients[client] {
			t.Error("Client should be unregistered")
		}
		
		// Check if user is removed from user list
		users := hub.GetConnectedUsers()
		for _, user := range users {
			if user == "TestUser" {
				t.Error("User should be removed from connected users list")
			}
		}
	}))
	defer server.Close()
	
	// Connect to test server
	url := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal("Failed to connect:", err)
	}
	defer conn.Close()
	
	// Wait for test to complete
	time.Sleep(200 * time.Millisecond)
}

func TestHubBroadcastMessage(t *testing.T) {
	hub := NewHub()
	
	// Start hub in goroutine
	go hub.Run()
	
	// Create a test message
	testMessage := Message{
		Type:    MessageTypeChat,
		From:    "TestUser",
		Content: "Hello, World!",
	}
	testMessage.SetTimestamp()
	
	// Test broadcasting the message
	hub.BroadcastMessage(testMessage)
	
	// Test JSON conversion
	jsonData, err := testMessage.ToJSON()
	if err != nil {
		t.Fatal("Failed to convert message to JSON:", err)
	}
	
	// Verify JSON structure
	var parsedMessage Message
	err = json.Unmarshal(jsonData, &parsedMessage)
	if err != nil {
		t.Fatal("Failed to parse JSON:", err)
	}
	
	if parsedMessage.Type != MessageTypeChat {
		t.Error("Message type not preserved in JSON")
	}
	
	if parsedMessage.From != "TestUser" {
		t.Error("Message sender not preserved in JSON")
	}
	
	if parsedMessage.Content != "Hello, World!" {
		t.Error("Message content not preserved in JSON")
	}
}

func TestHubBroadcastUserList(t *testing.T) {
	hub := NewHub()
	
	// Start hub in goroutine
	go hub.Run()
	
	// Create mock clients
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatal("Failed to upgrade connection:", err)
		}
		defer conn.Close()
		
		client1 := NewClient(hub, conn)
		client1.SetDisplayName("User1")
		
		client2 := NewClient(hub, conn)
		client2.SetDisplayName("User2")
		
		// Register clients
		hub.RegisterClient(client1, "User1")
		hub.RegisterClient(client2, "User2")
		
		// Give some time for processing
		time.Sleep(100 * time.Millisecond)
		
		// Check connected users
		users := hub.GetConnectedUsers()
		if len(users) != 2 {
			t.Errorf("Expected 2 users, got %d", len(users))
		}
		
		// Verify both users are in the list
		userMap := make(map[string]bool)
		for _, user := range users {
			userMap[user] = true
		}
		
		if !userMap["User1"] {
			t.Error("User1 not found in user list")
		}
		
		if !userMap["User2"] {
			t.Error("User2 not found in user list")
		}
	}))
	defer server.Close()
	
	// Connect to test server
	url := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal("Failed to connect:", err)
	}
	defer conn.Close()
	
	// Wait for test to complete
	time.Sleep(200 * time.Millisecond)
}

func TestHubSystemMessages(t *testing.T) {
	// Test system message creation for join events
	joinMsg := &Message{
		Type:    MessageTypeSystem,
		Content: "TestUser has joined the chat",
	}
	joinMsg.SetTimestamp()
	
	if err := joinMsg.Validate(); err != nil {
		t.Error("Join system message should be valid:", err)
	}
	
	// Test system message creation for leave events
	leaveMsg := &Message{
		Type:    MessageTypeSystem,
		Content: "TestUser has left the chat",
	}
	leaveMsg.SetTimestamp()
	
	if err := leaveMsg.Validate(); err != nil {
		t.Error("Leave system message should be valid:", err)
	}
	
	// Test user list message
	userListMsg := &Message{
		Type:  MessageTypeUserList,
		Users: []string{"User1", "User2"},
	}
	userListMsg.SetTimestamp()
	
	if err := userListMsg.Validate(); err != nil {
		t.Error("User list message should be valid:", err)
	}
}

func TestHubGetClientCount(t *testing.T) {
	hub := NewHub()
	
	// Initially should have 0 clients
	if count := hub.GetClientCount(); count != 0 {
		t.Errorf("Expected 0 clients, got %d", count)
	}
	
	// Start hub in goroutine
	go hub.Run()
	
	// Create mock WebSocket connection
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatal("Failed to upgrade connection:", err)
		}
		defer conn.Close()
		
		client := NewClient(hub, conn)
		client.SetDisplayName("TestUser")
		
		// Register client
		hub.RegisterClient(client, "TestUser")
		time.Sleep(50 * time.Millisecond)
		
		// Should have 1 client
		if count := hub.GetClientCount(); count != 1 {
			t.Errorf("Expected 1 client, got %d", count)
		}
	}))
	defer server.Close()
	
	// Connect to test server
	url := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal("Failed to connect:", err)
	}
	defer conn.Close()
	
	// Wait for test to complete
	time.Sleep(200 * time.Millisecond)
}

func TestHubConcurrentOperations(t *testing.T) {
	hub := NewHub()
	
	// Start hub in goroutine
	go hub.Run()
	
	// Test concurrent user list access
	done := make(chan bool, 10)
	
	// Start multiple goroutines that access user list
	for i := 0; i < 10; i++ {
		go func(id int) {
			// Simulate concurrent access to user list
			users := hub.GetConnectedUsers()
			_ = users // Use the result to avoid compiler optimization
			
			// Simulate concurrent broadcast
			testMsg := Message{
				Type:    MessageTypeSystem,
				Content: "Concurrent test message",
			}
			testMsg.SetTimestamp()
			hub.BroadcastMessage(testMsg)
			
			done <- true
		}(i)
	}
	
	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
	
	// If we reach here without deadlock or race conditions, test passes
}