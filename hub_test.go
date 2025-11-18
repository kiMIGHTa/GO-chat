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

// Test Hub.UpdateClientName mapping
func TestHub_UpdateClientName(t *testing.T) {
	hub := NewHub()
	
	// Create mock WebSocket connection
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatal("Failed to upgrade connection:", err)
		}
		defer conn.Close()
		
		client := NewClient(hub, conn)
		
		// Test UpdateClientName
		hub.UpdateClientName(client, "TestUser")
		
		// Verify client can be retrieved by name
		retrievedClient, ok := hub.GetClientByName("TestUser")
		if !ok {
			t.Error("Client not found after UpdateClientName")
		}
		if retrievedClient != client {
			t.Error("Retrieved client does not match original client")
		}
		
		// Test updating to a new name (adds new mapping, doesn't remove old)
		hub.UpdateClientName(client, "NewName")
		
		// New name should work
		retrievedClient, ok = hub.GetClientByName("NewName")
		if !ok {
			t.Error("Client not found with new name")
		}
		if retrievedClient != client {
			t.Error("Retrieved client does not match original client with new name")
		}
		
		// Test multiple clients with different names
		client2 := NewClient(hub, conn)
		hub.UpdateClientName(client2, "AnotherUser")
		
		retrievedClient2, ok := hub.GetClientByName("AnotherUser")
		if !ok {
			t.Error("Second client not found")
		}
		if retrievedClient2 != client2 {
			t.Error("Retrieved second client does not match")
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
	time.Sleep(100 * time.Millisecond)
}

// Test Hub.GetClientByName lookup
func TestHub_GetClientByName(t *testing.T) {
	hub := NewHub()
	
	// Create mock WebSocket connection
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatal("Failed to upgrade connection:", err)
		}
		defer conn.Close()
		
		client := NewClient(hub, conn)
		
		// Test lookup before adding
		_, ok := hub.GetClientByName("NonExistent")
		if ok {
			t.Error("Should not find non-existent client")
		}
		
		// Add client
		hub.UpdateClientName(client, "TestUser")
		
		// Test successful lookup
		retrievedClient, ok := hub.GetClientByName("TestUser")
		if !ok {
			t.Error("Should find existing client")
		}
		if retrievedClient != client {
			t.Error("Retrieved client does not match original")
		}
		
		// Test case sensitivity
		_, ok = hub.GetClientByName("testuser")
		if ok {
			t.Error("Lookup should be case-sensitive")
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
	time.Sleep(100 * time.Millisecond)
}

