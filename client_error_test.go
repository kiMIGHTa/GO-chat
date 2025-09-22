package main

import (
	"strings"
	"testing"
	"time"
)

// TestClientDisplayNameValidation tests display name validation
func TestClientDisplayNameValidation(t *testing.T) {
	client := &Client{}
	
	tests := []struct {
		name        string
		displayName string
		expectError bool
	}{
		{"Valid name", "JohnDoe", false},
		{"Empty name", "", true},
		{"Whitespace only", "   ", true},
		{"Too long name", strings.Repeat("a", 51), true},
		{"Max length name", strings.Repeat("a", 50), false},
		{"Name with spaces", "John Doe", false},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.SetDisplayName(tt.displayName)
			if tt.expectError && err == nil {
				t.Errorf("Expected error for display name '%s', got nil", tt.displayName)
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error for display name '%s', got %v", tt.displayName, err)
			}
		})
	}
}

// TestClientSendErrorMessage tests error message sending
func TestClientSendErrorMessage(t *testing.T) {
	hub := NewHub()
	client := &Client{
		hub:         hub,
		send:        make(chan []byte, 1),
		displayName: "test-user",
	}
	
	errorMsg := &Message{
		Type:  MessageTypeError,
		Error: "Test error message",
	}
	errorMsg.SetTimestamp()
	
	// Send error message
	client.sendErrorMessage(errorMsg)
	
	// Check if message was sent
	select {
	case data := <-client.send:
		// Verify the message was properly formatted
		message, err := MessageFromJSON(data)
		if err != nil {
			t.Errorf("Failed to parse sent error message: %v", err)
		}
		
		if message.Type != MessageTypeError {
			t.Errorf("Expected error message type, got %s", message.Type)
		}
		
		if message.Error != "Test error message" {
			t.Errorf("Expected 'Test error message', got '%s'", message.Error)
		}
	default:
		t.Error("Error message was not sent to client")
	}
}

// TestClientSendErrorMessageFullChannel tests error handling when send channel is full
func TestClientSendErrorMessageFullChannel(t *testing.T) {
	hub := NewHub()
	client := &Client{
		hub:         hub,
		send:        make(chan []byte, 1), // Small buffer
		displayName: "test-user",
	}
	
	// Fill the channel
	client.send <- []byte("blocking message")
	
	errorMsg := &Message{
		Type:  MessageTypeError,
		Error: "Test error message",
	}
	errorMsg.SetTimestamp()
	
	// This should not block or panic
	client.sendErrorMessage(errorMsg)
	
	// Channel should still have the original message
	select {
	case data := <-client.send:
		if string(data) != "blocking message" {
			t.Error("Original message was not preserved when error send failed")
		}
	default:
		t.Error("Channel should still contain the blocking message")
	}
}

// TestMessageValidationErrors tests various message validation scenarios
func TestMessageValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		message Message
		valid   bool
	}{
		{
			"Valid chat message",
			Message{Type: MessageTypeChat, From: "user", Content: "hello"},
			true,
		},
		{
			"Empty message type",
			Message{From: "user", Content: "hello"},
			false,
		},
		{
			"Invalid message type",
			Message{Type: "invalid", From: "user", Content: "hello"},
			false,
		},
		{
			"Chat message without sender",
			Message{Type: MessageTypeChat, Content: "hello"},
			false,
		},
		{
			"Chat message without content",
			Message{Type: MessageTypeChat, From: "user"},
			false,
		},
		{
			"Chat message with empty content",
			Message{Type: MessageTypeChat, From: "user", Content: "   "},
			false,
		},
		{
			"Join message without content",
			Message{Type: MessageTypeJoin},
			false,
		},
		{
			"Error message without error field",
			Message{Type: MessageTypeError},
			false,
		},
		{
			"User list message without users field",
			Message{Type: MessageTypeUserList},
			false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.message.Validate()
			if tt.valid && err != nil {
				t.Errorf("Expected valid message, got error: %v", err)
			}
			if !tt.valid && err == nil {
				t.Error("Expected validation error, got nil")
			}
		})
	}
}

// TestMalformedJSONHandling tests handling of malformed JSON
func TestMalformedJSONHandling(t *testing.T) {
	malformedJSONs := []string{
		`{"type": "chat", "from": "user", "content": "hello"`, // Missing closing brace
		`{"type": "chat" "from": "user", "content": "hello"}`, // Missing comma
		`{type: "chat", "from": "user", "content": "hello"}`,  // Unquoted key
		`{"type": "chat", "from": "user", "content": }`,       // Missing value
		`not json at all`,                                      // Not JSON
		``,                                                     // Empty string
	}
	
	for i, jsonStr := range malformedJSONs {
		t.Run("Malformed JSON "+string(rune(i)), func(t *testing.T) {
			_, err := MessageFromJSON([]byte(jsonStr))
			if err == nil {
				t.Errorf("Expected error for malformed JSON: %s", jsonStr)
			}
		})
	}
}

