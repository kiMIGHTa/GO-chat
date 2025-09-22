package main

import (
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// Time allowed to write a message to the peer
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer
	maxMessageSize = 1024

	// Rate limiting constants
	maxMessagesPerMinute = 30
	rateLimitWindow      = time.Minute
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Allow connections from any origin for development
		return true
	},
}

// Client represents a WebSocket client connection
type Client struct {
	// The hub that manages this client
	hub *Hub

	// The WebSocket connection
	conn *websocket.Conn

	// Buffered channel of outbound messages
	send chan []byte

	// Display name for this client
	displayName string

	// Rate limiting fields
	messageTimestamps []time.Time
	rateLimitMu       sync.Mutex

	// Connection metadata for monitoring
	connectedAt time.Time
	lastActivity time.Time
	activityMu   sync.RWMutex
}

// SetDisplayName validates and sets the display name for the client
func (c *Client) SetDisplayName(name string) error {
	// Use the enhanced validation function
	if err := validateDisplayName(name); err != nil {
		return err
	}

	// Sanitize and set the display name
	c.displayName = strings.TrimSpace(name)
	return nil
}

// GetDisplayName returns the client's display name
func (c *Client) GetDisplayName() string {
	return c.displayName
}

// updateActivity updates the last activity timestamp
func (c *Client) updateActivity() {
	c.activityMu.Lock()
	c.lastActivity = time.Now()
	c.activityMu.Unlock()
}

// GetLastActivity returns the last activity timestamp
func (c *Client) GetLastActivity() time.Time {
	c.activityMu.RLock()
	defer c.activityMu.RUnlock()
	return c.lastActivity
}

// GetConnectedAt returns when the client connected
func (c *Client) GetConnectedAt() time.Time {
	return c.connectedAt
}

// checkRateLimit checks if the client is within rate limits
func (c *Client) checkRateLimit() bool {
	c.rateLimitMu.Lock()
	defer c.rateLimitMu.Unlock()

	now := time.Now()
	
	// Remove timestamps older than the rate limit window
	cutoff := now.Add(-rateLimitWindow)
	validTimestamps := make([]time.Time, 0, len(c.messageTimestamps))
	
	for _, timestamp := range c.messageTimestamps {
		if timestamp.After(cutoff) {
			validTimestamps = append(validTimestamps, timestamp)
		}
	}
	
	c.messageTimestamps = validTimestamps
	
	// Check if we're at the limit
	if len(c.messageTimestamps) >= maxMessagesPerMinute {
		return false
	}
	
	// Add current timestamp
	c.messageTimestamps = append(c.messageTimestamps, now)
	return true
}

// getRemainingRateLimit returns how many messages the client can send
func (c *Client) getRemainingRateLimit() int {
	c.rateLimitMu.Lock()
	defer c.rateLimitMu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rateLimitWindow)
	
	// Count valid timestamps
	validCount := 0
	for _, timestamp := range c.messageTimestamps {
		if timestamp.After(cutoff) {
			validCount++
		}
	}
	
	remaining := maxMessagesPerMinute - validCount
	if remaining < 0 {
		return 0
	}
	return remaining
}

