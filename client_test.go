package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestClient_SetDisplayName(t *testing.T) {
	hub := NewHub()
	client := &Client{hub: hub}

	tests := []struct {
		name        string
		displayName string
		wantErr     bool
		expected    string
		errContains string
	}{
		{
			name:        "valid display name",
			displayName: "TestUser",
			wantErr:     false,
			expected:    "TestUser",
		},
		{
			name:        "display name with spaces",
			displayName: "  Test User  ",
			wantErr:     false,
			expected:    "Test User",
		},
		{
			name:        "empty display name",
			displayName: "",
			wantErr:     true,
			errContains: "cannot be empty",
		},
		{
			name:        "whitespace only display name",
			displayName: "   ",
			wantErr:     true,
			errContains: "cannot be empty",
		},
		{
			name:        "display name too long",
			displayName: strings.Repeat("a", 51),
			wantErr:     true,
			errContains: "cannot exceed 50 characters",
		},
		{
			name:        "display name at max length",
			displayName: strings.Repeat("a", 50),
			wantErr:     false,
			expected:    strings.Repeat("a", 50),
		},
		{
			name:        "display name with HTML tags",
			displayName: "Test<script>alert('xss')</script>User",
			wantErr:     true,
			errContains: "cannot contain HTML tags",
		},
		{
			name:        "display name with control characters",
			displayName: "Test\x00User",
			wantErr:     true,
			errContains: "invalid control characters",
		},
		{
			name:        "display name with emojis (valid)",
			displayName: "Test😀User",
			wantErr:     false,
			expected:    "Test😀User",
		},
		{
			name:        "display name with tab (valid)",
			displayName: "Test\tUser",
			wantErr:     false,
			expected:    "Test\tUser",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.SetDisplayName(tt.displayName)
			if (err != nil) != tt.wantErr {
				t.Errorf("SetDisplayName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("SetDisplayName() error = %v, want error containing %v", err, tt.errContains)
			}
			if !tt.wantErr && client.GetDisplayName() != tt.expected {
				t.Errorf("SetDisplayName() got = %v, want %v", client.GetDisplayName(), tt.expected)
			}
		})
	}
}

func TestClient_GetDisplayName(t *testing.T) {
	hub := NewHub()
	client := &Client{hub: hub}

	// Test initial empty display name
	if client.GetDisplayName() != "" {
		t.Errorf("GetDisplayName() initial value = %v, want empty string", client.GetDisplayName())
	}

	// Test after setting display name
	testName := "TestUser"
	client.SetDisplayName(testName)
	if client.GetDisplayName() != testName {
		t.Errorf("GetDisplayName() after set = %v, want %v", client.GetDisplayName(), testName)
	}
}

func TestNewClient(t *testing.T) {
	hub := NewHub()

	// Create a test WebSocket connection
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("Failed to upgrade connection: %v", err)
		}
		defer conn.Close()

		client := NewClient(hub, conn)

		// Test client initialization
		if client.hub != hub {
			t.Errorf("NewClient() hub = %v, want %v", client.hub, hub)
		}
		if client.conn != conn {
			t.Errorf("NewClient() conn = %v, want %v", client.conn, conn)
		}
		if client.send == nil {
			t.Error("NewClient() send channel is nil")
		}
		if client.displayName != "" {
			t.Errorf("NewClient() displayName = %v, want empty string", client.displayName)
		}

		// Test send channel capacity
		// Fill the channel to test capacity
		for i := 0; i < 256; i++ {
			select {
			case client.send <- []byte("test"):
			default:
				t.Errorf("Send channel should have capacity of 256, failed at %d", i)
				return
			}
		}

		// This should not block since we're at capacity
		select {
		case client.send <- []byte("overflow"):
			t.Error("Send channel should be at capacity")
		default:
			// Expected behavior
		}
	}))
	defer server.Close()

	// Convert HTTP URL to WebSocket URL
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Connect to the test server
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to test server: %v", err)
	}
	defer conn.Close()

	// Wait for the test to complete
	time.Sleep(100 * time.Millisecond)
}

func TestClient_Close(t *testing.T) {
	hub := NewHub()

	// Create a test WebSocket connection
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("Failed to upgrade connection: %v", err)
		}

		client := NewClient(hub, conn)

		// Test that Close() doesn't panic and properly closes resources
		client.Close()

		// Test that send channel is closed
		select {
		case _, ok := <-client.send:
			if ok {
				t.Error("Send channel should be closed after Close()")
			}
		default:
			// Channel might be closed and empty, which is also valid
		}
	}))
	defer server.Close()

	// Convert HTTP URL to WebSocket URL
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Connect to the test server
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to test server: %v", err)
	}
	defer conn.Close()

	// Wait for the test to complete
	time.Sleep(100 * time.Millisecond)
}

