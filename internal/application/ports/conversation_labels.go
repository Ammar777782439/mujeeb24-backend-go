package ports

import "context"

type ConversationLabelRepository interface {
	List(ctx context.Context, businessID, conversationID string) ([]string, error)
	Apply(ctx context.Context, businessID, conversationID string, add, remove []string) error
}
