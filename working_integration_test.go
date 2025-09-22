package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// WorkingTestClient represents a test WebSocket client that works reliably
type WorkingTestClient struct {
	conn        *websocket.Conn
	displayName string
	messages    []Message
	mu          sync.RWMutex
	t           *testing.T
	connected   bool
	connMu      sync.RWMutex
}

// NewWorkingTestClient creates a new test client with proper synchronization
func NewWorkingTestClient(t *testing.T, server *httptest.Server, displayName string) *WorkingTestClient {
	// Convert HTTP URL to WebSocket URL
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	
	client := &WorkingTestClient{
		conn:        conn,
		displayName: displayName,
		messages:    make([]Message, 0),
		t:           t,
		connected:   true,
	}
	
	// Start message reader
	go client.readMessages()
	
	// Give a moment for the connection to be established
	time.Sleep(50 * time.Millisecond)
	
	return client
}

// readMessages reads messages from the WebSocket connection
func (tc *WorkingTestClient) readMessages() {
	defer func() {
		tc.connMu.Lock()
		tc.connected = false
		tc.connMu.Unlock()
		tc.conn.Close()
	}()
	
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
		
		tc.t.Logf("Client %s received: Type=%s, Content=%s, From=%s, Users=%v", 
			tc.displayName, message.Type, message.Content, message.From, message.Users)
	}
}

// SendMessage sends a message to the WebSocket
func (tc *WorkingTestClient) SendMessage(message Message) error {
	tc.connMu.RLock()
	connected := tc.connected
	tc.connMu.RUnlock()
	
	if !connected {
		return nil // Connection closed
	}
	
	message.SetTimestamp()
	data, err := message.ToJSON()
	if err != nil {
		return err
	}
	
	return tc.conn.WriteMessage(websocket.TextMessage, data)
}

// GetMessages returns all received messages
func (tc *WorkingTestClient) GetMessages() []Message {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	
	messages := make([]Message, len(tc.messages))
	copy(messages, tc.messages)
	return messages
}

