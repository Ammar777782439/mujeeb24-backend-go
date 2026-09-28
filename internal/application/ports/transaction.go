package ports

import "context"

// TransactionManager defines the atomic application boundary. The callback
// receives a derived context carrying the transaction owned by the adapter.
// Application code depends on this contract, never on pgx or database/sql.
type TransactionManager interface {
	Within(ctx context.Context, fn func(context.Context) error) error
}
