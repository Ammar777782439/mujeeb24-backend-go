package chatwoot

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

func TestClientCreatesContactConversationAndMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("api_access_token") != "chatwoot-token" {
			t.Errorf("api_access_token header = %q", request.Header.Get("api_access_token"))
		}
		body, _ := io.ReadAll(request.Body)
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/accounts/12/contacts":
			if !strings.Contains(string(body), `"inbox_id":34`) || !strings.Contains(string(body), `"identifier":"social-user-1"`) || !strings.Contains(string(body), `"tier":"gold"`) {
				t.Errorf("unexpected contact body: %s", body)
			}
			_, _ = writer.Write([]byte(`{"payload":[{"id":56}]}`))
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/accounts/12/conversations":
			if !strings.Contains(string(body), `"source_id":"social-user-1"`) || !strings.Contains(string(body), `"contact_id":56`) || !strings.Contains(string(body), `"content":"hello"`) {
				t.Errorf("unexpected conversation body: %s", body)
			}
			_, _ = writer.Write([]byte(`{"id":78,"account_id":12,"inbox_id":34}`))
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/accounts/12/conversations/78/messages":
			if !strings.Contains(string(body), `"content":"reply"`) || !strings.Contains(string(body), `"message_type":"outgoing"`) || !strings.Contains(string(body), `"private":false`) {
				t.Errorf("unexpected message body: %s", body)
			}
			_, _ = writer.Write([]byte(`{"id":90,"content":"reply","message_type":"outgoing","created_at":1787659200,"private":false,"status":"sent"}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	client := NewClient(Config{BaseURL: server.URL, APIToken: "chatwoot-token", HTTPClient: server.Client()})
	ctx := httptest.NewRequest(http.MethodPost, "/", nil).Context()
	contact, err := client.CreateContact(ctx, ports.WorkspaceContactDraft{AccountID: 12, InboxID: 34, Name: "Ali", Identifier: "social-user-1", AdditionalAttrs: []byte(`{"tier":"gold"}`)})
	if err != nil || contact.ID != 56 || contact.AccountID != 12 || contact.InboxID != 34 {
		t.Fatalf("CreateContact = %#v, err=%v", contact, err)
	}
	conversation, err := client.CreateConversation(ctx, ports.WorkspaceConversationDraft{AccountID: 12, InboxID: 34, ContactID: 56, SourceID: "social-user-1", InitialText: "hello", Status: "open"})
	if err != nil || conversation.ID != 78 || conversation.InboxID != 34 {
		t.Fatalf("CreateConversation = %#v, err=%v", conversation, err)
	}
	message, err := client.CreateMessage(ctx, ports.WorkspaceMessageDraft{AccountID: 12, ConversationID: 78, Text: "reply"})
	if err != nil || message.ID != 90 || message.Status != "sent" || message.Text != "reply" {
		t.Fatalf("CreateMessage = %#v, err=%v", message, err)
	}
}

func TestClientDecodesRealContactPayloadShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/accounts/12/contacts" {
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"payload":{"contact":{"id":57},"contact_inbox":{"id":91}}}`))
	}))
	defer server.Close()
	client := NewClient(Config{BaseURL: server.URL, APIToken: "chatwoot-token", HTTPClient: server.Client()})
	contact, err := client.CreateContact(httptest.NewRequest(http.MethodPost, "/", nil).Context(), ports.WorkspaceContactDraft{AccountID: 12, InboxID: 34, Name: "Ali", Identifier: "real-payload-shape"})
	if err != nil || contact.ID != 57 {
		t.Fatalf("CreateContact real payload = %#v, err=%v", contact, err)
	}
}

func TestClientReusesExistingContactOnlyForDuplicateIdentifierInSameInbox(t *testing.T) {
	var createCalls, searchCalls int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/accounts/12/contacts":
			createCalls++
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = writer.Write([]byte(`{"message":"Identifier has already been taken"}`))
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/accounts/12/contacts/search":
			searchCalls++
			if request.URL.Query().Get("q") != "social-user-1" {
				t.Errorf("search query=%q", request.URL.Query().Get("q"))
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"payload":[{"id":56,"identifier":"social-user-1","contact_inboxes":[{"inbox":{"id":34}}]}]}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	client := NewClient(Config{BaseURL: server.URL, APIToken: "chatwoot-token", HTTPClient: server.Client()})
	contact, err := client.CreateContact(httptest.NewRequest(http.MethodPost, "/", nil).Context(), ports.WorkspaceContactDraft{AccountID: 12, InboxID: 34, Name: "Ali", Identifier: "social-user-1"})
	if err != nil || contact.ID != 56 || createCalls != 1 || searchCalls != 1 {
		t.Fatalf("contact=%#v err=%v create=%d search=%d", contact, err, createCalls, searchCalls)
	}
}

