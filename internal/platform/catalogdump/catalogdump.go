// Package catalogdump stores opt-in, development-only AI catalog traces as JSONL.
package catalogdump

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type contextKey struct{}

var (
	mu       sync.Mutex
	warnOnce sync.Once
)

// WithRun propagates the business execution run ID into native provider calls.
func WithRun(ctx context.Context, runID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, contextKey{}, strings.TrimSpace(runID))
}

// Raw preserves valid provider JSON; malformed/partial response bytes remain text.
func Raw(body []byte) any {
	if json.Valid(body) {
		return json.RawMessage(append([]byte(nil), body...))
	}
	return string(body)
}

// Record writes complete stage payloads to a file, never standard output.
// A failed write does not alter model or commercial execution.
func Record(ctx context.Context, stage string, payload any) {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("MUJEEB_CATALOG_DEBUG_DUMP")), "true") ||
		ctx == nil || strings.TrimSpace(stage) == "" {
		return
	}
	runID, _ := ctx.Value(contextKey{}).(string)
	path := strings.TrimSpace(os.Getenv("MUJEEB_CATALOG_DEBUG_FILE"))
	if runID == "" || path == "" {
		return
	}
	body, err := json.Marshal(struct {
		Time    time.Time `json:"time"`
		RunID   string    `json:"run_id"`
		Stage   string    `json:"stage"`
		Payload any       `json:"payload"`
	}{time.Now().UTC(), runID, stage, payload})
	if err != nil {
		warn(err)
		return
	}
	mu.Lock()
	defer mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		warn(err)
		return
	}
	f, err := os.OpenFile(filepath.Clean(path), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		warn(err)
		return
	}
	defer f.Close()
	if _, err := f.Write(append(body, '\n')); err != nil {
		warn(err)
	}
}

func warn(err error) {
	// No diagnostic payload is ever sent to the terminal as a fallback.
	warnOnce.Do(func() { log.Printf("[CatalogDump] FILE_WRITE_FAILED err=%v", err) })
}
