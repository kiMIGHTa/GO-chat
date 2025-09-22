package main

import (
	"log"
	"sync"
	"time"
)

const (
	// Maximum number of concurrent connections
	maxConcurrentConnections = 1000
	
	// Connection cleanup interval
	cleanupInterval = 5 * time.Minute
	
	// Idle connection timeout
	idleTimeout = 30 * time.Minute
)

// Hub maintains the set of active clients and broadcasts messages to the clients
type Hub struct {
	// Registered clients
	clients map[*Client]bool

	// Inbound messages from the clients
	broadcast chan []byte

	// Register requests from the clients
	register chan *Client

	// Unregister requests from clients
	unregister chan *Client

	// Map of client to display name for user list management
	userList map[*Client]string

	// Mutex to protect concurrent access to userList
	mu sync.RWMutex

	// Stop channel for graceful shutdown
	stop chan struct{}
	
	// Cleanup ticker for periodic maintenance
	cleanupTicker *time.Ticker
}

// NewHub creates a new Hub instance
func NewHub() *Hub {
	hub := &Hub{
		clients:       make(map[*Client]bool),
		broadcast:     make(chan []byte),
		register:      make(chan *Client),
		unregister:    make(chan *Client),
		userList:      make(map[*Client]string),
		stop:          make(chan struct{}),
		cleanupTicker: time.NewTicker(cleanupInterval),
	}
	return hub
}

// Run starts the hub's main event loop with panic recovery
func (h *Hub) Run() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Hub panic recovered: %v", r)
			// Restart the hub after a brief delay
			time.Sleep(1 * time.Second)
			go h.Run()
		}
	}()

	for {
		select {
		case <-h.stop:
			// Graceful shutdown
			log.Println("Hub stopping...")
			h.cleanupTicker.Stop()
			return
		case <-h.cleanupTicker.C:
			// Periodic cleanup of idle connections
			h.cleanupIdleConnections()
		case client := <-h.register:
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("Client registration panic recovered: %v", r)
					}
				}()
				h.clients[client] = true
				log.Printf("Client registered: %s", client.GetDisplayName())
			}()

		case client := <-h.unregister:
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("Client unregistration panic recovered: %v", r)
					}
				}()
				if _, ok := h.clients[client]; ok {
					delete(h.clients, client)
					
					// Safely close the send channel
					func() {
						defer func() {
							if r := recover(); r != nil {
								log.Printf("Error closing client send channel: %v", r)
							}
						}()
						close(client.send)
					}()
					
					// Remove from user list and broadcast updated list
					h.mu.Lock()
					displayName := h.userList[client]
					delete(h.userList, client)
					h.mu.Unlock()
					
					log.Printf("Client unregistered: %s", displayName)
					
					// Broadcast system message about user leaving
					if displayName != "" {
						systemMsg := &Message{
							Type:    MessageTypeSystem,
							Content: displayName + " has left the chat",
						}
						systemMsg.SetTimestamp()
						h.BroadcastMessage(*systemMsg)
					}
					
					// Broadcast updated user list
					h.BroadcastUserList()
				}
			}()

		case message := <-h.broadcast:
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("Message broadcast panic recovered: %v", r)
					}
				}()
				// Broadcast message to all clients
				for client := range h.clients {
					select {
					case client.send <- message:
					default:
						// Client send channel is full or closed, clean up
						func() {
							defer func() {
								if r := recover(); r != nil {
									log.Printf("Error cleaning up client during broadcast: %v", r)
								}
							}()
							close(client.send)
							delete(h.clients, client)
							h.mu.Lock()
							delete(h.userList, client)
							h.mu.Unlock()
						}()
					}
				}
			}()
		}
	}
}

// RegisterClient registers a new client with the hub and broadcasts join message
func (h *Hub) RegisterClient(client *Client, displayName string) {
	// Add to user list first
	h.mu.Lock()
	h.userList[client] = displayName
	h.mu.Unlock()
	
	// Register the client
	h.register <- client
	
	// Give a small delay to ensure registration is processed
	time.Sleep(50 * time.Millisecond)
	
	// Broadcast system message about user joining
	systemMsg := &Message{
		Type:    MessageTypeSystem,
		Content: displayName + " has joined the chat",
	}
	systemMsg.SetTimestamp()
	h.BroadcastMessage(*systemMsg)
	
	// Give another small delay before broadcasting user list
	time.Sleep(10 * time.Millisecond)
	
	// Broadcast updated user list to all clients
	h.BroadcastUserList()
}

// UnregisterClient removes a client from the hub
func (h *Hub) UnregisterClient(client *Client) {
	h.unregister <- client
}

// BroadcastMessage sends a message to all connected clients
func (h *Hub) BroadcastMessage(message Message) {
	// Convert message to JSON
	jsonData, err := message.ToJSON()
	if err != nil {
		log.Printf("Error converting message to JSON: %v", err)
		return
	}
	
	// Send to broadcast channel
	h.broadcast <- jsonData
}

// BroadcastUserList sends the current list of online users to all clients
func (h *Hub) BroadcastUserList() {
	h.mu.RLock()
	users := make([]string, 0, len(h.userList))
	for _, displayName := range h.userList {
		users = append(users, displayName)
	}
	h.mu.RUnlock()
	
	// Create user list message
	userListMsg := &Message{
		Type:  MessageTypeUserList,
		Users: users,
	}
	userListMsg.SetTimestamp()
	
	// Broadcast the user list
	h.BroadcastMessage(*userListMsg)
}

// GetConnectedUsers returns a slice of currently connected user display names
func (h *Hub) GetConnectedUsers() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	
	users := make([]string, 0, len(h.userList))
	for _, displayName := range h.userList {
		users = append(users, displayName)
	}
	return users
}

// GetClientCount returns the number of currently connected clients
func (h *Hub) GetClientCount() int {
	return len(h.clients)
}

// Stop gracefully stops the hub
func (h *Hub) Stop() {
	if h.cleanupTicker != nil {
		h.cleanupTicker.Stop()
	}
	close(h.stop)
}

// cleanupIdleConnections removes idle connections to free up resources
func (h *Hub) cleanupIdleConnections() {
	now := time.Now()
	idleClients := make([]*Client, 0)
	
	// Find idle clients
	for client := range h.clients {
		if now.Sub(client.GetLastActivity()) > idleTimeout {
			idleClients = append(idleClients, client)
		}
	}
	
	// Remove idle clients
	for _, client := range idleClients {
		log.Printf("Removing idle client: %s (idle for %v)", 
			client.GetDisplayName(), now.Sub(client.GetLastActivity()))
		h.UnregisterClient(client)
		client.Close()
	}
	
	if len(idleClients) > 0 {
		log.Printf("Cleaned up %d idle connections", len(idleClients))
	}
}

// CanAcceptNewConnection checks if the hub can accept a new connection
func (h *Hub) CanAcceptNewConnection() bool {
	return len(h.clients) < maxConcurrentConnections
}

// GetConnectionStats returns connection statistics for monitoring
func (h *Hub) GetConnectionStats() map[string]interface{} {
	h.mu.RLock()
	defer h.mu.RUnlock()
	
	now := time.Now()
	activeConnections := 0
	idleConnections := 0
	
	for client := range h.clients {
		if now.Sub(client.GetLastActivity()) > 5*time.Minute {
			idleConnections++
		} else {
			activeConnections++
		}
	}
	
	return map[string]interface{}{
		"total_connections":  len(h.clients),
		"active_connections": activeConnections,
		"idle_connections":   idleConnections,
		"max_connections":    maxConcurrentConnections,
		"users_online":       len(h.userList),
	}
}