// ReadPump pumps messages from the WebSocket connection to the hub
func (c *Client) ReadPump() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("ReadPump panic recovered for client %s: %v", c.GetDisplayName(), r)
		}
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, messageData, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		// Parse the incoming message with enhanced error handling
		message, err := MessageFromJSON(messageData)
		if err != nil {
			log.Printf("JSON parsing error from client %s: %v", c.GetDisplayName(), err)
			// Send detailed error message back to client
			errorMsg := &Message{
				Type:  MessageTypeError,
				Error: "Invalid message format: " + err.Error(),
			}
			errorMsg.SetTimestamp()
			c.sendErrorMessage(errorMsg)
			continue
		}

		// Validate message structure
		if err := message.Validate(); err != nil {
			log.Printf("Message validation error from client %s: %v", c.GetDisplayName(), err)
			errorMsg := &Message{
				Type:  MessageTypeError,
				Error: "Message validation failed: " + err.Error(),
			}
			errorMsg.SetTimestamp()
			c.sendErrorMessage(errorMsg)
			continue
		}

		// Update activity timestamp
		c.updateActivity()

		// Handle different message types with enhanced error handling
		switch message.Type {
		case MessageTypeJoin:
			// Set display name from join message
			if err := c.SetDisplayName(message.Content); err != nil {
				log.Printf("Display name validation error from client: %v", err)
				errorMsg := &Message{
					Type:  MessageTypeError,
					Error: "Display name error: " + err.Error(),
				}
				errorMsg.SetTimestamp()
				c.sendErrorMessage(errorMsg)
				continue
			}

			// Register client with hub
			log.Printf("Client %s joining chat", c.displayName)
			c.hub.RegisterClient(c, c.displayName)

		case MessageTypeChat:
			// Additional validation for chat messages
			if c.displayName == "" {
				errorMsg := &Message{
					Type:  MessageTypeError,
					Error: "Must join chat before sending messages",
				}
				errorMsg.SetTimestamp()
				c.sendErrorMessage(errorMsg)
				continue
			}

			// Check rate limiting
			if !c.checkRateLimit() {
				remaining := c.getRemainingRateLimit()
				log.Printf("Rate limit exceeded for client %s, remaining: %d", c.displayName, remaining)
				errorMsg := &Message{
					Type:  MessageTypeError,
					Error: "Rate limit exceeded. Please slow down your messages.",
				}
				errorMsg.SetTimestamp()
				c.sendErrorMessage(errorMsg)
				continue
			}

			// Validate message content using enhanced validation
			if err := validateMessageContent(message.Content); err != nil {
				log.Printf("Message content validation error from client %s: %v", c.displayName, err)
				errorMsg := &Message{
					Type:  MessageTypeError,
					Error: "Message validation failed: " + err.Error(),
				}
				errorMsg.SetTimestamp()
				c.sendErrorMessage(errorMsg)
				continue
			}

			// Set sender and timestamp
			message.From = c.displayName
			message.SetTimestamp()

			// Sanitize input to prevent XSS
			message.SanitizeInput()

			// Broadcast message through hub
			log.Printf("Broadcasting message from %s (remaining rate limit: %d)", c.displayName, c.getRemainingRateLimit())
			c.hub.BroadcastMessage(*message)

		default:
			// Unknown message type
			log.Printf("Unknown message type '%s' from client %s", message.Type, c.GetDisplayName())
			errorMsg := &Message{
				Type:  MessageTypeError,
				Error: "Unknown message type: " + message.Type,
			}
			errorMsg.SetTimestamp()
			c.sendErrorMessage(errorMsg)
		}
	}
}

// WritePump pumps messages from the hub to the WebSocket connection
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		if r := recover(); r != nil {
			log.Printf("WritePump panic recovered for client %s: %v", c.GetDisplayName(), r)
		}
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// Send the message directly as a single WebSocket text message
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

			// Process any additional queued messages individually
			n := len(c.send)
			for i := 0; i < n; i++ {
				select {
				case queuedMessage := <-c.send:
					c.conn.SetWriteDeadline(time.Now().Add(writeWait))
					if err := c.conn.WriteMessage(websocket.TextMessage, queuedMessage); err != nil {
						return
					}
				default:
					break
				}
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// Close gracefully closes the client connection and cleans up resources
func (c *Client) Close() {
	// Close the send channel to signal WritePump to stop
	close(c.send)

	// Close the WebSocket connection
	c.conn.Close()
}

// sendErrorMessage safely sends an error message to the client
func (c *Client) sendErrorMessage(errorMsg *Message) {
	if jsonData, jsonErr := errorMsg.ToJSON(); jsonErr == nil {
		select {
		case c.send <- jsonData:
		default:
			log.Printf("Failed to send error message to client %s: send channel full", c.GetDisplayName())
			// Don't close the connection here, just log the failure
		}
	} else {
		log.Printf("Failed to marshal error message for client %s: %v", c.GetDisplayName(), jsonErr)
	}
}

// NewClient creates a new client instance
func NewClient(hub *Hub, conn *websocket.Conn) *Client {
	now := time.Now()
	return &Client{
		hub:               hub,
		conn:              conn,
		send:              make(chan []byte, 256),
		messageTimestamps: make([]time.Time, 0),
		connectedAt:       now,
		lastActivity:      now,
	}
}
