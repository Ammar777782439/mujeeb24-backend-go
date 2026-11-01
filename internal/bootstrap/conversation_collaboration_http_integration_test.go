package bootstrap

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/contract"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/persistence/postgres"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
)

// Invoked from the authenticated PostgreSQL governance fixture. Two logged-in
// sessions communicate through the real HTTP mux, LISTEN/NOTIFY and SSE socket.
func exerciseInboxCollaborationHTTP(t *testing.T, ctx context.Context, runtime *APIRuntime, businessID, staffID, ownerToken, staffToken string) {
	t.Helper()
	pool := runtime.Database.Pool()
	customerID, connectionID, conversationID, referenceID, inboundID := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	var extraPrincipals []string
	defer func() {
		for _, query := range []string{
			`DELETE FROM outbox_entries WHERE business_id=$1::uuid`,
			`DELETE FROM conversation_read_cursors WHERE business_id=$1::uuid`,
			`DELETE FROM conversation_state WHERE business_id=$1::uuid`,
			`DELETE FROM communication_messages WHERE business_id=$1::uuid`,
			`DELETE FROM outbound_messages WHERE business_id=$1::uuid`,
			`DELETE FROM conversation_references WHERE business_id=$1::uuid`,
			`DELETE FROM conversations WHERE business_id=$1::uuid`,
			`DELETE FROM channel_connections WHERE business_id=$1::uuid`,
			`DELETE FROM customers WHERE business_id=$1::uuid`,
		} {
			if _, err := pool.Exec(context.Background(), query, businessID); err != nil {
				t.Error(err)
			}
		}
		for _, id := range extraPrincipals {
			for _, query := range []string{`DELETE FROM refresh_sessions WHERE principal_id=$1::uuid`, `DELETE FROM business_memberships WHERE principal_id=$1::uuid`, `DELETE FROM principals WHERE id=$1::uuid`} {
				if _, err := pool.Exec(context.Background(), query, id); err != nil {
					t.Error(err)
				}
			}
		}
	}()
	queries := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO customers(id,business_id,profile,contact_points,status,created_at,updated_at) VALUES($1::uuid,$2::uuid,'{}','[]','active',now(),now())`, []any{customerID, businessID}},
		{`INSERT INTO channel_connections(id,business_id,provider_ref,channel,provider_connection_ref,status,secret_reference,created_at,updated_at) VALUES($1::uuid,$2::uuid,'socialapi','whatsapp',$3,'active','isolated-fixture',now(),now())`, []any{connectionID, businessID, "collaboration-" + connectionID}},
		{`INSERT INTO conversations(id,business_id,customer_id,state,ownership,priority,last_activity_at,created_at,updated_at) VALUES($1::uuid,$2::uuid,$3::uuid,'ai_handling','ai','normal',now(),now(),now())`, []any{conversationID, businessID, customerID}},
		{`INSERT INTO conversation_references(id,business_id,conversation_id,system,provider_ref,resource_type,resource_id,connection_id,conversation_kind,is_current,mapping_status,created_at,updated_at) VALUES($1::uuid,$2::uuid,$3::uuid,'provider','socialapi','conversation',$4,$5::uuid,'dm',true,'active',now(),now())`, []any{referenceID, businessID, conversationID, "remote-" + conversationID, connectionID}},
	}
	for _, q := range queries {
		if _, err := pool.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	text := "أحتاج معرفة السعر قبل تأكيد الطلب"
	if _, err := postgres.NewMessageRepository(runtime.Database).Record(ctx, ports.CommunicationMessageDraft{ID: inboundID, BusinessID: businessID, ConversationReferenceID: referenceID, Direction: "inbound", Origin: "customer", Transport: "provider", ContentType: "text", TextContent: &text, ContentReference: "inbound:" + inboundID, Visibility: "public", OccurredAt: time.Now().UTC(), CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	base := "/api/v1/businesses/" + businessID
	conversationPath := base + "/conversations/" + conversationID
	governanceHTTPRequest(t, runtime.HTTP.Handler, http.MethodGet, base+"/team/members", nil, ownerToken, "", http.StatusForbidden)
	directory := governanceHTTPRequest(t, runtime.HTTP.Handler, http.MethodGet, base+"/conversations/assignees", nil, ownerToken, "", http.StatusOK)
	if !strings.Contains(directory.Body.String(), staffID) || strings.Contains(directory.Body.String(), `"email"`) || strings.Contains(directory.Body.String(), `"permissions"`) {
		t.Fatal("assignment directory did not preserve minimum scope")
	}
	governanceHTTPRequest(t, runtime.HTTP.Handler, http.MethodGet, base+"/conversations/assignees", nil, staffToken, "", http.StatusForbidden)
	if err := runtime.RealtimeBroker.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(runtime.HTTP.Handler)
	defer server.Close()
	streamCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	request, _ := http.NewRequestWithContext(streamCtx, http.MethodGet, server.URL+base+"/realtime/events", nil)
	request.Header.Set("Authorization", "Bearer "+staffToken)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.Contains(response.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("SSE response status %d", response.StatusCode)
	}
	events := make(chan ports.RealtimeEvent, 10)
	go func() {
		defer close(events)
		scanner := bufio.NewScanner(response.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data: ") {
				var event ports.RealtimeEvent
				if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event) == nil {
					select {
					case events <- event:
					case <-streamCtx.Done():
						return
					}
				}
			}
		}
	}()
	await := func(kind string) {
		t.Helper()
		timer := time.NewTimer(3 * time.Second)
		defer timer.Stop()
		for {
			select {
			case event, ok := <-events:
				if !ok {
					t.Fatal("SSE disconnected before collaboration event")
				}
				if event.EventType != kind {
					continue
				}
				if event.BusinessID != businessID || event.ResourceID != conversationID || len(event.Data) > 0 {
					t.Fatal("invalid or overexposed SSE invalidation")
				}
				return
			case <-timer.C:
				t.Fatalf("missing cross-session SSE %s", kind)
			}
		}
	}
	body := []byte(`{"assignee_principal_id":"` + staffID + `"}`)
	missingVersion := governanceHTTPRequest(t, runtime.HTTP.Handler, http.MethodPost, conversationPath+"/assign", body, ownerToken, "", http.StatusUnprocessableEntity)
	_ = missingVersion
	assigned := callBusinessMutationWithMethod(t, runtime.HTTP.Handler, http.MethodPost, conversationPath+"/assign", body, ownerToken, "1")
	if assigned.Code != http.StatusOK {
		t.Fatalf("assignment status=%d body=%s", assigned.Code, assigned.Body.String())
	}
	var result struct {
		Data contract.Conversation `json:"data"`
	}
	if json.Unmarshal(assigned.Body.Bytes(), &result) != nil || result.Data.Assignment["principal_id"] != staffID || result.Data.State != "human_handling" || result.Data.Ownership != "human" || result.Data.AIMode != "disabled" {
		t.Fatal("assignment state or principal was lost")
	}
	await("conversation.assigned")
	stale := callBusinessMutationWithMethod(t, runtime.HTTP.Handler, http.MethodPost, conversationPath+"/assign", body, ownerToken, "1")
	if stale.Code != http.StatusConflict {
		t.Fatalf("stale assignment status=%d", stale.Code)
	}
	for _, path := range []string{conversationPath, base + "/conversations"} {
		got := governanceHTTPRequest(t, runtime.HTTP.Handler, http.MethodGet, path, nil, staffToken, "", http.StatusOK)
		if !strings.Contains(got.Body.String(), `"principal_id":"`+staffID+`"`) {
			t.Fatal("assignment missing from read model")
		}
	}
	noteText := strings.TrimSpace(strings.Repeat("ملاحظة داخلية طويلة ", 400))
	noteBody, _ := json.Marshal(map[string]string{"text": noteText, "related_message_id": inboundID})
	noteKey := uuid.NewString()
	firstNote := governanceHTTPRequest(t, runtime.HTTP.Handler, http.MethodPost, conversationPath+"/notes", noteBody, ownerToken, noteKey, http.StatusAccepted)
	await("conversation.note_added")
	replayedNote := governanceHTTPRequest(t, runtime.HTTP.Handler, http.MethodPost, conversationPath+"/notes", noteBody, ownerToken, noteKey, http.StatusAccepted)
	var original, replayed struct {
		Data contract.Message `json:"data"`
	}
	if json.Unmarshal(firstNote.Body.Bytes(), &original) != nil || json.Unmarshal(replayedNote.Body.Bytes(), &replayed) != nil || original.Data.ID != replayed.Data.ID || !original.Data.CreatedAt.Equal(replayed.Data.CreatedAt) {
		t.Fatal("note retry changed its identity or timestamp")
	}
	changedNote, _ := json.Marshal(map[string]string{"text": "a different note", "related_message_id": inboundID})
	governanceHTTPRequest(t, runtime.HTTP.Handler, http.MethodPost, conversationPath+"/notes", changedNote, ownerToken, noteKey, http.StatusConflict)
	timeline := governanceHTTPRequest(t, runtime.HTTP.Handler, http.MethodGet, conversationPath+"/messages", nil, staffToken, "", http.StatusOK)
	var messages struct {
		Data []contract.Message `json:"data"`
	}
	if json.Unmarshal(timeline.Body.Bytes(), &messages) != nil || len(messages.Data) != 2 {
		t.Fatal("employee timeline missing note")
	}
	var noteID string
	for _, m := range messages.Data {
		if m.Private {
			noteID = string(m.ID)
			if m.RelatedMessageID == nil || *m.RelatedMessageID != inboundID || m.RelatedMessageText == nil || *m.RelatedMessageText != text || m.Text != noteText {
				t.Fatalf("note context mismatch: target=%v quote=%v text_length=%d expected_length=%d", m.RelatedMessageID, m.RelatedMessageText != nil && *m.RelatedMessageText == text, len(m.Text), len(noteText))
			}
		}
	}
	if noteID == "" {
		t.Fatal("internal note was exposed as a public reply")
	}
	for _, target := range []string{uuid.NewString(), noteID} {
		invalid, _ := json.Marshal(map[string]string{"text": "must not save", "related_message_id": target})
		governanceHTTPRequest(t, runtime.HTTP.Handler, http.MethodPost, conversationPath+"/notes", invalid, ownerToken, uuid.NewString(), http.StatusNotFound)
	}
	// The same browser retry key belongs to a different authenticated author.
	staffNote := governanceHTTPRequest(t, runtime.HTTP.Handler, http.MethodPost, conversationPath+"/notes", noteBody, staffToken, noteKey, http.StatusAccepted)
	var staffCreated struct {
		Data contract.Message `json:"data"`
	}
	if json.Unmarshal(staffNote.Body.Bytes(), &staffCreated) != nil || staffCreated.Data.ID == original.Data.ID {
		t.Fatal("note retry identity crossed author scope")
	}
	// Concurrent duplicate saves must commit exactly one additional note.
	concurrentKey := uuid.NewString()
	responses := make(chan *httptest.ResponseRecorder, 6)
	for i := 0; i < 6; i++ {
		go func() {
			request := httptest.NewRequest(http.MethodPost, conversationPath+"/notes", strings.NewReader(string(noteBody))).WithContext(ctx)
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer "+ownerToken)
			request.Header.Set("Idempotency-Key", concurrentKey)
			response := httptest.NewRecorder()
			runtime.HTTP.Handler.ServeHTTP(response, request)
			responses <- response
		}()
	}
	var concurrentID contract.UUID
	for i := 0; i < 6; i++ {
		response := <-responses
		var saved struct {
			Data contract.Message `json:"data"`
		}
		if response.Code != http.StatusAccepted || json.Unmarshal(response.Body.Bytes(), &saved) != nil || saved.Data.ID == "" {
			t.Fatalf("concurrent note save status=%d", response.Code)
		}
		if concurrentID == "" {
			concurrentID = saved.Data.ID
		} else if concurrentID != saved.Data.ID {
			t.Fatal("concurrent note retries created distinct messages")
		}
	}
	var messageCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM communication_messages WHERE business_id=$1::uuid`, businessID).Scan(&messageCount); err != nil || messageCount != 4 {
		t.Fatalf("expected inbound + three independent notes, got %d", messageCount)
	}
	var outbound, outbox int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM outbound_messages WHERE business_id=$1::uuid),(SELECT count(*) FROM outbox_entries WHERE business_id=$1::uuid)`, businessID).Scan(&outbound, &outbox); err != nil || outbound != 0 || outbox != 0 {
		t.Fatal("private note enqueued a customer delivery")
	}
	exerciseInboxIdentityHTTP(t, ctx, runtime, businessID, staffID, ownerToken, staffToken, customerID, connectionID, conversationID, referenceID, inboundID, func(id string) { extraPrincipals = append(extraPrincipals, id) }, events)
}

func callBusinessMutationWithMethod(t *testing.T, handler http.Handler, method, path string, body []byte, token, version string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(string(body)))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("If-Match", version)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
