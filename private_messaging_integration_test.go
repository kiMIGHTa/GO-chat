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

// PrivateTestClient represents a test WebSocket client for private messaging tests
type PrivateTestClient struct {
	conn        *websocket.Conn
	displayName string
	messages    []Message
	mu          sync.RWMutex
	t           *testing.T
	connected   bool
	connMu      sync.RWMutex
}

// NewPrivateTestClient creates a new test client
func NewPrivateTestClient(t *testing.T, server *httptest.Server, displayName string) *PrivateTestClient {
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	
	client := &PrivateTestClient{
		conn:        conn,
		displayName: displayName,
		messages:    make([]Message, 0),
		t:           t,
		connected:   true,
	}
	
	go client.readMessages()
	time.Sleep(50 * time.Millisecond)
	
	return client
}

func (tc *PrivateTestClient) readMessages() {
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
		
		tc.t.Logf("Client %s received: Type=%s, From=%s, To=%s, Content=%s", 
			tc.displayName, message.Type, message.From, message.To, message.Content)
	}
}

func (tc *PrivateTestClient) SendMessage(message Message) error {
	tc.connMu.RLock()
	connected := tc.connected
	tc.connMu.RUnlock()
	
	if !connected {
		return nil
	}
	
	message.SetTimestamp()
	data, err := message.ToJSON()
	if err != nil {
		return err
	}
	
	return tc.conn.WriteMessage(websocket.TextMessage, data)
}

func (tc *PrivateTestClient) GetMessages() []Message {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	
	messages := make([]Message, len(tc.messages))
	copy(messages, tc.messages)
	return messages
}