// TestXSSPreventionValidation tests XSS prevention in validation
func TestXSSPreventionValidation(t *testing.T) {
	xssPayloads := []struct {
		name    string
		payload string
		field   string // "displayName" or "content"
	}{
		{"Script tag", "<script>alert('xss')</script>", "displayName"},
		{"Image onerror", "<img src=x onerror=alert('xss')>", "displayName"},
		{"JavaScript URL", "javascript:alert('xss')", "displayName"},
		{"Event handler", "<div onclick=alert('xss')>", "displayName"},
		{"SVG script", "<svg onload=alert('xss')>", "displayName"},
		{"Script in content", "Hello <script>alert('xss')</script>", "content"},
		{"Iframe", "<iframe src=javascript:alert('xss')>", "displayName"},
		{"Object", "<object data=javascript:alert('xss')>", "displayName"},
		{"Embed", "<embed src=javascript:alert('xss')>", "displayName"},
		{"Link javascript", "<a href=javascript:alert('xss')>", "displayName"},
	}
	
	for _, tt := range xssPayloads {
		t.Run(tt.name, func(t *testing.T) {
			if tt.field == "displayName" {
				err := validateDisplayName(tt.payload)
				if err == nil {
					t.Errorf("Expected validation error for XSS payload in display name: %s", tt.payload)
				}
			} else if tt.field == "content" {
				// Content validation doesn't reject HTML, but sanitization should handle it
				message := &Message{
					Type:    MessageTypeChat,
					From:    "testuser",
					Content: tt.payload,
				}
				message.SanitizeInput()
				
				// After sanitization, the content should not contain executable script
				if strings.Contains(message.Content, "<script>") {
					t.Errorf("Sanitization failed to escape script tag in: %s", tt.payload)
				}
			}
		})
	}
}

// TestSpecialCharacterHandling tests handling of special characters and emojis
func TestSpecialCharacterHandling(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		field       string // "displayName" or "content"
		shouldPass  bool
		description string
	}{
		{"Emoji in display name", "User😀", "displayName", true, "Emojis should be allowed"},
		{"Unicode characters", "Üser", "displayName", true, "Unicode should be allowed"},
		{"Chinese characters", "用户", "displayName", true, "Chinese characters should be allowed"},
		{"Arabic characters", "مستخدم", "displayName", true, "Arabic characters should be allowed"},
		{"Russian characters", "Пользователь", "displayName", true, "Russian characters should be allowed"},
		{"Mixed emoji and text", "Hello 🌍 World", "content", true, "Mixed content should be allowed"},
		{"Mathematical symbols", "∑∆∇∂", "displayName", true, "Math symbols should be allowed"},
		{"Currency symbols", "$€£¥", "displayName", true, "Currency symbols should be allowed"},
		{"Control character null", "User\x00", "displayName", false, "Null character should be rejected"},
		{"Control character bell", "User\x07", "displayName", false, "Bell character should be rejected"},
		{"Control character escape", "User\x1B", "displayName", false, "Escape character should be rejected"},
		{"Tab character", "User\tName", "displayName", true, "Tab should be allowed"},
		{"Newline character", "User\nName", "displayName", true, "Newline should be allowed"},
		{"Carriage return", "User\rName", "displayName", true, "Carriage return should be allowed"},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			if tt.field == "displayName" {
				err = validateDisplayName(tt.input)
			} else {
				err = validateMessageContent(tt.input)
			}
			
			if tt.shouldPass && err != nil {
				t.Errorf("%s: Expected to pass but got error: %v", tt.description, err)
			}
			if !tt.shouldPass && err == nil {
				t.Errorf("%s: Expected to fail but passed", tt.description)
			}
		})
	}
}

// TestInputSanitizationEdgeCases tests edge cases in input sanitization
func TestInputSanitizationEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			"HTML entities",
			"&lt;script&gt;alert('test')&lt;/script&gt;",
			"&amp;lt;script&amp;gt;alert(&#39;test&#39;)&amp;lt;/script&amp;gt;",
		},
		{
			"Mixed quotes",
			`He said "Hello 'World'"`,
			"He said &#34;Hello &#39;World&#39;&#34;",
		},
		{
			"Ampersand",
			"Tom & Jerry",
			"Tom &amp; Jerry",
		},
		{
			"Less than and greater than",
			"5 < 10 > 3",
			"5 &lt; 10 &gt; 3",
		},
		{
			"Empty string",
			"",
			"",
		},
		{
			"Whitespace only",
			"   ",
			"",
		},
		{
			"Leading and trailing whitespace",
			"  Hello World  ",
			"Hello World",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message := &Message{
				Type:    MessageTypeChat,
				From:    "testuser",
				Content: tt.input,
			}
			message.SanitizeInput()
			
			if message.Content != tt.expected {
				t.Errorf("SanitizeInput() = %v, want %v", message.Content, tt.expected)
			}
		})
	}
}

// TestValidationPerformance tests validation performance with large inputs
func TestValidationPerformance(t *testing.T) {
	// Test with maximum allowed lengths
	maxDisplayName := strings.Repeat("a", 50)
	maxContent := strings.Repeat("a", 1000)
	
	// Test display name validation performance
	start := time.Now()
	for i := 0; i < 1000; i++ {
		validateDisplayName(maxDisplayName)
	}
	displayNameDuration := time.Since(start)
	
	// Test content validation performance
	start = time.Now()
	for i := 0; i < 1000; i++ {
		validateMessageContent(maxContent)
	}
	contentDuration := time.Since(start)
	
	// Performance should be reasonable (less than 100ms for 1000 validations)
	if displayNameDuration > 100*time.Millisecond {
		t.Errorf("Display name validation too slow: %v", displayNameDuration)
	}
	if contentDuration > 100*time.Millisecond {
		t.Errorf("Content validation too slow: %v", contentDuration)
	}
	
	t.Logf("Display name validation: %v for 1000 iterations", displayNameDuration)
	t.Logf("Content validation: %v for 1000 iterations", contentDuration)
}