package main

import (
	"fmt"
	"log"
	"os"
	"time"
)

// Logger provides structured logging for the application
type Logger struct {
	*log.Logger
	level LogLevel
}

// LogLevel represents different log levels
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
)

var (
	// Global logger instance
	appLogger *Logger
)

// InitLogger initializes the global logger
func InitLogger(level LogLevel) {
	appLogger = &Logger{
		Logger: log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds),
		level:  level,
	}
}

// Debug logs debug messages
func (l *Logger) Debug(format string, args ...interface{}) {
	if l.level <= DEBUG {
		l.logWithLevel("DEBUG", format, args...)
	}
}

// Info logs info messages
func (l *Logger) Info(format string, args ...interface{}) {
	if l.level <= INFO {
		l.logWithLevel("INFO", format, args...)
	}
}

// Warn logs warning messages
func (l *Logger) Warn(format string, args ...interface{}) {
	if l.level <= WARN {
		l.logWithLevel("WARN", format, args...)
	}
}

// Error logs error messages
func (l *Logger) Error(format string, args ...interface{}) {
	if l.level <= ERROR {
		l.logWithLevel("ERROR", format, args...)
	}
}

// logWithLevel logs a message with the specified level
func (l *Logger) logWithLevel(level, format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	l.Printf("[%s] %s", level, message)
}

// LogConnectionStats logs connection statistics
func LogConnectionStats(hub *Hub) {
	if appLogger == nil {
		return
	}
	
	stats := hub.GetConnectionStats()
	appLogger.Info("Connection Stats: Total=%d, Active=%d, Idle=%d, Users=%d, Max=%d",
		stats["total_connections"],
		stats["active_connections"], 
		stats["idle_connections"],
		stats["users_online"],
		stats["max_connections"])
}

// LogRateLimit logs rate limiting events
func LogRateLimit(clientName string, remaining int) {
	if appLogger == nil {
		return
	}
	
	appLogger.Warn("Rate limit triggered for client %s, remaining: %d", clientName, remaining)
}

// LogClientActivity logs client connection/disconnection events
func LogClientActivity(action, clientName, remoteAddr string) {
	if appLogger == nil {
		return
	}
	
	appLogger.Info("Client %s: %s from %s", action, clientName, remoteAddr)
}

// LogError logs error events
func LogError(context, message string, err error) {
	if appLogger == nil {
		return
	}
	
	if err != nil {
		appLogger.Error("%s: %s - %v", context, message, err)
	} else {
		appLogger.Error("%s: %s", context, message)
	}
}

// StartPeriodicLogging starts periodic logging of system stats
func StartPeriodicLogging(hub *Hub) {
	if appLogger == nil {
		return
	}
	
	ticker := time.NewTicker(1 * time.Minute)
	go func() {
		for range ticker.C {
			LogConnectionStats(hub)
		}
	}()
}