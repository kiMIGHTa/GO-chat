package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestClient represents a test WebSocket client
type TestClient struct {
	conn        *websocket.Conn
	displayName string
	messages    []Message
	mu          sync.RWMutex
	t           *testing.T
}

// NewTestClient creates a new test client
func NewTestClient(t *testing.T, server *httptest.Server, displayName string) *TestClient {
	// Convert HTTP URL to WebSocket URL
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	
	client := &TestClient{
		conn:        conn,
		displayName: displayName,
		messages:    make([]Message, 0),
		t:           t,
	}
	
	// Start message reader
	go client.readMessages()
	
	return client
}

// readMessages reads messages from the WebSocket connection
func (tc *TestClient) readMessages() {
	defer tc.conn.Close()
	
	for {
		_, messageData, err := tc.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				tc.t.Logf("WebSocket read error for %s: %v", tc.displayName, err)
			}
			return
		}
		
		var message Message
		if err := json.Unmarshal(messageData, &message); err != nil {
			tc.t.Errorf("Failed to unmarshal message for %s: %v", tc.displayName, err)
			continue
		}
		
		tc.mu.Lock()
		tc.messages = append(tc.messages, message)
		tc.mu.Unlock()
	}
}

// SendMessage sends a message to the WebSocket
func (tc *TestClient) SendMessage(message Message) error {
	message.SetTimestamp()
	data, err := message.ToJSON()
	if err != nil {
		return err
	}
	
	return tc.conn.WriteMessage(websocket.TextMessage, data)
}

// GetMessages returns all received messages
func (tc *TestClient) GetMessages() []Message {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	
	messages := make([]Message, len(tc.messages))
	copy(messages, tc.messages)
	return messages
}

// GetLastMessage returns the last received message
func (tc *TestClient) GetLastMessage() *Message {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	
	if len(tc.messages) == 0 {
		return nil
	}
	return &tc.messages[len(tc.messages)-1]
}

// WaitForMessage waits for a message of the specified type
func (tc *TestClient) WaitForMessage(messageType string, timeout time.Duration) *Message {
	deadline := time.Now().Add(timeout)
	lastCheckedIndex := -1
	
	for time.Now().Before(deadline) {
		tc.mu.RLock()
		// Check for new messages since last check
		for i := lastCheckedIndex + 1; i < len(tc.messages); i++ {
			if tc.messages[i].Type == messageType {
				msg := tc.messages[i]
				lastCheckedIndex = i
				tc.mu.RUnlock()
				return &msg
			}
		}
		lastCheckedIndex = len(tc.messages) - 1
		tc.mu.RUnlock()
		
		time.Sleep(10 * time.Millisecond)
	}
	
	return nil
}

// WaitForMessageCount waits for a specific number of messages
func (tc *TestClient) WaitForMessageCount(count int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	
	for time.Now().Before(deadline) {
		tc.mu.RLock()
		currentCount := len(tc.messages)
		tc.mu.RUnlock()
		
		if currentCount >= count {
			return true
		}
		
		time.Sleep(10 * time.Millisecond)
	}
	
	return false
}

// Close closes the test client connection
func (tc *TestClient) Close() {
	tc.conn.Close()
}

