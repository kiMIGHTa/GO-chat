package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

func main() {
	// Initialize logger
	InitLogger(INFO)
	
	// Create and start the hub
	hub := NewHub()
	go hub.Run()
	
	// Start periodic logging
	StartPeriodicLogging(hub)

	// Set up HTTP routes
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		handleWebSocket(hub, w, r)
	})

	// Serve static files from the static directory
	fs := http.FileServer(http.Dir("./static/"))
	http.Handle("/", fs)

	// Set up graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	// Get port from environment or default to 8080
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Start server in a goroutine
	server := &http.Server{
		Addr: ":" + port,
	}

	go func() {
		log.Printf("Starting server on :%s", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Wait for interrupt signal
	<-c
	log.Println("Shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Stop the hub first
	hub.Stop()
	log.Println("Hub stopped")

	// Shutdown HTTP server gracefully
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	} else {
		log.Println("Server gracefully stopped")
	}
}

// handleWebSocket handles WebSocket upgrade requests and manages client connections
func handleWebSocket(hub *Hub, w http.ResponseWriter, r *http.Request) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("WebSocket handler panic recovered: %v", r)
		}
	}()

	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed from %s: %v", r.RemoteAddr, err)
		// Don't call http.Error after upgrader.Upgrade fails, as it may have already written headers
		return
	}

	// Check connection limits
	if !hub.CanAcceptNewConnection() {
		log.Printf("Connection limit reached, rejecting connection from %s", r.RemoteAddr)
		conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseTryAgainLater, "Server at capacity"))
		conn.Close()
		return
	}

	// Create new client
	client := NewClient(hub, conn)

	// Start client goroutines with panic recovery
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("WritePump goroutine panic recovered: %v", r)
			}
		}()
		client.WritePump()
	}()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("ReadPump goroutine panic recovered: %v", r)
			}
		}()
		client.ReadPump()
	}()

	LogClientActivity("connected", "unknown", r.RemoteAddr)
}