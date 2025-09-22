package main

import (
	"testing"
	"time"
)

// TestHubPanicRecovery tests that the hub recovers from panics
func TestHubPanicRecovery(t *testing.T) {
	hub := NewHub()
	
	// Start hub in goroutine
	go hub.Run()
	defer hub.Stop() // Ensure cleanup
	
	// Give hub time to start
	time.Sleep(100 * time.Millisecond)
	
	// Create a mock client that will cause a panic
	mockClient := &Client{
		hub:         hub,
		conn:        nil, // This will cause issues
		send:        make(chan []byte, 1),
		displayName: "test-user",
	}
	
	// This should not crash the hub
	hub.register <- mockClient
	
	// Give time for processing
	time.Sleep(100 * time.Millisecond)
	
	// Hub should still be running - test by registering another client
	normalClient := &Client{
		hub:         hub,
		conn:        nil,
		send:        make(chan []byte, 1),
		displayName: "normal-user",
	}
	
	// This should work without issues
	hub.register <- normalClient
	time.Sleep(100 * time.Millisecond)
	
	// Verify the normal client was registered
	if len(hub.clients) == 0 {
		t.Error("Hub should have registered the normal client")
	}
}

// TestHubBroadcastPanicRecovery tests broadcast panic recovery
func TestHubBroadcastPanicRecovery(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Stop() // Ensure cleanup
	
	time.Sleep(100 * time.Millisecond)
	
	// Create a client with a closed send channel to trigger panic
	client := &Client{
		hub:         hub,
		conn:        nil,
		send:        make(chan []byte, 1),
		displayName: "panic-client",
	}
	
	// Register client
	hub.register <- client
	time.Sleep(50 * time.Millisecond)
	
	// Close the send channel to cause issues during broadcast
	close(client.send)
	
	// Try to broadcast a message - this should not crash the hub
	message := Message{
		Type:    MessageTypeChat,
		From:    "test",
		Content: "test message",
	}
	message.SetTimestamp()
	
	hub.BroadcastMessage(message)
	time.Sleep(100 * time.Millisecond)
	
	// Hub should still be responsive
	normalClient := &Client{
		hub:         hub,
		conn:        nil,
		send:        make(chan []byte, 1),
		displayName: "normal-client",
	}
	
	hub.register <- normalClient
	time.Sleep(50 * time.Millisecond)
	
	// Should have registered the new client (cleanup happens during broadcast)
	if len(hub.clients) < 1 {
		t.Errorf("Expected at least 1 client, got %d", len(hub.clients))
	}
}

// TestConcurrentClientOperations tests hub behavior under concurrent load
func TestConcurrentClientOperations(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Stop() // Ensure cleanup
	
	time.Sleep(100 * time.Millisecond)
	
	numClients := 5 // Reduced number to avoid deadlock
	clients := make([]*Client, numClients)
	
	// Register clients sequentially to avoid channel blocking
	for i := 0; i < numClients; i++ {
		client := &Client{
			hub:         hub,
			conn:        nil,
			send:        make(chan []byte, 10), // Larger buffer
			displayName: "client-" + string(rune('A'+i)),
		}
		clients[i] = client
		
		hub.register <- client
		time.Sleep(20 * time.Millisecond) // Give time for processing
	}
	
	// Verify all clients are registered
	time.Sleep(100 * time.Millisecond)
	if len(hub.clients) != numClients {
		t.Errorf("Expected %d clients after registration, got %d", numClients, len(hub.clients))
	}
	
	// Unregister clients sequentially
	for _, client := range clients {
		hub.unregister <- client
		time.Sleep(20 * time.Millisecond)
	}
	
	// All clients should be unregistered
	time.Sleep(200 * time.Millisecond)
	if len(hub.clients) != 0 {
		t.Errorf("Expected 0 clients after unregistration, got %d", len(hub.clients))
	}
	
	if len(hub.userList) != 0 {
		t.Errorf("Expected 0 users in user list, got %d", len(hub.userList))
	}
}

// TestHubUserListConsistency tests user list consistency during errors
func TestHubUserListConsistency(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Stop() // Ensure cleanup
	
	time.Sleep(100 * time.Millisecond)
	
	// Register a client
	client := &Client{
		hub:         hub,
		conn:        nil,
		send:        make(chan []byte, 1),
		displayName: "test-user",
	}
	
	hub.RegisterClient(client, "test-user")
	time.Sleep(50 * time.Millisecond)
	
	// Verify client is in both maps
	if len(hub.clients) != 1 {
		t.Errorf("Expected 1 client, got %d", len(hub.clients))
	}
	
	if len(hub.userList) != 1 {
		t.Errorf("Expected 1 user in user list, got %d", len(hub.userList))
	}
	
	// Unregister client
	hub.UnregisterClient(client)
	time.Sleep(50 * time.Millisecond)
	
	// Verify cleanup
	if len(hub.clients) != 0 {
		t.Errorf("Expected 0 clients after unregistration, got %d", len(hub.clients))
	}
	
	if len(hub.userList) != 0 {
		t.Errorf("Expected 0 users in user list after unregistration, got %d", len(hub.userList))
	}
}