// TestEndToEndIntegration tests the complete user flow
func TestEndToEndIntegration(t *testing.T) {
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
	
	t.Run("Single User Join and Chat Flow", func(t *testing.T) {
		// Create test client
		client := NewTestClient(t, server, "TestUser1")
		defer client.Close()
		
		// Send join message
		joinMsg := Message{
			Type:    MessageTypeJoin,
			Content: "TestUser1",
		}
		
		if err := client.SendMessage(joinMsg); err != nil {
			t.Fatalf("Failed to send join message: %v", err)
		}
		
		// Wait for system message about joining
		systemMsg := client.WaitForMessage(MessageTypeSystem, 2*time.Second)
		if systemMsg == nil {
			t.Fatal("Did not receive system message for user join")
		}
		
		if !strings.Contains(systemMsg.Content, "TestUser1 has joined the chat") {
			t.Errorf("Expected join message, got: %s", systemMsg.Content)
		}
		
		// Wait for user list message
		userListMsg := client.WaitForMessage(MessageTypeUserList, 2*time.Second)
		if userListMsg == nil {
			t.Fatal("Did not receive user list message")
		}
		
		if len(userListMsg.Users) != 1 || userListMsg.Users[0] != "TestUser1" {
			t.Errorf("Expected user list [TestUser1], got: %v", userListMsg.Users)
		}
		
		// Send chat message
		chatMsg := Message{
			Type:    MessageTypeChat,
			From:    "TestUser1",
			Content: "Hello, world!",
		}
		
		if err := client.SendMessage(chatMsg); err != nil {
			t.Fatalf("Failed to send chat message: %v", err)
		}
		
		// Wait for chat message echo
		echoMsg := client.WaitForMessage(MessageTypeChat, 2*time.Second)
		if echoMsg == nil {
			t.Fatal("Did not receive chat message echo")
		}
		
		if echoMsg.From != "TestUser1" || echoMsg.Content != "Hello, world!" {
			t.Errorf("Expected echo message from TestUser1 with content 'Hello, world!', got: %s from %s", echoMsg.Content, echoMsg.From)
		}
		
		// Verify timestamp is set
		if echoMsg.Timestamp.IsZero() {
			t.Error("Message timestamp was not set")
		}
	})
	
	t.Run("Multiple Users and Real-time Broadcasting", func(t *testing.T) {
		// Create multiple test clients
		client1 := NewTestClient(t, server, "User1")
		client2 := NewTestClient(t, server, "User2")
		client3 := NewTestClient(t, server, "User3")
		
		defer client1.Close()
		defer client2.Close()
		defer client3.Close()
		
		// All users join
		users := []*TestClient{client1, client2, client3}
		userNames := []string{"User1", "User2", "User3"}
		
		for i, client := range users {
			joinMsg := Message{
				Type:    MessageTypeJoin,
				Content: userNames[i],
			}
			
			if err := client.SendMessage(joinMsg); err != nil {
				t.Fatalf("Failed to send join message for %s: %v", userNames[i], err)
			}
		}
		
		// Wait for all clients to receive all join messages
		time.Sleep(1 * time.Second)
		
		// Verify each client received join messages for all users
		for i, client := range users {
			messages := client.GetMessages()
			
			t.Logf("Client %s received %d total messages", userNames[i], len(messages))
			for j, msg := range messages {
				t.Logf("  Message %d: Type=%s, Content=%s, Users=%v", j, msg.Type, msg.Content, msg.Users)
			}
			
			// Count system messages (join notifications)
			systemMsgCount := 0
			for _, msg := range messages {
				if msg.Type == MessageTypeSystem && strings.Contains(msg.Content, "has joined the chat") {
					systemMsgCount++
				}
			}
			
			// Each client should see join messages for all users (including themselves)
			if systemMsgCount != 3 {
				t.Errorf("Client %s should have received 3 join messages, got %d", userNames[i], systemMsgCount)
			}
			
			// Verify final user list
			userListMsg := client.WaitForMessage(MessageTypeUserList, 2*time.Second)
			if userListMsg == nil {
				t.Errorf("Client %s did not receive user list message", userNames[i])
				continue
			}
			
			if len(userListMsg.Users) != 3 {
				t.Errorf("Client %s expected 3 users in list, got %d: %v", userNames[i], len(userListMsg.Users), userListMsg.Users)
			}
		}
		
		// Test message broadcasting
		chatMsg := Message{
			Type:    MessageTypeChat,
			From:    "User1",
			Content: "Hello everyone!",
		}
		
		if err := client1.SendMessage(chatMsg); err != nil {
			t.Fatalf("Failed to send broadcast message: %v", err)
		}
		
		// All clients should receive the message
		for i, client := range users {
			broadcastMsg := client.WaitForMessage(MessageTypeChat, 2*time.Second)
			if broadcastMsg == nil {
				t.Errorf("Client %s did not receive broadcast message", userNames[i])
				continue
			}
			
			if broadcastMsg.From != "User1" || broadcastMsg.Content != "Hello everyone!" {
				t.Errorf("Client %s received incorrect broadcast message: %s from %s", userNames[i], broadcastMsg.Content, broadcastMsg.From)
			}
		}
	})
	
	t.Run("Message Ordering with Timestamps", func(t *testing.T) {
		client := NewTestClient(t, server, "TimestampUser")
		defer client.Close()
		
		// Join first
		joinMsg := Message{
			Type:    MessageTypeJoin,
			Content: "TimestampUser",
		}
		
		if err := client.SendMessage(joinMsg); err != nil {
			t.Fatalf("Failed to send join message: %v", err)
		}
		
		// Wait for join to complete
		time.Sleep(200 * time.Millisecond)
		
		// Send multiple messages quickly
		messages := []string{"Message 1", "Message 2", "Message 3"}
		sendTimes := make([]time.Time, len(messages))
		
		for i, content := range messages {
			sendTimes[i] = time.Now()
			chatMsg := Message{
				Type:    MessageTypeChat,
				From:    "TimestampUser",
				Content: content,
			}
			
			if err := client.SendMessage(chatMsg); err != nil {
				t.Fatalf("Failed to send message %d: %v", i+1, err)
			}
			
			// Small delay to ensure different timestamps
			time.Sleep(10 * time.Millisecond)
		}
		
		// Wait for all messages to be received
		if !client.WaitForMessageCount(6, 3*time.Second) { // 3 chat + 1 system + 2 user_list
			t.Fatal("Did not receive all expected messages")
		}
		
		// Verify message ordering
		receivedMessages := client.GetMessages()
		chatMessages := make([]Message, 0)
		
		for _, msg := range receivedMessages {
			if msg.Type == MessageTypeChat {
				chatMessages = append(chatMessages, msg)
			}
		}
		
		if len(chatMessages) != 3 {
			t.Fatalf("Expected 3 chat messages, got %d", len(chatMessages))
		}
		
		// Verify timestamps are in order and content matches
		for i, msg := range chatMessages {
			expectedContent := messages[i]
			if msg.Content != expectedContent {
				t.Errorf("Message %d: expected content '%s', got '%s'", i+1, expectedContent, msg.Content)
			}
			
			if msg.Timestamp.IsZero() {
				t.Errorf("Message %d: timestamp not set", i+1)
			}
			
			if i > 0 && msg.Timestamp.Before(chatMessages[i-1].Timestamp) {
				t.Errorf("Message %d: timestamp %v is before previous message timestamp %v", 
					i+1, msg.Timestamp, chatMessages[i-1].Timestamp)
			}
		}
	})
	
	t.Run("User Disconnect and Cleanup", func(t *testing.T) {
		// Create two clients
		client1 := NewTestClient(t, server, "StayUser")
		client2 := NewTestClient(t, server, "LeaveUser")
		
		defer client1.Close()
		
		// Both users join
		for _, client := range []*TestClient{client1, client2} {
			joinMsg := Message{
				Type:    MessageTypeJoin,
				Content: client.displayName,
			}
			
			if err := client.SendMessage(joinMsg); err != nil {
				t.Fatalf("Failed to send join message for %s: %v", client.displayName, err)
			}
		}
		
		// Wait for both to be registered
		time.Sleep(300 * time.Millisecond)
		
		// Verify both users are in the list
		userListMsg := client1.WaitForMessage(MessageTypeUserList, 2*time.Second)
		if userListMsg == nil || len(userListMsg.Users) != 2 {
			t.Fatal("Both users should be in the user list")
		}
		
		// Client2 disconnects
		client2.Close()
		
		// Client1 should receive leave message and updated user list
		leaveMsg := client1.WaitForMessage(MessageTypeSystem, 3*time.Second)
		if leaveMsg == nil {
			t.Fatal("Did not receive leave message")
		}
		
		if !strings.Contains(leaveMsg.Content, "LeaveUser has left the chat") {
			t.Errorf("Expected leave message for LeaveUser, got: %s", leaveMsg.Content)
		}
		
		// Wait a bit more for user list update
		time.Sleep(200 * time.Millisecond)
		
		// Find the most recent user list message
		messages := client1.GetMessages()
		var lastUserList *Message
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Type == MessageTypeUserList {
				lastUserList = &messages[i]
				break
			}
		}
		
		if lastUserList == nil {
			t.Fatal("Did not receive updated user list after disconnect")
		}
		
		if len(lastUserList.Users) != 1 || lastUserList.Users[0] != "StayUser" {
			t.Errorf("Expected user list [StayUser] after disconnect, got: %v", lastUserList.Users)
		}
	})
	
	t.Run("Error Handling and Validation", func(t *testing.T) {
		client := NewTestClient(t, server, "ErrorTestUser")
		defer client.Close()
		
		// Test sending message before joining
		chatMsg := Message{
			Type:    MessageTypeChat,
			From:    "ErrorTestUser",
			Content: "This should fail",
		}
		
		if err := client.SendMessage(chatMsg); err != nil {
			t.Fatalf("Failed to send message: %v", err)
		}
		
		// Should receive error message
		errorMsg := client.WaitForMessage(MessageTypeError, 2*time.Second)
		if errorMsg == nil {
			t.Fatal("Expected error message for sending chat before joining")
		}
		
		if !strings.Contains(errorMsg.Error, "Must join chat before sending messages") {
			t.Errorf("Expected join error, got: %s", errorMsg.Error)
		}
		
		// Test invalid display name
		joinMsg := Message{
			Type:    MessageTypeJoin,
			Content: "", // Empty display name
		}
		
		if err := client.SendMessage(joinMsg); err != nil {
			t.Fatalf("Failed to send invalid join message: %v", err)
		}
		
		// Should receive error message
		errorMsg = client.WaitForMessage(MessageTypeError, 2*time.Second)
		if errorMsg == nil {
			t.Fatal("Expected error message for empty display name")
		}
		
		// Test valid join
		joinMsg.Content = "ErrorTestUser"
		if err := client.SendMessage(joinMsg); err != nil {
			t.Fatalf("Failed to send valid join message: %v", err)
		}
		
		// Should receive system message
		systemMsg := client.WaitForMessage(MessageTypeSystem, 2*time.Second)
		if systemMsg == nil {
			t.Fatal("Did not receive system message for valid join")
		}
		
		// Test empty chat message
		emptyChatMsg := Message{
			Type:    MessageTypeChat,
			From:    "ErrorTestUser",
			Content: "", // Empty content
		}
		
		if err := client.SendMessage(emptyChatMsg); err != nil {
			t.Fatalf("Failed to send empty chat message: %v", err)
		}
		
		// Should receive error message
		errorMsg = client.WaitForMessage(MessageTypeError, 2*time.Second)
		if errorMsg == nil {
			t.Fatal("Expected error message for empty chat content")
		}
	})
}