// WaitForMessageCount waits for a specific number of messages
func (tc *WorkingTestClient) WaitForMessageCount(count int, timeout time.Duration) bool {
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

// WaitForMessage waits for a message of the specified type
func (tc *WorkingTestClient) WaitForMessage(messageType string, timeout time.Duration) *Message {
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

// Close closes the test client connection
func (tc *WorkingTestClient) Close() {
	tc.connMu.Lock()
	tc.connected = false
	tc.connMu.Unlock()
	tc.conn.Close()
}

// TestWorkingEndToEndIntegration tests the complete user flow with proper synchronization
func TestWorkingEndToEndIntegration(t *testing.T) {

	t.Run("Single User Complete Flow", func(t *testing.T) {
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
		// Create test client
		client := NewWorkingTestClient(t, server, "TestUser1")
		defer client.Close()

		// Send join message
		joinMsg := Message{
			Type:    MessageTypeJoin,
			Content: "TestUser1",
		}

		if err := client.SendMessage(joinMsg); err != nil {
			t.Fatalf("Failed to send join message: %v", err)
		}

		// Wait for join messages (system message + user list)
		if !client.WaitForMessageCount(2, 3*time.Second) {
			t.Fatal("Did not receive expected join messages")
		}

		messages := client.GetMessages()
		t.Logf("Received %d messages after join", len(messages))

		// Verify system message
		foundSystemMsg := false
		foundUserList := false

		for _, msg := range messages {
			if msg.Type == MessageTypeSystem && strings.Contains(msg.Content, "TestUser1 has joined") {
				foundSystemMsg = true
			}
			if msg.Type == MessageTypeUserList && len(msg.Users) == 1 && msg.Users[0] == "TestUser1" {
				foundUserList = true
			}
		}

		if !foundSystemMsg {
			t.Error("Did not receive system join message")
		}
		if !foundUserList {
			t.Error("Did not receive correct user list message")
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
		chatEcho := client.WaitForMessage(MessageTypeChat, 2*time.Second)
		if chatEcho == nil {
			t.Fatal("Did not receive chat message echo")
		}

		if chatEcho.From != "TestUser1" || chatEcho.Content != "Hello, world!" {
			t.Errorf("Chat echo incorrect: From=%s, Content=%s", chatEcho.From, chatEcho.Content)
		}

		// Verify timestamp is set
		if chatEcho.Timestamp.IsZero() {
			t.Error("Message timestamp was not set")
		}
	})

	t.Run("Multiple Users Real-time Broadcasting", func(t *testing.T) {
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
		// Create multiple test clients with staggered timing
		client1 := NewWorkingTestClient(t, server, "User1")
		defer client1.Close()

		// User1 joins first
		joinMsg1 := Message{Type: MessageTypeJoin, Content: "User1"}
		if err := client1.SendMessage(joinMsg1); err != nil {
			t.Fatalf("Failed to send join message for User1: %v", err)
		}

		// Wait for User1 to be fully joined (system message + user list)
		if !client1.WaitForMessageCount(2, 3*time.Second) {
			messages := client1.GetMessages()
			t.Fatalf("User1 did not complete join process, got %d messages", len(messages))
		}

		// Now create and join User2
		client2 := NewWorkingTestClient(t, server, "User2")
		defer client2.Close()

		joinMsg2 := Message{Type: MessageTypeJoin, Content: "User2"}
		if err := client2.SendMessage(joinMsg2); err != nil {
			t.Fatalf("Failed to send join message for User2: %v", err)
		}

		// Wait for User2's join to complete
		if !client2.WaitForMessageCount(2, 3*time.Second) {
			messages := client2.GetMessages()
			t.Fatalf("User2 did not complete join process, got %d messages", len(messages))
		}

		// Wait a bit more for cross-notifications
		time.Sleep(500 * time.Millisecond)

		// Check that both clients received appropriate messages
		messages1 := client1.GetMessages()
		messages2 := client2.GetMessages()

		t.Logf("Client1 received %d messages", len(messages1))
		t.Logf("Client2 received %d messages", len(messages2))

		// Verify User1 received User2's join notification
		foundUser2Join := false
		for _, msg := range messages1 {
			if msg.Type == MessageTypeSystem && strings.Contains(msg.Content, "User2 has joined") {
				foundUser2Join = true
				break
			}
		}
		if !foundUser2Join {
			t.Error("User1 did not receive User2's join notification")
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

		// Both clients should receive the message
		chat1 := client1.WaitForMessage(MessageTypeChat, 2*time.Second)
		chat2 := client2.WaitForMessage(MessageTypeChat, 2*time.Second)

		if chat1 == nil || chat1.Content != "Hello everyone!" {
			t.Error("Client1 did not receive broadcast message")
		}
		if chat2 == nil || chat2.Content != "Hello everyone!" {
			t.Error("Client2 did not receive broadcast message")
		}
	})

	t.Run("Error Handling", func(t *testing.T) {
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
		client := NewWorkingTestClient(t, server, "ErrorUser")
		defer client.Close()

		// Test sending message before joining
		chatMsg := Message{
			Type:    MessageTypeChat,
			From:    "ErrorUser",
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

		// Test valid join
		joinMsg := Message{
			Type:    MessageTypeJoin,
			Content: "ErrorUser",
		}

		if err := client.SendMessage(joinMsg); err != nil {
			t.Fatalf("Failed to send valid join message: %v", err)
		}

		// Should receive system message
		systemMsg := client.WaitForMessage(MessageTypeSystem, 2*time.Second)
		if systemMsg == nil {
			t.Fatal("Did not receive system message for valid join")
		}

		if !strings.Contains(systemMsg.Content, "ErrorUser has joined") {
			t.Errorf("Expected join system message, got: %s", systemMsg.Content)
		}
	})
}