// Test Hub.SendPrivateMessage with valid recipient
func TestHub_SendPrivateMessage_ValidRecipient(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	
	// Create mock WebSocket connections for sender and recipient
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatal("Failed to upgrade connection:", err)
		}
		defer conn.Close()
		
		// Create sender and recipient clients
		senderConn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			// Use the same connection for testing
			senderConn = conn
		}
		
		sender := NewClient(hub, senderConn)
		sender.send = make(chan []byte, 256)
		hub.UpdateClientName(sender, "Alice")
		
		recipient := NewClient(hub, conn)
		recipient.send = make(chan []byte, 256)
		hub.UpdateClientName(recipient, "Bob")
		
		// Create a private message
		message := Message{
			Type:    MessageTypePrivate,
			From:    "Alice",
			To:      "Bob",
			Content: "Hello Bob!",
		}
		message.SetTimestamp()
		
		// Send private message
		err = hub.SendPrivateMessage("Alice", "Bob", message)
		if err != nil {
			t.Errorf("SendPrivateMessage failed: %v", err)
		}
		
		// Check recipient received message
		select {
		case msg := <-recipient.send:
			var receivedMsg Message
			if err := json.Unmarshal(msg, &receivedMsg); err != nil {
				t.Errorf("Failed to unmarshal received message: %v", err)
			}
			if receivedMsg.From != "Alice" {
				t.Errorf("Expected From='Alice', got '%s'", receivedMsg.From)
			}
			if receivedMsg.To != "Bob" {
				t.Errorf("Expected To='Bob', got '%s'", receivedMsg.To)
			}
			if receivedMsg.Content != "Hello Bob!" {
				t.Errorf("Expected Content='Hello Bob!', got '%s'", receivedMsg.Content)
			}
		case <-time.After(100 * time.Millisecond):
			t.Error("Recipient did not receive message")
		}
		
		// Check sender received echo
		select {
		case msg := <-sender.send:
			var echoMsg Message
			if err := json.Unmarshal(msg, &echoMsg); err != nil {
				t.Errorf("Failed to unmarshal echo message: %v", err)
			}
			if echoMsg.From != "Alice" {
				t.Errorf("Echo: Expected From='Alice', got '%s'", echoMsg.From)
			}
			if echoMsg.To != "Bob" {
				t.Errorf("Echo: Expected To='Bob', got '%s'", echoMsg.To)
			}
		case <-time.After(100 * time.Millisecond):
			t.Error("Sender did not receive echo")
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

// Test Hub.SendPrivateMessage with offline recipient
func TestHub_SendPrivateMessage_OfflineRecipient(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	
	// Create mock WebSocket connection for sender only
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatal("Failed to upgrade connection:", err)
		}
		defer conn.Close()
		
		sender := NewClient(hub, conn)
		sender.send = make(chan []byte, 256)
		hub.UpdateClientName(sender, "Alice")
		
		// Create a private message to non-existent recipient
		message := Message{
			Type:    MessageTypePrivate,
			From:    "Alice",
			To:      "NonExistentUser",
			Content: "Hello!",
		}
		message.SetTimestamp()
		
		// Send private message should fail
		err = hub.SendPrivateMessage("Alice", "NonExistentUser", message)
		if err == nil {
			t.Error("SendPrivateMessage should fail for offline recipient")
		}
		if !strings.Contains(err.Error(), "not found or offline") {
			t.Errorf("Expected error about offline recipient, got: %v", err)
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
	time.Sleep(100 * time.Millisecond)
}

// Test that private messages are not broadcast to other clients
func TestHub_PrivateMessage_NotBroadcast(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	
	// Create mock WebSocket connections for three clients
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatal("Failed to upgrade connection:", err)
		}
		defer conn.Close()
		
		// Create three clients
		alice := NewClient(hub, conn)
		alice.send = make(chan []byte, 256)
		hub.UpdateClientName(alice, "Alice")
		
		bob := NewClient(hub, conn)
		bob.send = make(chan []byte, 256)
		hub.UpdateClientName(bob, "Bob")
		
		charlie := NewClient(hub, conn)
		charlie.send = make(chan []byte, 256)
		hub.UpdateClientName(charlie, "Charlie")
		
		// Alice sends private message to Bob
		message := Message{
			Type:    MessageTypePrivate,
			From:    "Alice",
			To:      "Bob",
			Content: "Private message for Bob",
		}
		message.SetTimestamp()
		
		err = hub.SendPrivateMessage("Alice", "Bob", message)
		if err != nil {
			t.Errorf("SendPrivateMessage failed: %v", err)
		}
		
		// Wait a bit for message delivery
		time.Sleep(50 * time.Millisecond)
		
		// Charlie should NOT receive the message
		select {
		case <-charlie.send:
			t.Error("Charlie should not receive private message between Alice and Bob")
		case <-time.After(50 * time.Millisecond):
			// Expected - Charlie should not receive anything
		}
		
		// Bob should receive the message
		select {
		case msg := <-bob.send:
			var receivedMsg Message
			if err := json.Unmarshal(msg, &receivedMsg); err != nil {
				t.Errorf("Failed to unmarshal Bob's message: %v", err)
			}
			if receivedMsg.Content != "Private message for Bob" {
				t.Errorf("Bob received wrong message: %s", receivedMsg.Content)
			}
		case <-time.After(50 * time.Millisecond):
			t.Error("Bob should have received the private message")
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

// Test that sender receives echo of private message
func TestHub_PrivateMessage_SenderEcho(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	
	// Create mock WebSocket connections
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatal("Failed to upgrade connection:", err)
		}
		defer conn.Close()
		
		sender := NewClient(hub, conn)
		sender.send = make(chan []byte, 256)
		hub.UpdateClientName(sender, "Alice")
		
		recipient := NewClient(hub, conn)
		recipient.send = make(chan []byte, 256)
		hub.UpdateClientName(recipient, "Bob")
		
		// Send private message
		message := Message{
			Type:    MessageTypePrivate,
			From:    "Alice",
			To:      "Bob",
			Content: "Test echo message",
		}
		message.SetTimestamp()
		
		err = hub.SendPrivateMessage("Alice", "Bob", message)
		if err != nil {
			t.Errorf("SendPrivateMessage failed: %v", err)
		}
		
		// Verify sender receives echo
		select {
		case msg := <-sender.send:
			var echoMsg Message
			if err := json.Unmarshal(msg, &echoMsg); err != nil {
				t.Errorf("Failed to unmarshal echo: %v", err)
			}
			if echoMsg.Type != MessageTypePrivate {
				t.Errorf("Echo type = %s, want %s", echoMsg.Type, MessageTypePrivate)
			}
			if echoMsg.From != "Alice" {
				t.Errorf("Echo From = %s, want Alice", echoMsg.From)
			}
			if echoMsg.To != "Bob" {
				t.Errorf("Echo To = %s, want Bob", echoMsg.To)
			}
			if echoMsg.Content != "Test echo message" {
				t.Errorf("Echo Content = %s, want 'Test echo message'", echoMsg.Content)
			}
		case <-time.After(100 * time.Millisecond):
			t.Error("Sender did not receive echo of private message")
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