func (tc *PrivateTestClient) WaitForMessageType(messageType string, timeout time.Duration) *Message {
	deadline := time.Now().Add(timeout)
	lastCheckedIndex := -1
	
	for time.Now().Before(deadline) {
		tc.mu.RLock()
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

func (tc *PrivateTestClient) WaitForPrivateMessage(from string, timeout time.Duration) *Message {
	deadline := time.Now().Add(timeout)
	lastCheckedIndex := -1
	
	for time.Now().Before(deadline) {
		tc.mu.RLock()
		for i := lastCheckedIndex + 1; i < len(tc.messages); i++ {
			if tc.messages[i].Type == MessageTypePrivate && tc.messages[i].From == from {
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

func (tc *PrivateTestClient) Close() {
	tc.connMu.Lock()
	tc.connected = false
	tc.connMu.Unlock()
	tc.conn.Close()
}

// TestPrivateMessagingIntegration tests end-to-end private message delivery
func TestPrivateMessagingIntegration(t *testing.T) {
	t.Run("End-to-end private message delivery", func(t *testing.T) {
		hub := NewHub()
		go hub.Run()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/ws" {
				handleWebSocket(hub, w, r)
			}
		}))
		defer server.Close()

		// Create two clients
		alice := NewPrivateTestClient(t, server, "Alice")
		defer alice.Close()
		
		bob := NewPrivateTestClient(t, server, "Bob")
		defer bob.Close()

		// Both join the chat
		alice.SendMessage(Message{Type: MessageTypeJoin, Content: "Alice"})
		bob.SendMessage(Message{Type: MessageTypeJoin, Content: "Bob"})

		time.Sleep(500 * time.Millisecond)

		// Alice sends private message to Bob
		privateMsg := Message{
			Type:    MessageTypePrivate,
			From:    "Alice",
			To:      "Bob",
			Content: "Hello Bob, this is private!",
		}
		
		if err := alice.SendMessage(privateMsg); err != nil {
			t.Fatalf("Failed to send private message: %v", err)
		}

		// Bob should receive the message
		bobMsg := bob.WaitForPrivateMessage("Alice", 2*time.Second)
		if bobMsg == nil {
			t.Fatal("Bob did not receive private message from Alice")
		}

		if bobMsg.Content != "Hello Bob, this is private!" {
			t.Errorf("Bob received wrong content: %s", bobMsg.Content)
		}
		if bobMsg.To != "Bob" {
			t.Errorf("Message To field incorrect: %s", bobMsg.To)
		}
	})

	t.Run("Private message echo to sender", func(t *testing.T) {
		hub := NewHub()
		go hub.Run()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/ws" {
				handleWebSocket(hub, w, r)
			}
		}))
		defer server.Close()

		alice := NewPrivateTestClient(t, server, "Alice")
		defer alice.Close()
		
		bob := NewPrivateTestClient(t, server, "Bob")
		defer bob.Close()

		alice.SendMessage(Message{Type: MessageTypeJoin, Content: "Alice"})
		bob.SendMessage(Message{Type: MessageTypeJoin, Content: "Bob"})

		time.Sleep(500 * time.Millisecond)

		// Alice sends private message
		privateMsg := Message{
			Type:    MessageTypePrivate,
			From:    "Alice",
			To:      "Bob",
			Content: "Test echo message",
		}
		
		alice.SendMessage(privateMsg)

		// Alice should receive echo
		aliceEcho := alice.WaitForPrivateMessage("Alice", 2*time.Second)
		if aliceEcho == nil {
			t.Fatal("Alice did not receive echo of her private message")
		}

		if aliceEcho.Content != "Test echo message" {
			t.Errorf("Echo content incorrect: %s", aliceEcho.Content)
		}
		if aliceEcho.To != "Bob" {
			t.Errorf("Echo To field incorrect: %s", aliceEcho.To)
		}
	})

	t.Run("Private message not visible to other users", func(t *testing.T) {
		hub := NewHub()
		go hub.Run()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/ws" {
				handleWebSocket(hub, w, r)
			}
		}))
		defer server.Close()

		alice := NewPrivateTestClient(t, server, "Alice")
		defer alice.Close()
		
		bob := NewPrivateTestClient(t, server, "Bob")
		defer bob.Close()
		
		charlie := NewPrivateTestClient(t, server, "Charlie")
		defer charlie.Close()

		alice.SendMessage(Message{Type: MessageTypeJoin, Content: "Alice"})
		bob.SendMessage(Message{Type: MessageTypeJoin, Content: "Bob"})
		charlie.SendMessage(Message{Type: MessageTypeJoin, Content: "Charlie"})

		time.Sleep(500 * time.Millisecond)

		// Alice sends private message to Bob
		privateMsg := Message{
			Type:    MessageTypePrivate,
			From:    "Alice",
			To:      "Bob",
			Content: "Secret message for Bob only",
		}
		
		alice.SendMessage(privateMsg)

		// Wait for message delivery
		time.Sleep(300 * time.Millisecond)

		// Charlie should NOT receive the private message
		charlieMessages := charlie.GetMessages()
		for _, msg := range charlieMessages {
			if msg.Type == MessageTypePrivate && msg.Content == "Secret message for Bob only" {
				t.Error("Charlie received private message between Alice and Bob")
			}
		}

		// Bob should receive it
		bobMsg := bob.WaitForPrivateMessage("Alice", 1*time.Second)
		if bobMsg == nil {
			t.Fatal("Bob did not receive private message")
		}
	})

	t.Run("Conversation switching with message history", func(t *testing.T) {
		hub := NewHub()
		go hub.Run()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/ws" {
				handleWebSocket(hub, w, r)
			}
		}))
		defer server.Close()

		alice := NewPrivateTestClient(t, server, "Alice")
		defer alice.Close()
		
		bob := NewPrivateTestClient(t, server, "Bob")
		defer bob.Close()

		alice.SendMessage(Message{Type: MessageTypeJoin, Content: "Alice"})
		bob.SendMessage(Message{Type: MessageTypeJoin, Content: "Bob"})

		time.Sleep(500 * time.Millisecond)

		// Send multiple messages
		for i := 1; i <= 3; i++ {
			msg := Message{
				Type:    MessageTypePrivate,
				From:    "Alice",
				To:      "Bob",
				Content: "Message " + string(rune('0'+i)),
			}
			alice.SendMessage(msg)
			time.Sleep(100 * time.Millisecond)
		}

		time.Sleep(300 * time.Millisecond)

		// Bob should have received all messages
		bobMessages := bob.GetMessages()
		privateCount := 0
		for _, msg := range bobMessages {
			if msg.Type == MessageTypePrivate && msg.From == "Alice" {
				privateCount++
			}
		}

		if privateCount < 3 {
			t.Errorf("Bob should have received 3 private messages, got %d", privateCount)
		}
	})

	t.Run("User disconnect handling in active conversation", func(t *testing.T) {
		hub := NewHub()
		go hub.Run()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/ws" {
				handleWebSocket(hub, w, r)
			}
		}))
		defer server.Close()

		alice := NewPrivateTestClient(t, server, "Alice")
		defer alice.Close()
		
		bob := NewPrivateTestClient(t, server, "Bob")

		alice.SendMessage(Message{Type: MessageTypeJoin, Content: "Alice"})
		bob.SendMessage(Message{Type: MessageTypeJoin, Content: "Bob"})

		time.Sleep(500 * time.Millisecond)

		// Bob disconnects
		bob.Close()
		time.Sleep(300 * time.Millisecond)

		// Alice tries to send message to Bob
		privateMsg := Message{
			Type:    MessageTypePrivate,
			From:    "Alice",
			To:      "Bob",
			Content: "Are you there?",
		}
		
		alice.SendMessage(privateMsg)

		// Alice should receive error message
		errorMsg := alice.WaitForMessageType(MessageTypeError, 2*time.Second)
		if errorMsg == nil {
			t.Fatal("Alice should receive error for offline recipient")
		}

		// Error could be either "not found or offline" or "server busy" depending on timing
		if !strings.Contains(errorMsg.Error, "not found or offline") && !strings.Contains(errorMsg.Error, "server busy") {
			t.Errorf("Expected offline or server busy error, got: %s", errorMsg.Error)
		}
	})

	t.Run("Error handling for offline recipients", func(t *testing.T) {
		hub := NewHub()
		go hub.Run()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/ws" {
				handleWebSocket(hub, w, r)
			}
		}))
		defer server.Close()

		alice := NewPrivateTestClient(t, server, "Alice")
		defer alice.Close()

		alice.SendMessage(Message{Type: MessageTypeJoin, Content: "Alice"})
		time.Sleep(300 * time.Millisecond)

		// Try to send to non-existent user
		privateMsg := Message{
			Type:    MessageTypePrivate,
			From:    "Alice",
			To:      "NonExistentUser",
			Content: "Hello?",
		}
		
		alice.SendMessage(privateMsg)

		// Should receive error
		errorMsg := alice.WaitForMessageType(MessageTypeError, 2*time.Second)
		if errorMsg == nil {
			t.Fatal("Should receive error for non-existent recipient")
		}

		if !strings.Contains(errorMsg.Error, "not found or offline") {
			t.Errorf("Expected offline error, got: %s", errorMsg.Error)
		}
	})

	t.Run("Self-messaging prevention", func(t *testing.T) {
		hub := NewHub()
		go hub.Run()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/ws" {
				handleWebSocket(hub, w, r)
			}
		}))
		defer server.Close()

		alice := NewPrivateTestClient(t, server, "Alice")
		defer alice.Close()

		alice.SendMessage(Message{Type: MessageTypeJoin, Content: "Alice"})
		time.Sleep(300 * time.Millisecond)

		// Try to send message to self
		selfMsg := Message{
			Type:    MessageTypePrivate,
			From:    "Alice",
			To:      "Alice",
			Content: "Message to myself",
		}
		
		alice.SendMessage(selfMsg)

		// Should receive error
		errorMsg := alice.WaitForMessageType(MessageTypeError, 2*time.Second)
		if errorMsg == nil {
			t.Fatal("Should receive error for self-messaging")
		}

		if !strings.Contains(errorMsg.Error, "cannot send private message to yourself") {
			t.Errorf("Expected self-messaging error, got: %s", errorMsg.Error)
		}
	})
}
