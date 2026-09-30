package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	realtimePostgres "github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/realtime/postgres"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
)

const (
	DefaultHeartbeatInterval = 15 * time.Second
)

type RealtimeSSEHandler struct {
	Hub               *realtimePostgres.LocalHub
	Scope             ScopeProvider
	HeartbeatInterval time.Duration
	BufferSize        int
}

func NewRealtimeSSEHandler(hub *realtimePostgres.LocalHub, scope ScopeProvider) *RealtimeSSEHandler {
	return &RealtimeSSEHandler{
		Hub:               hub,
		Scope:             scope,
		HeartbeatInterval: DefaultHeartbeatInterval,
		BufferSize:        realtimePostgres.DefaultConnectionBufferSize,
	}
}

func (h *RealtimeSSEHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	businessID := strings.TrimSpace(r.PathValue("business_id"))
	if businessID == "" {
		businessID = extractBusinessIDFromPath(r.URL.Path)
	}
	if businessID == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "business_id path parameter is required")
		return
	}

	if h.Scope == nil || h.Hub == nil {
		writeError(w, http.StatusNotImplemented, "not_implemented", "realtime is not configured")
		return
	}

	// Verify tenant authorization using PostgresScopeProvider
	_, err := h.Scope.Resolve(r.Context(), commands.BusinessID(businessID))
	if err != nil {
		status := http.StatusForbidden
		code := "forbidden"
		var typed *appErrors.Error
		if errors.As(err, &typed) && typed.Code == appErrors.CodeUnauthenticated {
			status = http.StatusUnauthorized
			code = "unauthorized"
		}
		writeError(w, status, code, err.Error())
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming_unsupported", "server-sent events are not supported by the client connection")
		return
	}

	// Parse topic and conversation filters from query parameters
	var topics []string
	if topicsParam := strings.TrimSpace(r.URL.Query().Get("topics")); topicsParam != "" {
		for _, t := range strings.Split(topicsParam, ",") {
			if trimmed := strings.TrimSpace(t); trimmed != "" {
				topics = append(topics, trimmed)
			}
		}
	}
	conversationID := strings.TrimSpace(r.URL.Query().Get("conversation_id"))

	// Set required SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Flush initial headers
	flusher.Flush()

	conn := realtimePostgres.NewClientConnection(businessID, topics, conversationID, h.BufferSize)
	h.Hub.Register(conn)
	defer h.Hub.Unregister(conn)

	interval := h.HeartbeatInterval
	if interval <= 0 {
		interval = DefaultHeartbeatInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-conn.Done:
			return
		case <-ticker.C:
			if _, writeErr := fmt.Fprintf(w, ": ping\n\n"); writeErr != nil {
				return
			}
			flusher.Flush()
		case event, ok := <-conn.Events:
			if !ok {
				return
			}
			payload, err := json.Marshal(event)
			if err != nil {
				continue
			}
			if _, writeErr := fmt.Fprintf(w, "id: %s\nevent: %s\ndata: %s\n\n", event.EventID, event.EventType, string(payload)); writeErr != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func extractBusinessIDFromPath(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i, p := range parts {
		if p == "businesses" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
}