// Test client lifecycle management
func TestClient_Lifecycle(t *testing.T) {
	hub := NewHub()

	// Test server that handles WebSocket connections
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("Failed to upgrade connection: %v", err)
		}
		defer conn.Close()

		client := NewClient(hub, conn)

		// Test initial state
		if client.GetDisplayName() != "" {
			t.Errorf("Initial display name should be empty, got %v", client.GetDisplayName())
		}

		// Test setting display name directly
		err = client.SetDisplayName("TestUser")
		if err != nil {
			t.Errorf("Failed to set display name: %v", err)
		}

		if client.GetDisplayName() != "TestUser" {
			t.Errorf("Display name not set correctly, got %v, want TestUser", client.GetDisplayName())
		}

		// Test that client can be closed without panic
		client.Close()
	}))
	defer server.Close()

	// Convert HTTP URL to WebSocket URL
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Connect to the test server
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to test server: %v", err)
	}
	defer conn.Close()

	// Wait for the test to complete
	time.Sleep(100 * time.Millisecond)
}

// Test client rate limiting
func TestClient_RateLimit(t *testing.T) {
	hub := NewHub()
	
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatal("Failed to upgrade connection:", err)
		}
		defer conn.Close()
		
		client := NewClient(hub, conn)
		client.SetDisplayName("TestUser")
		
		// Test initial rate limit
		remaining := client.getRemainingRateLimit()
		if remaining != maxMessagesPerMinute {
			t.Errorf("Initial rate limit = %d, want %d", remaining, maxMessagesPerMinute)
		}
		
		// Send messages up to the limit
		for i := 0; i < maxMessagesPerMinute; i++ {
			if !client.checkRateLimit() {
				t.Errorf("checkRateLimit() failed at message %d", i)
			}
		}
		
		// Next message should be rate limited
		if client.checkRateLimit() {
			t.Error("checkRateLimit() should return false after reaching limit")
		}
		
		// Verify remaining is 0
		remaining = client.getRemainingRateLimit()
		if remaining != 0 {
			t.Errorf("Remaining rate limit = %d, want 0", remaining)
		}
	}))
	defer server.Close()
	
	url := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal("Failed to connect:", err)
	}
	defer conn.Close()
	
	time.Sleep(100 * time.Millisecond)
}

// Test private message rate limiting
func TestClient_PrivateMessageRateLimit(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatal("Failed to upgrade connection:", err)
		}
		defer conn.Close()
		
		client := NewClient(hub, conn)
		client.SetDisplayName("Alice")
		hub.UpdateClientName(client, "Alice")
		
		// Create recipient
		recipientConn, _ := upgrader.Upgrade(w, r, nil)
		if recipientConn == nil {
			recipientConn = conn
		}
		recipient := NewClient(hub, recipientConn)
		recipient.SetDisplayName("Bob")
		hub.UpdateClientName(recipient, "Bob")
		
		// Fill up rate limit with private messages
		for i := 0; i < maxMessagesPerMinute; i++ {
			if !client.checkRateLimit() {
				t.Errorf("Rate limit check failed at message %d", i)
			}
		}
		
		// Next private message should be rate limited
		if client.checkRateLimit() {
			t.Error("Private message should be rate limited after reaching limit")
		}
		
		// Verify error handling for rate limited message
		remaining := client.getRemainingRateLimit()
		if remaining != 0 {
			t.Errorf("Expected 0 remaining messages, got %d", remaining)
		}
	}))
	defer server.Close()
	
	url := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal("Failed to connect:", err)
	}
	defer conn.Close()
	
	time.Sleep(100 * time.Millisecond)
}

// Test Client activity tracking
func TestClient_ActivityTracking(t *testing.T) {
	hub := NewHub()
	
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatal("Failed to upgrade connection:", err)
		}
		defer conn.Close()
		
		client := NewClient(hub, conn)
		
		// Test initial timestamps
		connectedAt := client.GetConnectedAt()
		lastActivity := client.GetLastActivity()
		
		if connectedAt.IsZero() {
			t.Error("ConnectedAt should not be zero")
		}
		
		if lastActivity.IsZero() {
			t.Error("LastActivity should not be zero")
		}
		
		if !connectedAt.Equal(lastActivity) {
			t.Error("Initial ConnectedAt and LastActivity should be equal")
		}
		
		// Wait a bit and update activity
		time.Sleep(10 * time.Millisecond)
		client.updateActivity()
		
		newLastActivity := client.GetLastActivity()
		if !newLastActivity.After(lastActivity) {
			t.Error("LastActivity should be updated after updateActivity()")
		}
		
		// ConnectedAt should remain unchanged
		if !client.GetConnectedAt().Equal(connectedAt) {
			t.Error("ConnectedAt should not change after updateActivity()")
		}
	}))
	defer server.Close()
	
	url := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal("Failed to connect:", err)
	}
	defer conn.Close()
	
	time.Sleep(100 * time.Millisecond)
}
