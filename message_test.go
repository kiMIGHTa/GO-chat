package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestMessage_SetTimestamp(t *testing.T) {
	message := &Message{
		Type:    MessageTypeChat,
		From:    "testuser",
		Content: "Hello world",
	}

	before := time.Now()
	message.SetTimestamp()
	after := time.Now()

	if message.Timestamp.Before(before) || message.Timestamp.After(after) {
		t.Errorf("SetTimestamp() did not set timestamp correctly. Got: %v", message.Timestamp)
	}
}

func TestMessage_Validate(t *testing.T) {
	tests := []struct {
		name    string
		message Message
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid chat message",
			message: Message{
				Type:    MessageTypeChat,
				From:    "testuser",
				Content: "Hello world",
			},
			wantErr: false,
		},
		{
			name: "valid join message",
			message: Message{
				Type:    MessageTypeJoin,
				Content: "testuser",
			},
			wantErr: false,
		},
		{
			name: "valid system message",
			message: Message{
				Type:    MessageTypeSystem,
				Content: "User joined",
			},
			wantErr: false,
		},
		{
			name: "valid user_list message",
			message: Message{
				Type:  MessageTypeUserList,
				Users: []string{"user1", "user2"},
			},
			wantErr: false,
		},
		{
			name: "valid error message",
			message: Message{
				Type:  MessageTypeError,
				Error: "Connection failed",
			},
			wantErr: false,
		},
		{
			name: "empty message type",
			message: Message{
				Content: "Hello",
			},
			wantErr: true,
			errMsg:  "message type is required",
		},
		{
			name: "invalid message type",
			message: Message{
				Type:    "invalid",
				Content: "Hello",
			},
			wantErr: true,
			errMsg:  "invalid message type",
		},
		{
			name: "chat message without content",
			message: Message{
				Type: MessageTypeChat,
				From: "testuser",
			},
			wantErr: true,
			errMsg:  "chat message content cannot be empty",
		},
		{
			name: "chat message with empty content",
			message: Message{
				Type:    MessageTypeChat,
				From:    "testuser",
				Content: "   ",
			},
			wantErr: true,
			errMsg:  "chat message content cannot be empty",
		},
		{
			name: "chat message without sender",
			message: Message{
				Type:    MessageTypeChat,
				Content: "Hello world",
			},
			wantErr: true,
			errMsg:  "chat message must have a sender",
		},
		{
			name: "join message without display name",
			message: Message{
				Type: MessageTypeJoin,
			},
			wantErr: true,
			errMsg:  "join message must include display name in content",
		},
		{
			name: "join message with empty display name",
			message: Message{
				Type:    MessageTypeJoin,
				Content: "   ",
			},
			wantErr: true,
			errMsg:  "join message must include display name in content",
		},
		{
			name: "error message without error field",
			message: Message{
				Type: MessageTypeError,
			},
			wantErr: true,
			errMsg:  "error message must have error field",
		},
		{
			name: "user_list message without users field",
			message: Message{
				Type: MessageTypeUserList,
			},
			wantErr: true,
			errMsg:  "user_list message must have users field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.message.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("Validate() expected error but got none")
					return
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %v, want error containing %v", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Validate() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestMessage_ToJSON(t *testing.T) {
	timestamp := time.Date(2023, 12, 25, 10, 30, 0, 0, time.UTC)
	message := &Message{
		Type:      MessageTypeChat,
		From:      "testuser",
		Content:   "Hello world",
		Timestamp: timestamp,
	}

	jsonData, err := message.ToJSON()
	if err != nil {
		t.Errorf("ToJSON() error = %v", err)
		return
	}

	// Verify JSON structure
	var result map[string]interface{}
	err = json.Unmarshal(jsonData, &result)
	if err != nil {
		t.Errorf("ToJSON() produced invalid JSON: %v", err)
		return
	}

	// Check required fields
	if result["type"] != MessageTypeChat {
		t.Errorf("ToJSON() type = %v, want %v", result["type"], MessageTypeChat)
	}
	if result["from"] != "testuser" {
		t.Errorf("ToJSON() from = %v, want %v", result["from"], "testuser")
	}
	if result["content"] != "Hello world" {
		t.Errorf("ToJSON() content = %v, want %v", result["content"], "Hello world")
	}
	if result["timestamp"] == nil {
		t.Errorf("ToJSON() timestamp should not be nil")
	}
}

func TestMessageFromJSON(t *testing.T) {
	tests := []struct {
		name     string
		jsonData string
		want     *Message
		wantErr  bool
	}{
		{
			name:     "valid chat message JSON",
			jsonData: `{"type":"chat","from":"testuser","content":"Hello world","timestamp":"2023-12-25T10:30:00Z"}`,
			want: &Message{
				Type:      MessageTypeChat,
				From:      "testuser",
				Content:   "Hello world",
				Timestamp: time.Date(2023, 12, 25, 10, 30, 0, 0, time.UTC),
			},
			wantErr: false,
		},
		{
			name:     "valid user_list message JSON",
			jsonData: `{"type":"user_list","users":["user1","user2"],"timestamp":"2023-12-25T10:30:00Z"}`,
			want: &Message{
				Type:      MessageTypeUserList,
				Users:     []string{"user1", "user2"},
				Timestamp: time.Date(2023, 12, 25, 10, 30, 0, 0, time.UTC),
			},
			wantErr: false,
		},
		{
			name:     "valid error message JSON",
			jsonData: `{"type":"error","error":"Connection failed","timestamp":"2023-12-25T10:30:00Z"}`,
			want: &Message{
				Type:      MessageTypeError,
				Error:     "Connection failed",
				Timestamp: time.Date(2023, 12, 25, 10, 30, 0, 0, time.UTC),
			},
			wantErr: false,
		},
		{
			name:     "invalid JSON",
			jsonData: `{"type":"chat","from":"testuser"`,
			want:     nil,
			wantErr:  true,
		},
		{
			name:     "empty JSON",
			jsonData: `{}`,
			want: &Message{
				Type: "",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MessageFromJSON([]byte(tt.jsonData))
			if tt.wantErr {
				if err == nil {
					t.Errorf("MessageFromJSON() expected error but got none")
				}
				return
			}
			if err != nil {
				t.Errorf("MessageFromJSON() error = %v", err)
				return
			}

			if got.Type != tt.want.Type {
				t.Errorf("MessageFromJSON() Type = %v, want %v", got.Type, tt.want.Type)
			}
			if got.From != tt.want.From {
				t.Errorf("MessageFromJSON() From = %v, want %v", got.From, tt.want.From)
			}
			if got.Content != tt.want.Content {
				t.Errorf("MessageFromJSON() Content = %v, want %v", got.Content, tt.want.Content)
			}
			if got.Error != tt.want.Error {
				t.Errorf("MessageFromJSON() Error = %v, want %v", got.Error, tt.want.Error)
			}
			if len(got.Users) != len(tt.want.Users) {
				t.Errorf("MessageFromJSON() Users length = %v, want %v", len(got.Users), len(tt.want.Users))
			}
			for i, user := range got.Users {
				if i < len(tt.want.Users) && user != tt.want.Users[i] {
					t.Errorf("MessageFromJSON() Users[%d] = %v, want %v", i, user, tt.want.Users[i])
				}
			}
			if !got.Timestamp.Equal(tt.want.Timestamp) {
				t.Errorf("MessageFromJSON() Timestamp = %v, want %v", got.Timestamp, tt.want.Timestamp)
			}
		})
	}
}

func TestMessage_JSONRoundTrip(t *testing.T) {
	original := &Message{
		Type:    MessageTypeChat,
		From:    "testuser",
		Content: "Hello world",
	}
	original.SetTimestamp()

	// Convert to JSON
	jsonData, err := original.ToJSON()
	if err != nil {
		t.Errorf("ToJSON() error = %v", err)
		return
	}

	// Convert back from JSON
	restored, err := MessageFromJSON(jsonData)
	if err != nil {
		t.Errorf("MessageFromJSON() error = %v", err)
		return
	}

	// Compare fields
	if restored.Type != original.Type {
		t.Errorf("Round trip Type = %v, want %v", restored.Type, original.Type)
	}
	if restored.From != original.From {
		t.Errorf("Round trip From = %v, want %v", restored.From, original.From)
	}
	if restored.Content != original.Content {
		t.Errorf("Round trip Content = %v, want %v", restored.Content, original.Content)
	}
	if !restored.Timestamp.Equal(original.Timestamp) {
		t.Errorf("Round trip Timestamp = %v, want %v", restored.Timestamp, original.Timestamp)
	}
}

func TestMessageTypeConstants(t *testing.T) {
	// Verify all message type constants are defined correctly
	expectedTypes := map[string]string{
		"MessageTypeChat":     "chat",
		"MessageTypeSystem":   "system",
		"MessageTypeUserList": "user_list",
		"MessageTypeError":    "error",
		"MessageTypeJoin":     "join",
	}

	actualTypes := map[string]string{
		"MessageTypeChat":     MessageTypeChat,
		"MessageTypeSystem":   MessageTypeSystem,
		"MessageTypeUserList": MessageTypeUserList,
		"MessageTypeError":    MessageTypeError,
		"MessageTypeJoin":     MessageTypeJoin,
	}

	for name, expected := range expectedTypes {
		if actual, exists := actualTypes[name]; !exists || actual != expected {
			t.Errorf("Constant %s = %v, want %v", name, actual, expected)
		}
	}
}

// Test enhanced validation functions
func TestValidateDisplayName(t *testing.T) {
	tests := []struct {
		name        string
		displayName string
		wantErr     bool
		errContains string
	}{
		{
			name:        "valid display name",
			displayName: "JohnDoe",
			wantErr:     false,
		},
		{
			name:        "valid display name with spaces",
			displayName: "John Doe",
			wantErr:     false,
		},
		{
			name:        "valid display name with emojis",
			displayName: "John 😀",
			wantErr:     false,
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
		},
		{
			name:        "display name with HTML tags",
			displayName: "John<script>alert('xss')</script>",
			wantErr:     true,
			errContains: "cannot contain HTML tags",
		},
		{
			name:        "display name with simple HTML",
			displayName: "John<b>Bold</b>",
			wantErr:     true,
			errContains: "cannot contain HTML tags",
		},
		{
			name:        "display name with control characters",
			displayName: "John\x00Doe",
			wantErr:     true,
			errContains: "invalid control characters",
		},
		{
			name:        "display name with tab (allowed)",
			displayName: "John\tDoe",
			wantErr:     false,
		},
		{
			name:        "display name with newline (allowed)",
			displayName: "John\nDoe",
			wantErr:     false,
		},
		{
			name:        "display name with invalid UTF-8",
			displayName: "John\xff\xfeDoe",
			wantErr:     true,
			errContains: "invalid characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDisplayName(tt.displayName)
			if tt.wantErr {
				if err == nil {
					t.Errorf("validateDisplayName() expected error but got none")
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("validateDisplayName() error = %v, want error containing %v", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("validateDisplayName() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestValidateMessageContent(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid message content",
			content: "Hello world!",
			wantErr: false,
		},
		{
			name:    "valid message with emojis",
			content: "Hello 😀🎉",
			wantErr: false,
		},
		{
			name:    "valid message with special characters",
			content: "Hello @#$%^&*()_+-=[]{}|;':\",./<>?",
			wantErr: false,
		},
		{
			name:        "empty message content",
			content:     "",
			wantErr:     true,
			errContains: "cannot be empty",
		},
		{
			name:        "whitespace only message content",
			content:     "   ",
			wantErr:     true,
			errContains: "cannot be empty",
		},
		{
			name:        "message content too long",
			content:     strings.Repeat("a", 1001),
			wantErr:     true,
			errContains: "cannot exceed 1000 characters",
		},
		{
			name:    "message content at max length",
			content: strings.Repeat("a", 1000),
			wantErr: false,
		},
		{
			name:        "message content with invalid UTF-8",
			content:     "Hello\xff\xfeWorld",
			wantErr:     true,
			errContains: "invalid characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMessageContent(tt.content)
			if tt.wantErr {
				if err == nil {
					t.Errorf("validateMessageContent() expected error but got none")
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("validateMessageContent() error = %v, want error containing %v", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("validateMessageContent() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestMessage_SanitizeInput(t *testing.T) {
	tests := []struct {
		name     string
		input    Message
		expected Message
	}{
		{
			name: "sanitize chat message",
			input: Message{
				Type:    MessageTypeChat,
				From:    "John<script>alert('xss')</script>",
				Content: "Hello <b>world</b>!",
			},
			expected: Message{
				Type:    MessageTypeChat,
				From:    "John&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;",
				Content: "Hello &lt;b&gt;world&lt;/b&gt;!",
			},
		},
		{
			name: "sanitize error message",
			input: Message{
				Type:  MessageTypeError,
				Error: "Error: <script>alert('xss')</script>",
			},
			expected: Message{
				Type:  MessageTypeError,
				Error: "Error: &lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;",
			},
		},
		{
			name: "sanitize user list",
			input: Message{
				Type:  MessageTypeUserList,
				Users: []string{"John<script>", "Jane</script>", "Bob"},
			},
			expected: Message{
				Type:  MessageTypeUserList,
				Users: []string{"John&lt;script&gt;", "Jane&lt;/script&gt;", "Bob"},
			},
		},
		{
			name: "sanitize with whitespace trimming",
			input: Message{
				Type:    MessageTypeChat,
				From:    "  John  ",
				Content: "  Hello world  ",
			},
			expected: Message{
				Type:    MessageTypeChat,
				From:    "John",
				Content: "Hello world",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy to avoid modifying the input
			message := tt.input
			message.SanitizeInput()

			if message.From != tt.expected.From {
				t.Errorf("SanitizeInput() From = %v, want %v", message.From, tt.expected.From)
			}
			if message.Content != tt.expected.Content {
				t.Errorf("SanitizeInput() Content = %v, want %v", message.Content, tt.expected.Content)
			}
			if message.Error != tt.expected.Error {
				t.Errorf("SanitizeInput() Error = %v, want %v", message.Error, tt.expected.Error)
			}
			if len(message.Users) != len(tt.expected.Users) {
				t.Errorf("SanitizeInput() Users length = %v, want %v", len(message.Users), len(tt.expected.Users))
			}
			for i, user := range message.Users {
				if i < len(tt.expected.Users) && user != tt.expected.Users[i] {
					t.Errorf("SanitizeInput() Users[%d] = %v, want %v", i, user, tt.expected.Users[i])
				}
			}
		})
	}
}

func TestMessage_ValidateEnhanced(t *testing.T) {
	tests := []struct {
		name    string
		message Message
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid chat message with enhanced validation",
			message: Message{
				Type:    MessageTypeChat,
				From:    "testuser",
				Content: "Hello world",
			},
			wantErr: false,
		},
		{
			name: "chat message with HTML in sender name",
			message: Message{
				Type:    MessageTypeChat,
				From:    "test<script>alert('xss')</script>",
				Content: "Hello world",
			},
			wantErr: true,
			errMsg:  "sender invalid",
		},
		{
			name: "chat message with too long content",
			message: Message{
				Type:    MessageTypeChat,
				From:    "testuser",
				Content: strings.Repeat("a", 1001),
			},
			wantErr: true,
			errMsg:  "cannot exceed 1000 characters",
		},
		{
			name: "join message with HTML in display name",
			message: Message{
				Type:    MessageTypeJoin,
				Content: "test<b>user</b>",
			},
			wantErr: true,
			errMsg:  "display name invalid",
		},
		{
			name: "join message with control characters",
			message: Message{
				Type:    MessageTypeJoin,
				Content: "test\x00user",
			},
			wantErr: true,
			errMsg:  "display name invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.message.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("Validate() expected error but got none")
					return
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %v, want error containing %v", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Validate() unexpected error = %v", err)
				}
			}
		})
	}
}
