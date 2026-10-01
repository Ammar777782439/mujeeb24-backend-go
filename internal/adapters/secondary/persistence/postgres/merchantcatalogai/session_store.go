package merchantcatalogaipersistence

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/merchantcatalogai"
)

type SessionStore struct {
	Adapter *Adapter
}

func NewSessionStore(adapter *Adapter) *SessionStore {
	return &SessionStore{Adapter: adapter}
}

func (r *SessionStore) CreateSession(ctx context.Context, businessID, principalID string) (string, error) {
	if r == nil || r.Adapter == nil {
		return "", ErrPoolClosed
	}
	if strings.TrimSpace(businessID) == "" || strings.TrimSpace(principalID) == "" {
		return "", invalidRepositoryInput("merchant_catalog_ai.session.create", "business_id and principal_id are required")
	}
	exec, err := r.Adapter.Executor(ctx)
	if err != nil {
		return "", err
	}
	id := uuid.NewString()
	now := time.Now().UTC()
	_, err = exec.Exec(ctx,
		"INSERT INTO merchant_ai_sessions (id, business_id, principal_id, created_at, updated_at) VALUES ($1::uuid,$2::uuid,$3::uuid,$4,$5)",
		id, businessID, principalID, now, now,
	)
	if err != nil {
		return "", &RepositoryError{Operation:"merchant_catalog_ai.session.create", Kind:RepositoryInvalid, Err:fmt.Errorf("insert merchant_ai_sessions: %w",err)}
	}
	return id, nil
}

func (r *SessionStore) ListMessages(ctx context.Context, businessID, sessionID string, limit int) ([]merchantcatalogai.SessionMessage, error) {
	if r == nil || r.Adapter == nil {
		return nil, ErrPoolClosed
	}
	if strings.TrimSpace(businessID) == "" || strings.TrimSpace(sessionID) == "" {
		return nil, invalidRepositoryInput("merchant_catalog_ai.message.list", "business_id and session_id are required")
	}
	if limit <= 0 { limit = 12 }
	if limit > 100 { limit = 100 }
	exec, err := r.Adapter.Executor(ctx)
	if err != nil { return nil, err }
	rows, err := exec.Query(ctx,
		"SELECT id::text, sender_type, text, created_at FROM merchant_ai_messages WHERE business_id=$1::uuid AND session_id=$2::uuid ORDER BY created_at DESC LIMIT $3",
		businessID, sessionID, limit,
	)
	if err != nil {
		return nil, &RepositoryError{Operation:"merchant_catalog_ai.message.list", Kind:RepositoryInvalid, Err:fmt.Errorf("query merchant_ai_messages: %w",err)}
	}
	defer rows.Close()
	out := make([]merchantcatalogai.SessionMessage,0,limit)
	for rows.Next() {
		var m merchantcatalogai.SessionMessage
		if err := rows.Scan(&m.ID,&m.SenderType,&m.Text,&m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out,m)
	}
	if err := rows.Err(); err != nil { return nil, err }
	for i,j := 0,len(out)-1;i<j;i,j=i+1,j-1 { out[i],out[j]=out[j],out[i] }
	return out,nil
}

func (r *SessionStore) AppendMessage(ctx context.Context, businessID, sessionID, senderType, text string) (string,error) {
	if r == nil || r.Adapter == nil { return "", ErrPoolClosed }
	if senderType != "merchant" && senderType != "assistant" {
		return "", invalidRepositoryInput("merchant_catalog_ai.message.append", "sender_type must be merchant or assistant")
	}
	if strings.TrimSpace(text) == "" { return "", invalidRepositoryInput("merchant_catalog_ai.message.append", "text is required") }
	exec, err := r.Adapter.Executor(ctx)
	if err != nil { return "", err }
	id := uuid.NewString()
	now := time.Now().UTC()
	_, err = exec.Exec(ctx,
		"INSERT INTO merchant_ai_messages (id,business_id,session_id,sender_type,text,created_at) VALUES ($1::uuid,$2::uuid,$3::uuid,$4,$5,$6)",
		id,businessID,sessionID,senderType,text,now,
	)
	if err != nil {
		return "", &RepositoryError{Operation:"merchant_catalog_ai.message.append", Kind:RepositoryInvalid, Err:fmt.Errorf("insert merchant_ai_messages: %w",err)}
	}
	_, _ = exec.Exec(ctx,"UPDATE merchant_ai_sessions SET updated_at=$1 WHERE business_id=$2::uuid AND id=$3::uuid",now,businessID,sessionID)
	return id,nil
}

func (r *SessionStore) GetStickyCatalogID(ctx context.Context, businessID, sessionID string) (string,error) {
	if r == nil || r.Adapter == nil { return "", ErrPoolClosed }
	exec, err := r.Adapter.Executor(ctx)
	if err != nil { return "", err }
	var value *string
	err = exec.QueryRow(ctx,"SELECT target_catalog_id::text FROM merchant_ai_sessions WHERE business_id=$1::uuid AND id=$2::uuid",businessID,sessionID).Scan(&value)
	if err != nil {
		if err == pgx.ErrNoRows { return "", &RepositoryError{Operation:"merchant_catalog_ai.session.get_catalog",Kind:RepositoryNotFound,Err:err} }
		return "", err
	}
	if value == nil { return "",nil }
	return *value,nil
}

func (r *SessionStore) SetStickyCatalogID(ctx context.Context,businessID,sessionID,catalogID string) error {
	if r == nil || r.Adapter == nil { return ErrPoolClosed }
	exec, err := r.Adapter.Executor(ctx)
	if err != nil { return err }
	now := time.Now().UTC()
	tag, err := exec.Exec(ctx,
		"UPDATE merchant_ai_sessions SET target_catalog_id=$1::uuid,target_catalog_set_at=$2,updated_at=$2 WHERE business_id=$3::uuid AND id=$4::uuid",
		catalogID,now,businessID,sessionID,
	)
	if err != nil { return err }
	if tag.RowsAffected()==0 { return &RepositoryError{Operation:"merchant_catalog_ai.session.set_catalog",Kind:RepositoryNotFound,Err:fmt.Errorf("session not found")} }
	return nil
}

var _ merchantcatalogai.SessionStore = (*SessionStore)(nil)
