package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestListenerRestartsAfterPanicAndConnectionFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	attempts := 0
	var warnings []string
	reconnectListener(ctx, func(ctx context.Context) error {
		attempts++
		switch attempts {
		case 1:
			panic("payload must not enter logs")
		case 2:
			return errors.New("connection dropped")
		default:
			cancel()
			return ctx.Err()
		}
	}, func(format string, args ...any) { warnings = append(warnings, fmt.Sprintf(format, args...)) })
	if attempts != 3 || len(warnings) != 2 || strings.Contains(strings.Join(warnings, " "), "payload must not enter logs") {
		t.Fatalf("listener recovery attempts=%d warnings=%d", attempts, len(warnings))
	}
}

func TestListenerCancellationStopsBackoffWithoutAnotherAttempt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	attempts := 0
	reconnectListener(ctx, func(context.Context) error { attempts++; return errors.New("offline") }, func(string, ...any) { cancel() })
	if attempts != 1 {
		t.Fatalf("canceled listener attempted %d reconnects", attempts)
	}
}