// TestConcurrentUsers tests the system with multiple concurrent users
func TestConcurrentUsers(t *testing.T) {
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
	
	const numClients = 10
	clients := make([]*TestClient, numClients)
	
	// Create and connect all clients
	for i := 0; i < numClients; i++ {
		displayName := fmt.Sprintf("User%d", i+1)
		clients[i] = NewTestClient(t, server, displayName)
		
		// Send join message
		joinMsg := Message{
			Type:    MessageTypeJoin,
			Content: displayName,
		}
		
		if err := clients[i].SendMessage(joinMsg); err != nil {
			t.Fatalf("Failed to send join message for %s: %v", displayName, err)
		}
	}
	
	// Clean up all clients
	defer func() {
		for _, client := range clients {
			if client != nil {
				client.Close()
			}
		}
	}()
	
	// Wait for all joins to complete
	time.Sleep(1 * time.Second)
	
	// Verify all clients see all users
	for i, client := range clients {
		userListMsg := client.WaitForMessage(MessageTypeUserList, 3*time.Second)
		if userListMsg == nil {
			t.Errorf("Client %d did not receive user list", i+1)
			continue
		}
		
		if len(userListMsg.Users) != numClients {
			t.Errorf("Client %d expected %d users, got %d", i+1, numClients, len(userListMsg.Users))
		}
	}
	
	// Test concurrent messaging
	var wg sync.WaitGroup
	messagesSent := 0
	
	for i, client := range clients {
		wg.Add(1)
		go func(clientIndex int, c *TestClient) {
			defer wg.Done()
			
			for j := 0; j < 3; j++ {
				chatMsg := Message{
					Type:    MessageTypeChat,
					From:    c.displayName,
					Content: fmt.Sprintf("Message %d from %s", j+1, c.displayName),
				}
				
				if err := c.SendMessage(chatMsg); err != nil {
					t.Errorf("Client %d failed to send message %d: %v", clientIndex+1, j+1, err)
				}
				
				messagesSent++
				time.Sleep(10 * time.Millisecond) // Small delay between messages
			}
		}(i, client)
	}
	
	wg.Wait()
	
	// Wait for all messages to be processed
	time.Sleep(2 * time.Second)
	
	// Verify all clients received all messages
	expectedChatMessages := numClients * 3
	
	for i, client := range clients {
		messages := client.GetMessages()
		chatMessageCount := 0
		
		for _, msg := range messages {
			if msg.Type == MessageTypeChat {
				chatMessageCount++
			}
		}
		
		if chatMessageCount != expectedChatMessages {
			t.Errorf("Client %d expected %d chat messages, got %d", i+1, expectedChatMessages, chatMessageCount)
		}
	}
	
	t.Logf("Successfully tested %d concurrent users with %d total messages", numClients, expectedChatMessages)
}