func TestClientReusesDuplicateContactWhenChatwootSearchOmitsContactInboxes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/accounts/12/contacts":
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = writer.Write([]byte(`{"message":"Identifier has already been taken"}`))
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/accounts/12/contacts/search":
			if request.URL.Query().Get("include_contacts") != "true" {
				t.Errorf("include_contacts=%q", request.URL.Query().Get("include_contacts"))
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"payload":[{"id":56,"identifier":"social-user-1"}]}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	client := NewClient(Config{BaseURL: server.URL, APIToken: "chatwoot-token", HTTPClient: server.Client()})
	contact, err := client.CreateContact(httptest.NewRequest(http.MethodPost, "/", nil).Context(), ports.WorkspaceContactDraft{AccountID: 12, InboxID: 34, Name: "Ali", Identifier: "social-user-1"})
	if err != nil || contact.ID != 56 || contact.InboxID != 34 {
		t.Fatalf("contact=%#v err=%v", contact, err)
	}
}

func TestClientDoesNotTreatUnrelated422AsDuplicateContact(t *testing.T) {
	var searchCalls int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet {
			searchCalls++
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = writer.Write([]byte(`{"message":"Phone number is invalid"}`))
	}))
	defer server.Close()
	client := NewClient(Config{BaseURL: server.URL, APIToken: "chatwoot-token", HTTPClient: server.Client()})
	_, err := client.CreateContact(httptest.NewRequest(http.MethodPost, "/", nil).Context(), ports.WorkspaceContactDraft{AccountID: 12, InboxID: 34, Name: "Ali", Identifier: "social-user-1"})
	providerErr, ok := err.(*Error)
	if !ok || providerErr.StatusCode != http.StatusUnprocessableEntity || searchCalls != 0 {
		t.Fatalf("err=%#v search=%d", err, searchCalls)
	}
}

func TestClientDecodesRealMessageResponseShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/accounts/12/conversations/78/messages" {
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"id":90,"content":"reply","message_type":0,"created_at":1787659200,"private":false,"status":"sent","sender":null,"content_attributes":{},"content_type":"text","conversation_id":78,"inbox_id":34,"source_id":null}`))
	}))
	defer server.Close()
	client := NewClient(Config{BaseURL: server.URL, APIToken: "chatwoot-token", HTTPClient: server.Client()})
	message, err := client.CreateMessage(httptest.NewRequest(http.MethodPost, "/", nil).Context(), ports.WorkspaceMessageDraft{AccountID: 12, ConversationID: 78, Text: "reply", MessageType: "incoming"})
	if err != nil || message.ID != 90 || message.MessageType != "incoming" || message.Status != "sent" {
		t.Fatalf("CreateMessage real payload = %#v, err=%v", message, err)
	}
}

func TestClientRejectsInvalidAttributesAndClassifiesRateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusTooManyRequests)
		_, _ = writer.Write([]byte(`{"message":"rate limited"}`))
	}))
	defer server.Close()
	client := NewClient(Config{BaseURL: server.URL, APIToken: "chatwoot-token", HTTPClient: server.Client()})
	ctx := httptest.NewRequest(http.MethodPost, "/", nil).Context()
	_, err := client.CreateContact(ctx, ports.WorkspaceContactDraft{AccountID: 12, InboxID: 34, Name: "Ali", Identifier: "id", AdditionalAttrs: []byte(`[]`)})
	if err == nil || !strings.Contains(err.Error(), "attributes must be a JSON object") {
		t.Fatalf("invalid attrs err=%v", err)
	}
	_, err = client.CreateMessage(ctx, ports.WorkspaceMessageDraft{AccountID: 12, ConversationID: 78, Text: "reply"})
	providerErr, ok := err.(*Error)
	if !ok || !providerErr.Retryable || providerErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("rate limit err=%#v", err)
	}
}

func TestDecodeObjectAcceptsOnlyObjects(t *testing.T) {
	object, err := decodeObject([]byte(`{"a":1}`))
	if err != nil || object["a"].(float64) != 1 {
		t.Fatalf("object=%#v err=%v", object, err)
	}
	var raw json.RawMessage
	if err := json.Unmarshal([]byte(`{"a":1}`), &raw); err != nil {
		t.Fatal(err)
	}
	if _, err := decodeObject(raw); err != nil {
		t.Fatalf("raw object: %v", err)
	}
}
