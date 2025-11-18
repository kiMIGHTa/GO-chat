# Frontend Tests

This directory contains frontend unit tests for the private messaging functionality.

## Test Files

- `conversation-manager.test.js` - Unit tests for the ConversationManager class

## Running Tests

The frontend tests are written in a Jest-compatible format but can be run with any JavaScript testing framework.

### Option 1: Using Jest (Recommended)

1. Install Jest:
```bash
npm install --save-dev jest
```

2. Run tests:
```bash
npx jest static/conversation-manager.test.js
```

### Option 2: Using Node.js with a simple test runner

The tests use a describe/test/expect syntax that can be implemented with a simple test runner.

### Option 3: Manual Testing

The ConversationManager functionality can be tested manually by:

1. Opening the chat application in a browser
2. Opening the browser console
3. Testing the following scenarios:

#### Test: Switch Conversation
```javascript
// Should switch to public conversation
conversationManager.switchConversation(null);
console.assert(conversationManager.activeConversation === null, "Failed to switch to public");

// Should switch to private conversation
conversationManager.switchConversation("Alice");
console.assert(conversationManager.activeConversation === "Alice", "Failed to switch to Alice");
```

#### Test: Add Message
```javascript
// Should add public message
const publicMsg = {type: "chat", from: "Alice", content: "Hello!", timestamp: new Date().toISOString()};
conversationManager.addMessage(publicMsg);
console.assert(conversationManager.getMessages(null).length > 0, "Failed to add public message");

// Should add private message
const privateMsg = {type: "private", from: "Alice", to: displayName, content: "Hi!", timestamp: new Date().toISOString()};
conversationManager.addMessage(privateMsg);
console.assert(conversationManager.getMessages("Alice").length > 0, "Failed to add private message");
```

#### Test: Unread Count
```javascript
// Should track unread messages
conversationManager.switchConversation(null); // Switch away from Alice
const msg = {type: "private", from: "Alice", to: displayName, content: "Test", timestamp: new Date().toISOString()};
conversationManager.addMessage(msg);
console.assert(conversationManager.getUnreadCount("Alice") > 0, "Failed to track unread");

// Should clear unread on switch
conversationManager.switchConversation("Alice");
console.assert(conversationManager.getUnreadCount("Alice") === 0, "Failed to clear unread");
```

#### Test: Clear Conversation
```javascript
// Should clear conversation data
conversationManager.clearConversation("Alice");
console.assert(conversationManager.getMessages("Alice").length === 0, "Failed to clear messages");
console.assert(conversationManager.getUnreadCount("Alice") === 0, "Failed to clear unread count");
```

## Test Coverage

The tests cover the following ConversationManager methods:

- ✅ `switchConversation()` - Switching between public and private conversations
- ✅ `addMessage()` - Adding messages to correct conversations
- ✅ `getMessages()` - Retrieving conversation messages
- ✅ `markAsRead()` - Marking conversations as read
- ✅ `getUnreadCount()` - Getting unread message counts
- ✅ `clearConversation()` - Clearing conversation data

## UI Interaction Tests

The following UI interactions should be tested manually:

1. **User List Click Handling**
   - Click on a user in the user list
   - Verify conversation switches to that user
   - Verify active highlighting updates

2. **Active Conversation Highlighting**
   - Switch between conversations
   - Verify only active conversation is highlighted
   - Verify public chatroom button highlights correctly

3. **Unread Badge Display**
   - Receive message in inactive conversation
   - Verify unread badge appears with correct count
   - Switch to conversation
   - Verify unread badge disappears

4. **Public Chatroom Button**
   - Click public chatroom button
   - Verify switches to public conversation
   - Verify message input placeholder updates
   - Verify header updates

## Integration Test Scenarios

These scenarios should be tested with multiple browser windows:

1. **End-to-End Private Message Delivery**
   - User A sends private message to User B
   - Verify User B receives message
   - Verify User A sees echo
   - Verify User C does not see message

2. **Conversation Switching with History**
   - Send messages in multiple conversations
   - Switch between conversations
   - Verify message history loads correctly

3. **User Disconnect Handling**
   - User A in conversation with User B
   - User B disconnects
   - Verify User A receives notification
   - Verify conversation handling

4. **Unread Message Tracking**
   - User A sends message to User B
   - User B in different conversation
   - Verify unread badge appears
   - User B switches to conversation
   - Verify unread badge clears
