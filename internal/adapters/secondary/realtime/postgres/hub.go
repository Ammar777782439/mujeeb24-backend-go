package postgres

import (
	"strings"
	"sync"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
)

const (
	DefaultConnectionBufferSize = 128
)

// ClientConnection represents an active, authenticated SSE stream connection.
type ClientConnection struct {
	ID             string
	BusinessID     string
	Topics         map[string]struct{}
	ConversationID string
	Events         chan ports.RealtimeEvent
	Done           chan struct{}
	closeOnce      sync.Once
}

func NewClientConnection(businessID string, topics []string, conversationID string, bufferSize int) *ClientConnection {
	if bufferSize <= 0 {
		bufferSize = DefaultConnectionBufferSize
	}
	topicMap := make(map[string]struct{})
	for _, t := range topics {
		norm := strings.ToLower(strings.TrimSpace(t))
		if norm != "" {
			topicMap[norm] = struct{}{}
		}
	}
	return &ClientConnection{
		ID:             uuid.NewString(),
		BusinessID:     strings.TrimSpace(businessID),
		Topics:         topicMap,
		ConversationID: strings.TrimSpace(conversationID),
		Events:         make(chan ports.RealtimeEvent, bufferSize),
		Done:           make(chan struct{}),
	}
}

func (c *ClientConnection) Close() {
	c.closeOnce.Do(func() {
		close(c.Done)
	})
}

// LocalHub manages local in-memory SSE connections for a single backend instance,
// strictly isolated by BusinessID.
type LocalHub struct {
	mu          sync.RWMutex
	connections map[string]map[string]*ClientConnection // businessID -> connectionID -> ClientConnection
	closed      bool
}

func NewLocalHub() *LocalHub {
	return &LocalHub{
		connections: make(map[string]map[string]*ClientConnection),
	}
}

func (h *LocalHub) Register(conn *ClientConnection) {
	if conn == nil || conn.BusinessID == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		conn.Close()
		return
	}
	bizConns, exists := h.connections[conn.BusinessID]
	if !exists {
		bizConns = make(map[string]*ClientConnection)
		h.connections[conn.BusinessID] = bizConns
	}
	bizConns[conn.ID] = conn
}

func (h *LocalHub) Unregister(conn *ClientConnection) {
	if conn == nil || conn.BusinessID == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if bizConns, exists := h.connections[conn.BusinessID]; exists {
		delete(bizConns, conn.ID)
		if len(bizConns) == 0 {
			delete(h.connections, conn.BusinessID)
		}
	}
	conn.Close()
}

func (h *LocalHub) ActiveConnectionCount(businessID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if businessID == "" {
		total := 0
		for _, m := range h.connections {
			total += len(m)
		}
		return total
	}
	return len(h.connections[businessID])
}

// Broadcast delivers an event to all matching local connections belonging to event.BusinessID.
func (h *LocalHub) Broadcast(event ports.RealtimeEvent) {
	if event.BusinessID == "" {
		return
	}
	h.mu.RLock()
	bizConns, exists := h.connections[event.BusinessID]
	if !exists || len(bizConns) == 0 {
		h.mu.RUnlock()
		return
	}
	// Copy connection pointers under read lock
	targets := make([]*ClientConnection, 0, len(bizConns))
	for _, conn := range bizConns {
		targets = append(targets, conn)
	}
	h.mu.RUnlock()

	domainTopic := eventDomainTopic(event.EventType)
	for _, conn := range targets {
		if !connMatches(conn, domainTopic, event) {
			continue
		}
		select {
		case <-conn.Done:
			// Connection already closed
		case conn.Events <- event:
			// Delivered into bounded buffer successfully
		default:
			// Slow consumer: bounded buffer is full.
			// Disconnect slow consumer without blocking hub or other clients.
			go func(slowConn *ClientConnection) {
				h.Unregister(slowConn)
			}(conn)
		}
	}
}

func (h *LocalHub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.closed = true
	for _, bizConns := range h.connections {
		for _, conn := range bizConns {
			conn.Close()
		}
	}
	h.connections = make(map[string]map[string]*ClientConnection)
}

func connMatches(conn *ClientConnection, domainTopic string, event ports.RealtimeEvent) bool {
	// Topic filtering: if Topics map is empty, client is subscribed to all authorized topics.
	if len(conn.Topics) > 0 {
		if _, matches := conn.Topics[domainTopic]; !matches {
			// Also check singular / plural aliases if needed
			if _, matchesPlural := conn.Topics[domainTopic+"s"]; !matchesPlural {
				return false
			}
		}
	}
	// Optional resource filtering: if conversation_id is specified, only match events for that conversation
	if conn.ConversationID != "" {
		if event.ResourceType == "conversation" && event.ResourceID != conn.ConversationID {
			return false
		}
	}
	return true
}

func eventDomainTopic(eventType string) string {
	parts := strings.Split(eventType, ".")
	if len(parts) > 0 {
		prefix := strings.ToLower(strings.TrimSpace(parts[0]))
		switch prefix {
		case "conversation", "conversations":
			return "conversations"
		case "channel", "channels":
			return "channels"
		case "lead", "leads":
			return "leads"
		case "transaction", "transactions":
			return "transactions"
		case "ai":
			return "ai"
		case "catalog":
			return "catalog"
		case "notification", "notifications":
			return "notifications"
		default:
			return prefix
		}
	}
	return "general"
}
