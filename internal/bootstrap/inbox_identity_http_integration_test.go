package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/contract"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/dto"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/persistence/postgres"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// Uses real authenticated sessions and PostgreSQL row locks. No provider sends.
func exerciseInboxIdentityHTTP(t *testing.T, ctx context.Context, runtime *APIRuntime, businessID, staffID, ownerToken, staffToken, customerID, connectionID, conversationID, referenceID, inboundID string, registerPrincipal func(string), events <-chan ports.RealtimeEvent) {
	t.Helper()
	pool := runtime.Database.Pool()
	handler := runtime.HTTP.Handler
	otherID := uuid.NewString()
	registerPrincipal(otherID)
	hash, err := bcrypt.GenerateFromPassword([]byte("isolated-http-fixture-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	email := "inbox-" + otherID + "@identity-http.test"
	_, err = postgres.NewAuthenticationRepository(runtime.Database).EnsurePrincipalAndMembership(ctx, ports.PrincipalRecord{ID: commands.PrincipalID(otherID), Email: email, DisplayName: "الموظف الثاني", PasswordHash: string(hash), Status: "active"}, commands.BusinessID(businessID), "agent", []string{"conversations:read", "conversations:reply", "conversations:assign"}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	login, _ := json.Marshal(map[string]string{"email": email, "password": "isolated-http-fixture-password"})
	otherToken := extractAccessToken(t, governanceHTTPRequest(t, handler, http.MethodPost, "/api/v1/auth/login", login, "", "", http.StatusOK))
	base := "/api/v1/businesses/" + businessID + "/conversations"
	path := base + "/" + conversationID
	get := func(path, token string) *httptest.ResponseRecorder {
		return governanceHTTPRequest(t, handler, http.MethodGet, path, nil, token, "", http.StatusOK)
	}
	sharedID, sharedRef := uuid.NewString(), uuid.NewString()
	if _, err = pool.Exec(ctx, `INSERT INTO conversations(id,business_id,customer_id,state,ownership,priority,last_activity_at,created_at,updated_at) VALUES($1::uuid,$2::uuid,$3::uuid,'waiting_human','none','normal',now(),now(),now())`, sharedID, businessID, customerID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO conversation_references(id,business_id,conversation_id,system,provider_ref,resource_type,resource_id,connection_id,conversation_kind,is_current,mapping_status,created_at,updated_at) VALUES($1::uuid,$2::uuid,$3::uuid,'provider','socialapi','conversation',$4,$5::uuid,'dm',true,'active',now(),now())`, sharedRef, businessID, sharedID, "shared-"+sharedID, connectionID); err != nil {
		t.Fatal(err)
	}
	var overview struct {
		Data dto.ConversationInboxOverview `json:"data"`
	}
	if json.Unmarshal(get(base+"/inbox-overview?scope=all", staffToken).Body.Bytes(), &overview) != nil || overview.Data.Total != 2 || overview.Data.AssignedToMe != 1 || overview.Data.Unassigned != 1 {
		t.Fatalf("incorrect server inbox counts: %+v", overview.Data)
	}
	var page struct {
		Data       []contract.Conversation `json:"data"`
		Pagination struct {
			HasMore    bool   `json:"has_more"`
			NextCursor string `json:"next_cursor"`
		} `json:"pagination"`
	}
	if json.Unmarshal(get(base+"?scope=all&limit=1", staffToken).Body.Bytes(), &page) != nil || len(page.Data) != 1 || !page.Pagination.HasMore || page.Pagination.NextCursor == "" {
		t.Fatal("scoped inbox pagination missing")
	}
	next := get(base+"?scope=all&limit=1&cursor="+page.Pagination.NextCursor, staffToken)
	if strings.Contains(next.Body.String(), `"id":"`+string(page.Data[0].ID)+`"`) {
		t.Fatal("pagination repeated first conversation")
	}
	if !strings.Contains(get(base, otherToken).Body.String(), sharedID) || strings.Contains(get(base, otherToken).Body.String(), conversationID) {
		t.Fatal("employee list exposed someone else's conversation")
	}
	for _, suffix := range []string{"", "/messages", "/summary"} {
		governanceHTTPRequest(t, handler, http.MethodGet, path+suffix, nil, otherToken, "", http.StatusNotFound)
	}
	for _, item := range []struct{ Suffix, Body string }{
		{"/notes", `{"text":"must not send"}`}, {"/read", `{}`}, {"/messages", `{"text":"must not send"}`}, {"/labels", `{"add":["private"]}`},
	} {
		governanceHTTPRequest(t, handler, http.MethodPost, path+item.Suffix, []byte(item.Body), otherToken, uuid.NewString(), http.StatusNotFound)
	}

	forbiddenAssign := callBusinessMutationWithMethod(t, handler, http.MethodPost, path+"/assign", []byte(`{"assignee_principal_id":"`+otherID+`"}`), otherToken, "2")
	if forbiddenAssign.Code != http.StatusNotFound {
		t.Fatalf("assign permission bypassed another agent's scope: %d", forbiddenAssign.Code)
	}
	forbiddenState := callBusinessMutationWithMethod(t, handler, http.MethodPatch, path, []byte(`{"state":"closed"}`), otherToken, "2")
	if forbiddenState.Code != http.StatusNotFound {
		t.Fatalf("state edit bypassed another agent's scope: %d", forbiddenState.Code)
	}
	// Shared conversations require an explicit atomic claim before any reply.
	governanceHTTPRequest(t, handler, http.MethodPost, base+"/"+sharedID+"/messages", []byte(`{"text":"claim first"}`), staffToken, uuid.NewString(), http.StatusConflict)
	empty := get(path+"/summary", staffToken)
	if !strings.Contains(empty.Body.String(), `"status":"not_available"`) {
		t.Fatal("missing summary fabricated content")
	}
	states := postgres.NewConversationStateRepository(runtime.Database)
	state, err := states.EnsureSummaryState(ctx, businessID, conversationID)
	if err != nil {
		t.Fatal(err)
	}
	summary := "طلب العميل معرفة السعر وإرسال صورة أصلية. يحتاج الموظف متابعة الطلب."
	saved, err := states.SaveSummaryIfVersion(ctx, businessID, conversationID, state.Version, summary, inboundID, 1)
	if err != nil || !saved {
		t.Fatalf("save summary checkpoint: %v", err)
	}
	var summaryResponse struct {
		Data dto.ConversationSummary `json:"data"`
	}
	if json.Unmarshal(get(path+"/summary", staffToken).Body.Bytes(), &summaryResponse) != nil || summaryResponse.Data.Status != "available" || summaryResponse.Data.Text != summary || summaryResponse.Data.CoveredInboundTurns != 1 {
		t.Fatal("persisted conversation summary missing")
	}
	// Sender comes from each authenticated actor, independent of assignee.
	send := func(token, text string) contract.Message {
		t.Helper()
		body, _ := json.Marshal(map[string]string{"text": text})
		response := governanceHTTPRequest(t, handler, http.MethodPost, path+"/messages", body, token, uuid.NewString(), http.StatusAccepted)
		var result struct {
			Data contract.Message `json:"data"`
		}
		if json.Unmarshal(response.Body.Bytes(), &result) != nil || result.Data.AuthorPrincipalID == nil || result.Data.AuthorDisplayName == nil || *result.Data.AuthorDisplayName == "" {
			t.Fatal("message acknowledgment lost authenticated author")
		}
		return result.Data
	}
	staffMessage := send(staffToken, "رد من الموظف الأول")
	if *staffMessage.AuthorPrincipalID != staffID {
		t.Fatal("wrong staff author")
	}
	ownerMessage := send(ownerToken, "رد من المالك")
	if *ownerMessage.AuthorPrincipalID == staffID {
		t.Fatal("owner message incorrectly attributed to assignee")
	}
	legacyText := "رد قديم دون هوية محفوظة"
	legacyID := uuid.NewString()
	if _, err = postgres.NewMessageRepository(runtime.Database).Record(ctx, ports.CommunicationMessageDraft{ID: legacyID, BusinessID: businessID, ConversationReferenceID: referenceID, Direction: "outbound", Origin: "human", Transport: "provider", ContentType: "text", TextContent: &legacyText, ContentReference: "legacy:" + legacyID, Visibility: "public", OccurredAt: time.Now().UTC(), CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	var history struct {
		Data []contract.Message `json:"data"`
	}
	json.Unmarshal(get(path+"/messages", staffToken).Body.Bytes(), &history)
	found := map[string]bool{}
	for _, m := range history.Data {
		if m.ID == staffMessage.ID || m.ID == ownerMessage.ID {
			if m.AuthorPrincipalID == nil {
				t.Fatal("history discarded persisted author")
			}
			found[string(m.ID)] = true
		}
		if string(m.ID) == legacyID && m.AuthorPrincipalID != nil {
			t.Fatal("historical author was invented")
		}
	}
	if len(found) != 2 {
		t.Fatal("send acknowledgement is not the persisted timeline message")
	}
	// Both sessions observe version one; exactly one can own the shared row.
	replies := make(chan struct {
		ID       string
		Response *httptest.ResponseRecorder
	}, 2)
	for _, candidate := range []struct{ ID, Token string }{{staffID, staffToken}, {otherID, otherToken}} {
		go func(id, token string) {
			response := callBusinessMutationWithMethod(t, handler, http.MethodPost, base+"/"+sharedID+"/claim", nil, token, "1")
			replies <- struct {
				ID       string
				Response *httptest.ResponseRecorder
			}{id, response}
		}(candidate.ID, candidate.Token)
	}
	wins := 0
	for i := 0; i < 2; i++ {
		result := <-replies
		if result.Response.Code == http.StatusOK {
			wins++
			if !strings.Contains(result.Response.Body.String(), `"principal_id":"`+result.ID+`"`) {
				t.Fatal("claim assigned another actor")
			}
		} else if result.Response.Code != http.StatusNotFound && result.Response.Code != http.StatusConflict {
			t.Fatalf("claim race status=%d body=%s", result.Response.Code, result.Response.Body.String())
		}
	}
	if wins != 1 {
		t.Fatalf("claim race has %d winners", wins)
	}
	// Transfer revokes old session access and preserves historical sender identity.
	var current struct {
		Data contract.Conversation `json:"data"`
	}
	json.Unmarshal(get(path, ownerToken).Body.Bytes(), &current)
	transfer := callBusinessMutationWithMethod(t, handler, http.MethodPost, path+"/assign", []byte(fmt.Sprintf(`{"assignee_principal_id":%q}`, otherID)), ownerToken, current.Data.ResourceVersion)
	if transfer.Code != http.StatusOK {
		t.Fatalf("transfer failed: %d %s", transfer.Code, transfer.Body.String())
	}
	// The previous assignee gets a generic invalidation, never another agent’s conversation ID.
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	receivedResync := false
	for !receivedResync {
		select {
		case event, ok := <-events:
			if !ok {
				t.Fatal("SSE closed before ownership resync")
			}
			if event.EventType == "realtime.resync_required" {
				if event.ResourceID != businessID || event.ResourceType != "business" || len(event.Data) > 0 {
					t.Fatal("SSE leaked transferred conversation")
				}
				receivedResync = true
			}
		case <-timer.C:
			t.Fatal("former assignee did not receive access resync")
		}
	}
	governanceHTTPRequest(t, handler, http.MethodGet, path+"/messages", nil, staffToken, "", http.StatusNotFound)
	governanceHTTPRequest(t, handler, http.MethodPost, path+"/messages", []byte(`{"text":"stale composer"}`), staffToken, uuid.NewString(), http.StatusNotFound)
	if !strings.Contains(get(path+"/summary", otherToken).Body.String(), summary) {
		t.Fatal("new assignee cannot read summary")
	}
	if *send(otherToken, "رد من الموظف الثاني").AuthorPrincipalID != otherID {
		t.Fatal("new assignee author missing")
	}
}
