package postgres

import (
	"context"
	"errors"
	"testing"
)

type fakeTx struct {
	commitErr   error
	rollbackErr error
	committed   bool
	rolledBack  bool
}

func (f *fakeTx) Commit(context.Context) error   { f.committed = true; return f.commitErr }
func (f *fakeTx) Rollback(context.Context) error { f.rolledBack = true; return f.rollbackErr }

type fakeBeginner struct {
	tx     tx
	err    error
	begins int
}

func (f *fakeBeginner) Begin(context.Context) (tx, error) { f.begins++; return f.tx, f.err }

func TestWithinCommitsSuccessfulUnit(t *testing.T) {
	fake := &fakeTx{}
	beginner := &fakeBeginner{tx: fake}
	adapter := &Adapter{beginner: beginner}
	called := false
	if err := adapter.Within(context.Background(), func(ctx context.Context) error {
		called = true
		if _, ok := TransactionFromContext(ctx); !ok {
			t.Fatal("transaction missing from callback context")
		}
		return nil
	}); err != nil {
		t.Fatalf("Within: %v", err)
	}
	if !called || !fake.committed || fake.rolledBack || beginner.begins != 1 {
		t.Fatalf("unexpected state: called=%v committed=%v rolledBack=%v begins=%d", called, fake.committed, fake.rolledBack, beginner.begins)
	}
}

func TestWithinRollsBackCallbackFailure(t *testing.T) {
	fake := &fakeTx{}
	want := errors.New("unit failure")
	adapter := &Adapter{beginner: &fakeBeginner{tx: fake}}
	if err := adapter.Within(context.Background(), func(context.Context) error { return want }); !errors.Is(err, want) {
		t.Fatalf("expected callback error, got %v", err)
	}
	if fake.committed || !fake.rolledBack {
		t.Fatalf("expected rollback only: committed=%v rolledBack=%v", fake.committed, fake.rolledBack)
	}
}

func TestWithinRejectsNestedTransaction(t *testing.T) {
	outer := &fakeTx{}
	adapter := &Adapter{beginner: &fakeBeginner{tx: &fakeTx{}}}
	ctx := context.WithValue(context.Background(), txContextKey{}, outer)
	if err := adapter.Within(ctx, func(context.Context) error { return nil }); !errors.Is(err, ErrNestedTransaction) {
		t.Fatalf("expected nested transaction error, got %v", err)
	}
}

func TestWithinHonorsCanceledContextBeforeBegin(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	beginner := &fakeBeginner{tx: &fakeTx{}}
	if err := (&Adapter{beginner: beginner}).Within(ctx, func(context.Context) error { t.Fatal("callback must not run"); return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
	if beginner.begins != 0 {
		t.Fatalf("begin called %d times", beginner.begins)
	}
}

func TestWithinRollsBackWhenCallbackObservesCancellation(t *testing.T) {
	fake := &fakeTx{}
	adapter := &Adapter{beginner: &fakeBeginner{tx: fake}}
	ctx, cancel := context.WithCancel(context.Background())
	if err := adapter.Within(ctx, func(callbackCtx context.Context) error { cancel(); return callbackCtx.Err() }); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected callback cancellation, got %v", err)
	}
	if !fake.rolledBack || fake.committed {
		t.Fatalf("expected rollback after cancellation")
	}
}

func TestAdapterLifecycleAndInvalidOpen(t *testing.T) {
	adapter := NewFromPool(nil)
	if !errors.Is(adapter.Ping(context.Background()), ErrPoolClosed) {
		t.Fatal("nil pool must report ErrPoolClosed")
	}
	adapter.Close()
	if _, err := Open(context.Background(), "", DefaultPoolConfig()); err == nil {
		t.Fatal("empty database URL must fail")
	}
}