// TestSystemResilience tests error recovery and system stability
func TestSystemResilience(t *testing.T) {
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
	
	t.Run("Malformed JSON Handling", func(t *testing.T) {
		client := NewTestClient(t, server, "MalformedUser")
		defer client.Close()
		
		// Send malformed JSON
		if err := client.conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"invalid json`)); err != nil {
			t.Fatalf("Failed to send malformed JSON: %v", err)
		}
		
		// Should receive error message
		errorMsg := client.WaitForMessage(MessageTypeError, 2*time.Second)
		if errorMsg == nil {
			t.Fatal("Expected error message for malformed JSON")
		}
		
		// Connection should still be alive - test with valid message
		joinMsg := Message{
			Type:    MessageTypeJoin,
			Content: "MalformedUser",
		}
		
		if err := client.SendMessage(joinMsg); err != nil {
			t.Fatalf("Failed to send valid message after malformed JSON: %v", err)
		}
		
		systemMsg := client.WaitForMessage(MessageTypeSystem, 2*time.Second)
		if systemMsg == nil {
			t.Fatal("Connection should still work after malformed JSON")
		}
	})
	
	t.Run("Unknown Message Type", func(t *testing.T) {
		client := NewTestClient(t, server, "UnknownTypeUser")
		defer client.Close()
		
		// Send message with unknown type
		unknownMsg := Message{
			Type:    "unknown_type",
			Content: "test",
		}
		
		if err := client.SendMessage(unknownMsg); err != nil {
			t.Fatalf("Failed to send unknown type message: %v", err)
		}
		
		// Should receive error message
		errorMsg := client.WaitForMessage(MessageTypeError, 2*time.Second)
		if errorMsg == nil {
			t.Fatal("Expected error message for unknown message type")
		}
		
		if !strings.Contains(errorMsg.Error, "Unknown message type") {
			t.Errorf("Expected unknown type error, got: %s", errorMsg.Error)
		}
	})
	
	t.Run("Rapid Connect/Disconnect", func(t *testing.T) {
		// Test rapid connection and disconnection cycles
		for i := 0; i < 5; i++ {
			client := NewTestClient(t, server, fmt.Sprintf("RapidUser%d", i))
			
			joinMsg := Message{
				Type:    MessageTypeJoin,
				Content: fmt.Sprintf("RapidUser%d", i),
			}
			
			if err := client.SendMessage(joinMsg); err != nil {
				t.Errorf("Failed to send join message for rapid user %d: %v", i, err)
			}
			
			// Wait briefly then disconnect
			time.Sleep(50 * time.Millisecond)
			client.Close()
		}
		
		// System should still be responsive
		testClient := NewTestClient(t, server, "TestAfterRapid")
		defer testClient.Close()
		
		joinMsg := Message{
			Type:    MessageTypeJoin,
			Content: "TestAfterRapid",
		}
		
		if err := testClient.SendMessage(joinMsg); err != nil {
			t.Fatalf("System not responsive after rapid connect/disconnect: %v", err)
		}
		
		systemMsg := testClient.WaitForMessage(MessageTypeSystem, 2*time.Second)
		if systemMsg == nil {
			t.Fatal("System not responsive after rapid connect/disconnect")
		}
	})
}