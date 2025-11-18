package main

import (
	"errors"
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

// PrivateMessageRequest represents a request to send a private message
type PrivateMessageRequest struct {
	From    string
	To      string
	Message Message
}

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
	
	// Private messaging support
	privateMessage chan PrivateMessageRequest
	clientsByName  map[string]*Client
}

// NewHub creates a new Hub instance
func NewHub() *Hub {
	hub := &Hub{
		clients:        make(map[*Client]bool),
		broadcast:      make(chan []byte),
		register:       make(chan *Client),
		unregister:     make(chan *Client),
		userList:       make(map[*Client]string),
		stop:           make(chan struct{}),
		cleanupTicker:  time.NewTicker(cleanupInterval),
		privateMessage: make(chan PrivateMessageRequest),
		clientsByName:  make(map[string]*Client),
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
		case req := <-h.privateMessage:
			func() {
				defer func() {
					if r := recover(); r != nil {
						// Log panic with full context
						log.Printf("[PRIVATE_MSG] Panic recovered: from=%s to=%s error=%v", 
							req.From, req.To, r)
					}
				}()
				
				// Log private message request received
				log.Printf("[PRIVATE_MSG] Request received: from=%s to=%s", req.From, req.To)
				
				// Route private message to specific recipient
				err := h.SendPrivateMessage(req.From, req.To, req.Message)
				if err != nil {
					// Log all private message errors with context
					log.Printf("[PRIVATE_MSG] Routing error: from=%s to=%s error=%s error_type=routing_failure", 
						req.From, req.To, err.Error())
					
					// Send error message back to sender if routing fails
					sender, ok := h.GetClientByName(req.From)
					if ok {
						errorMsg := &Message{
							Type:  MessageTypeError,
							Error: "Failed to send private message: " + err.Error(),
						}
						errorMsg.SetTimestamp()
						errorData, jsonErr := errorMsg.ToJSON()
						if jsonErr == nil {
							select {
							case sender.send <- errorData:
								log.Printf("[PRIVATE_MSG] Error notification sent: from=%s to=%s", 
									req.From, req.To)
							default:
								log.Printf("[PRIVATE_MSG] Error notification failed: from=%s to=%s reason=channel_full", 
									req.From, req.To)
							}
						} else {
							log.Printf("[PRIVATE_MSG] Error message marshal failed: from=%s to=%s error=%v", 
								req.From, req.To, jsonErr)
						}
					} else {
						log.Printf("[PRIVATE_MSG] Sender not found for error notification: from=%s to=%s", 
							req.From, req.To)
					}
				}
			}()
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
					
					// Remove from user list and clientsByName map
					h.mu.Lock()
					displayName := h.userList[client]
					delete(h.userList, client)
					delete(h.clientsByName, displayName)
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
							displayName := h.userList[client]
							delete(h.userList, client)
							delete(h.clientsByName, displayName)
							h.mu.Unlock()
						}()
					}
				}
			}()
		}
	}
}

// UpdateClientName updates the clientsByName mapping for a client
func (h *Hub) UpdateClientName(client *Client, name string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clientsByName[name] = client
}

// GetClientByName retrieves a client by display name
func (h *Hub) GetClientByName(name string) (*Client, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	client, ok := h.clientsByName[name]
	return client, ok
}

// SendPrivateMessage routes a private message to a specific user
func (h *Hub) SendPrivateMessage(from, to string, message Message) error {
	// Log private message routing attempt
	log.Printf("[PRIVATE_MSG] Routing attempt: from=%s to=%s content_length=%d", 
		from, to, len(message.Content))
	
	// Validate recipient exists
	recipient, ok := h.GetClientByName(to)
	if !ok {
		// Log recipient lookup failure with context
		log.Printf("[PRIVATE_MSG] Recipient lookup failed: from=%s to=%s error=recipient_not_found", 
			from, to)
		return errors.New("recipient not found or offline")
	}
	
	// Get sender for echo
	sender, senderExists := h.GetClientByName(from)
	
	// Convert message to JSON
	jsonData, err := message.ToJSON()
	if err != nil {
		// Log JSON conversion error with context
		log.Printf("[PRIVATE_MSG] JSON conversion error: from=%s to=%s error=%v", 
			from, to, err)
		return err
	}
	
	// Send message to recipient with error handling for closed channels
	select {
	case recipient.send <- jsonData:
		// Log successful delivery with context
		log.Printf("[PRIVATE_MSG] Delivered successfully: from=%s to=%s content_length=%d", 
			from, to, len(message.Content))
	default:
		// Log delivery failure with context
		log.Printf("[PRIVATE_MSG] Delivery failed: from=%s to=%s error=channel_full_or_closed", 
			from, to)
		return errors.New("failed to deliver message to recipient")
	}
	
	// Send echo copy to sender if sender exists
	if senderExists {
		select {
		case sender.send <- jsonData:
			// Log successful echo with context
			log.Printf("[PRIVATE_MSG] Echo sent: from=%s to=%s", from, to)
		default:
			// Log echo failure (non-critical) with context
			log.Printf("[PRIVATE_MSG] Echo failed (non-critical): from=%s to=%s error=channel_full_or_closed", 
				from, to)
			// Don't return error for echo failure - message was delivered to recipient
		}
	} else {
		// Log sender not found for echo
		log.Printf("[PRIVATE_MSG] Echo skipped: from=%s to=%s reason=sender_not_found", 
			from, to)
	}
	
	return nil
}

// RegisterClient registers a new client with the hub and broadcasts join message
func (h *Hub) RegisterClient(client *Client, displayName string) {
	// Add to user list first
	h.mu.Lock()
	h.userList[client] = displayName
	h.mu.Unlock()
	
	// Update clientsByName mapping
	h.UpdateClientName(client, displayName)
	
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