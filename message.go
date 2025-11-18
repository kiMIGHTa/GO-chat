package main

import (
	"encoding/json"
	"errors"
	"html"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// Message type constants
const (
	MessageTypeChat     = "chat"
	MessageTypePrivate  = "private"
	MessageTypeSystem   = "system"
	MessageTypeUserList = "user_list"
	MessageTypeError    = "error"
	MessageTypeJoin     = "join"
)

// Message represents a WebSocket message with JSON schema
type Message struct {
	Type      string    `json:"type"`
	From      string    `json:"from,omitempty"`
	To        string    `json:"to,omitempty"`
	Content   string    `json:"content,omitempty"`
	Users     []string  `json:"users,omitempty"`
	Error     string    `json:"error,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// SetTimestamp sets the current time as the message timestamp
func (m *Message) SetTimestamp() {
	m.Timestamp = time.Now()
}

// Validate checks if the message has valid fields based on its type
func (m *Message) Validate() error {
	if m.Type == "" {
		return errors.New("message type is required")
	}

	// Validate message type is one of the allowed constants
	switch m.Type {
	case MessageTypeChat, MessageTypePrivate, MessageTypeSystem, MessageTypeUserList, MessageTypeError, MessageTypeJoin:
		// Valid type
	default:
		return errors.New("invalid message type")
	}

	// Type-specific validation
	switch m.Type {
	case MessageTypeChat:
		if err := validateMessageContent(m.Content); err != nil {
			return err
		}
		if err := validateDisplayName(m.From); err != nil {
			return errors.New("chat message sender invalid: " + err.Error())
		}
	case MessageTypePrivate:
		if err := validateMessageContent(m.Content); err != nil {
			return err
		}
		if err := validateDisplayName(m.From); err != nil {
			return errors.New("private message sender invalid: " + err.Error())
		}
		if m.To == "" {
			return errors.New("private message must have recipient (To field)")
		}
		if err := validateDisplayName(m.To); err != nil {
			return errors.New("private message recipient invalid: " + err.Error())
		}
		if m.From == m.To {
			return errors.New("cannot send private message to yourself")
		}
	case MessageTypeJoin:
		if err := validateDisplayName(m.Content); err != nil {
			return errors.New("join message display name invalid: " + err.Error())
		}
	case MessageTypeError:
		if m.Error == "" {
			return errors.New("error message must have error field")
		}
	case MessageTypeUserList:
		if m.Users == nil {
			return errors.New("user_list message must have users field")
		}
	}

	return nil
}

// validateDisplayName validates and sanitizes display names
func validateDisplayName(name string) error {
	// Trim whitespace
	trimmed := strings.TrimSpace(name)
	
	if trimmed == "" {
		return errors.New("display name cannot be empty")
	}
	
	// Check length limits
	if len(trimmed) > 50 {
		return errors.New("display name cannot exceed 50 characters")
	}
	
	// Check for valid UTF-8
	if !utf8.ValidString(trimmed) {
		return errors.New("display name contains invalid characters")
	}
	
	// Check for control characters (except normal whitespace)
	for _, r := range trimmed {
		if r < 32 && r != 9 && r != 10 && r != 13 { // Allow tab, newline, carriage return
			return errors.New("display name contains invalid control characters")
		}
	}
	
	// Check for HTML/script injection patterns
	htmlPattern := regexp.MustCompile(`(?i)<[^>]*>`)
	if htmlPattern.MatchString(trimmed) {
		return errors.New("display name cannot contain HTML tags")
	}
	
	// Check for javascript: URLs and other script injection patterns
	scriptPattern := regexp.MustCompile(`(?i)(javascript:|vbscript:|data:|on\w+\s*=)`)
	if scriptPattern.MatchString(trimmed) {
		return errors.New("display name contains prohibited script content")
	}
	
	return nil
}

// validateMessageContent validates and sanitizes message content
func validateMessageContent(content string) error {
	// Trim whitespace
	trimmed := strings.TrimSpace(content)
	
	if trimmed == "" {
		return errors.New("message content cannot be empty")
	}
	
	// Check length limits
	if len(trimmed) > 1000 {
		return errors.New("message content cannot exceed 1000 characters")
	}
	
	// Check for valid UTF-8
	if !utf8.ValidString(trimmed) {
		return errors.New("message content contains invalid characters")
	}
	
	return nil
}

// SanitizeInput sanitizes user input to prevent XSS attacks
func (m *Message) SanitizeInput() {
	// Sanitize display name and content by HTML escaping
	if m.From != "" {
		m.From = html.EscapeString(strings.TrimSpace(m.From))
	}
	if m.To != "" {
		m.To = html.EscapeString(strings.TrimSpace(m.To))
	}
	if m.Content != "" {
		m.Content = html.EscapeString(strings.TrimSpace(m.Content))
	}
	if m.Error != "" {
		m.Error = html.EscapeString(strings.TrimSpace(m.Error))
	}
	
	// Sanitize user list
	for i, user := range m.Users {
		m.Users[i] = html.EscapeString(strings.TrimSpace(user))
	}
}

// ToJSON converts the message to JSON bytes
func (m *Message) ToJSON() ([]byte, error) {
	return json.Marshal(m)
}

// MessageFromJSON creates a Message from JSON bytes
func MessageFromJSON(data []byte) (*Message, error) {
	var message Message
	err := json.Unmarshal(data, &message)
	if err != nil {
		return nil, err
	}
	return &message, nil
}
