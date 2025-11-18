// Frontend Unit Tests for ConversationManager
// These tests can be run with a JavaScript testing framework like Jest or Mocha

// Mock ConversationManager class (extracted from index.html for testing)
class ConversationManager {
  constructor() {
    this.conversations = new Map();
    this.conversations.set(null, []);
    this.activeConversation = null;
    this.unreadCounts = new Map();
    this.scrollPositions = new Map();
    this.maxMessagesPerConversation = 100;
  }

  switchConversation(username) {
    if (this.activeConversation !== null || username !== this.activeConversation) {
      this.saveScrollPosition(this.activeConversation);
    }
    this.activeConversation = username;
    this.markAsRead(username);
  }

  addMessage(message) {
    let conversationKey = null;
    const currentUser = global.displayName || "TestUser";

    if (message.type === "private") {
      conversationKey = message.from === currentUser ? message.to : message.from;
    } else {
      conversationKey = null;
    }

    if (!this.conversations.has(conversationKey)) {
      this.conversations.set(conversationKey, []);
    }

    const messages = this.conversations.get(conversationKey);
    messages.push(message);

    if (messages.length > this.maxMessagesPerConversation) {
      messages.splice(0, messages.length - this.maxMessagesPerConversation);
    }

    if (conversationKey !== this.activeConversation) {
      this.incrementUnreadCount(conversationKey);
    }
  }

  getMessages(username) {
    return this.conversations.get(username) || [];
  }

  markAsRead(username) {
    this.unreadCounts.set(username, 0);
  }

  getUnreadCount(username) {
    return this.unreadCounts.get(username) || 0;
  }

  incrementUnreadCount(username) {
    const currentCount = this.getUnreadCount(username);
    this.unreadCounts.set(username, currentCount + 1);
  }

  saveScrollPosition(username) {
    // Mock implementation for testing
    this.scrollPositions.set(username, 100);
  }

  getScrollPosition(username) {
    return this.scrollPositions.get(username) || null;
  }

  clearConversation(username) {
    this.conversations.delete(username);
    this.unreadCounts.delete(username);
    this.scrollPositions.delete(username);
  }
}

// Test Suite
describe('ConversationManager', () => {
  let manager;

  beforeEach(() => {
    manager = new ConversationManager();
    global.displayName = "TestUser";
  });

  describe('switchConversation', () => {
    test('should switch to public conversation', () => {
      manager.switchConversation(null);
      expect(manager.activeConversation).toBe(null);
    });

    test('should switch to private conversation', () => {
      manager.switchConversation("Alice");
      expect(manager.activeConversation).toBe("Alice");
    });

    test('should mark conversation as read when switching', () => {
      manager.unreadCounts.set("Bob", 5);
      manager.switchConversation("Bob");
      expect(manager.getUnreadCount("Bob")).toBe(0);
    });

    test('should save scroll position when switching', () => {
      manager.switchConversation("Alice");
      manager.switchConversation("Bob");
      expect(manager.scrollPositions.has("Alice")).toBe(true);
    });
  });

  describe('addMessage', () => {
    test('should add public message to public conversation', () => {
      const message = {
        type: "chat",
        from: "Alice",
        content: "Hello everyone!",
        timestamp: new Date().toISOString()
      };
      manager.addMessage(message);
      const messages = manager.getMessages(null);
      expect(messages.length).toBe(1);
      expect(messages[0].content).toBe("Hello everyone!");
    });

    test('should add private message to correct conversation (received)', () => {
      const message = {
        type: "private",
        from: "Alice",
        to: "TestUser",
        content: "Hello TestUser!",
        timestamp: new Date().toISOString()
      };
      manager.addMessage(message);
      const messages = manager.getMessages("Alice");
      expect(messages.length).toBe(1);
      expect(messages[0].content).toBe("Hello TestUser!");
    });

    test('should add private message to correct conversation (sent)', () => {
      const message = {
        type: "private",
        from: "TestUser",
        to: "Bob",
        content: "Hello Bob!",
        timestamp: new Date().toISOString()
      };
      manager.addMessage(message);
      const messages = manager.getMessages("Bob");
      expect(messages.length).toBe(1);
      expect(messages[0].content).toBe("Hello Bob!");
    });

    test('should increment unread count for inactive conversation', () => {
      manager.switchConversation(null); // Active on public
      const message = {
        type: "private",
        from: "Alice",
        to: "TestUser",
        content: "Hello!",
        timestamp: new Date().toISOString()
      };
      manager.addMessage(message);
      expect(manager.getUnreadCount("Alice")).toBe(1);
    });

    test('should not increment unread count for active conversation', () => {
      manager.switchConversation("Alice"); // Active on Alice
      const message = {
        type: "private",
        from: "Alice",
        to: "TestUser",
        content: "Hello!",
        timestamp: new Date().toISOString()
      };
      manager.addMessage(message);
      expect(manager.getUnreadCount("Alice")).toBe(0);
    });

    test('should limit conversation history to maxMessagesPerConversation', () => {
      manager.maxMessagesPerConversation = 5;
      for (let i = 0; i < 10; i++) {
        const message = {
          type: "chat",
          from: "Alice",
          content: `Message ${i}`,
          timestamp: new Date().toISOString()
        };
        manager.addMessage(message);
      }
      const messages = manager.getMessages(null);
      expect(messages.length).toBe(5);
      expect(messages[0].content).toBe("Message 5");
      expect(messages[4].content).toBe("Message 9");
    });
  });

  describe('markAsRead', () => {
    test('should reset unread count to zero', () => {
      manager.unreadCounts.set("Alice", 5);
      manager.markAsRead("Alice");
      expect(manager.getUnreadCount("Alice")).toBe(0);
    });

    test('should work for conversation with no unread messages', () => {
      manager.markAsRead("Bob");
      expect(manager.getUnreadCount("Bob")).toBe(0);
    });
  });

  describe('getUnreadCount', () => {
    test('should return unread count for user', () => {
      manager.unreadCounts.set("Alice", 3);
      expect(manager.getUnreadCount("Alice")).toBe(3);
    });

    test('should return 0 for user with no unread messages', () => {
      expect(manager.getUnreadCount("Bob")).toBe(0);
    });
  });

  describe('clearConversation', () => {
    test('should remove conversation messages', () => {
      const message = {
        type: "private",
        from: "Alice",
        to: "TestUser",
        content: "Hello!",
        timestamp: new Date().toISOString()
      };
      manager.addMessage(message);
      manager.clearConversation("Alice");
      expect(manager.getMessages("Alice").length).toBe(0);
    });

    test('should remove unread count', () => {
      manager.unreadCounts.set("Alice", 5);
      manager.clearConversation("Alice");
      expect(manager.getUnreadCount("Alice")).toBe(0);
    });

    test('should remove scroll position', () => {
      manager.scrollPositions.set("Alice", 100);
      manager.clearConversation("Alice");
      expect(manager.getScrollPosition("Alice")).toBe(null);
    });
  });

  describe('getMessages', () => {
    test('should return messages for existing conversation', () => {
      const message = {
        type: "chat",
        from: "Alice",
        content: "Hello!",
        timestamp: new Date().toISOString()
      };
      manager.addMessage(message);
      const messages = manager.getMessages(null);
      expect(messages.length).toBe(1);
    });

    test('should return empty array for non-existent conversation', () => {
      const messages = manager.getMessages("NonExistent");
      expect(messages.length).toBe(0);
    });
  });
});

// Export for testing frameworks
if (typeof module !== 'undefined' && module.exports) {
  module.exports = { ConversationManager };
}
