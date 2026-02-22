package server

import (
	"context"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/garnizeh/eth-scanner/internal/database"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// For production, check origin properly.
	CheckOrigin: func(_ *http.Request) bool {
		return true
	},
}

// Hub maintains the set of active clients and broadcasts messages to the
// clients.
type Hub struct {
	// Registered clients.
	clients map[*Client]bool

	// Inbound messages from the clients as raw HTML.
	broadcast chan []byte

	// Register requests from the clients.
	register chan *Client

	// Unregister requests from clients.
	unregister chan *Client

	mu sync.Mutex
}

func newHub() *Hub {
	return &Hub{
		broadcast:  make(chan []byte, 10),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
	}
}

func (h *Hub) run(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
		case message := <-h.broadcast:
			h.mu.Lock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mu.Unlock()
		case <-ticker.C:
			// Heartbeat will be handled here if we want to send periodic stats
		}
	}
}

// Client is a middleman between the websocket connection and the hub.
type Client struct {
	hub *Hub

	// The websocket connection.
	conn *websocket.Conn

	// Buffered channel of outbound messages.
	send chan []byte
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(512)
	_ = c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error { _ = c.conn.SetReadDeadline(time.Now().Add(60 * time.Second)); return nil })
	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}
		// We don't expect messages from the client for now, just keep the connection alive.
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(50 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				// The hub closed the channel.
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			if _, err := w.Write(message); err != nil {
				return
			}

			// Add queued chat messages to the current websocket message.
			n := len(c.send)
			for range n {
				if _, err := w.Write([]byte("\n")); err != nil {
					return
				}
				if _, err := w.Write(<-c.send); err != nil {
					return
				}
			}

			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleWS handles websocket requests from the peer.
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("failed to upgrade to websocket: %v", err)
		return
	}
	client := &Client{hub: s.hub, conn: conn, send: make(chan []byte, 256)}
	client.hub.register <- client

	// Allow collection of memory referenced by the caller by doing all work in
	// new goroutines.
	go client.writePump()
	go client.readPump()
}

// Broadcast sends a message to all connected clients.
func (s *Server) Broadcast(message []byte) {
	s.hub.broadcast <- message
}

// broadcastStats is called periodically or when an update happens to broadcast
// refreshed stats to all connected dashboard clients.
func (s *Server) broadcastStats(ctx context.Context) {
	q := database.New(s.db)
	stats, err := q.GetStats(ctx)
	if err != nil {
		log.Printf("failed to get stats for broadcast: %v", err)
		return
	}

	activeWorkers, _ := q.GetActiveWorkerDetails(ctx)
	prefixProgress, _ := q.GetPrefixProgress(ctx)
	results, _ := q.GetDetailedResults(ctx, 10)

	// Normalize total keys scanned to int64
	var totalKeys int64
	switch v := stats.TotalKeysScanned.(type) {
	case int64:
		totalKeys = v
	case int:
		totalKeys = int64(v)
	case float64:
		totalKeys = int64(v)
	default:
		totalKeys = 0
	}

	// Normalize global throughput to float64
	var globalThroughput float64
	switch v := stats.GlobalKeysPerSecond.(type) {
	case float64:
		globalThroughput = v
	case int64:
		globalThroughput = float64(v)
	case int:
		globalThroughput = float64(v)
	default:
		globalThroughput = 0
	}

	data := struct {
		ActiveWorkerCount   int64
		TotalKeysScanned    int64
		CompletedJobCount   int64
		ProcessingJobCount  int64
		PendingJobCount     int64
		TotalWorkers        int64
		GlobalKeysPerSecond float64
		ActiveWorkers       []database.GetActiveWorkerDetailsRow
		PrefixProgress      []database.GetPrefixProgressRow
		Results             []database.GetDetailedResultsRow
		NowTimestamp        int64
	}{
		ActiveWorkerCount:   stats.ActiveWorkers,
		TotalKeysScanned:    totalKeys,
		CompletedJobCount:   stats.CompletedBatches,
		ProcessingJobCount:  stats.ProcessingBatches,
		PendingJobCount:     stats.PendingBatches,
		TotalWorkers:        stats.TotalWorkers,
		GlobalKeysPerSecond: globalThroughput,
		ActiveWorkers:       activeWorkers,
		PrefixProgress:      prefixProgress,
		Results:             results,
		NowTimestamp:        time.Now().Unix(),
	}

	var buf strings.Builder
	if err := s.renderer.RenderFragment(&buf, "fragments.html", "fleet-stats", data); err != nil {
		log.Printf("failed to render stats fragment: %v", err)
		// continue anyway to try other fragments
	}

	// Also render the active workers table for the dashboard
	if err := s.renderer.RenderFragment(&buf, "active_workers.html", "active-workers", map[string]any{
		"ActiveWorkers": activeWorkers,
	}); err != nil {
		log.Printf("failed to render active workers fragment: %v", err)
	}

	// Render prefix progress overview
	if err := s.renderer.RenderFragment(&buf, "fragments.html", "prefix-progress", data); err != nil {
		log.Printf("failed to render prefix progress fragment: %v", err)
	}

	s.Broadcast([]byte(buf.String()))
}
