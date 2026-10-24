package catalogdump

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestOptInFileAndEmptyValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "catalog.jsonl")
	t.Setenv("MUJEEB_CATALOG_DEBUG_FILE", path)
	t.Setenv("MUJEEB_CATALOG_DEBUG_DUMP", "false")
	ctx := WithRun(context.Background(), "run-1")
	Record(ctx, "ignore", map[string]any{"empty": ""})
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("disabled dump created a file: %v", err)
	}
	t.Setenv("MUJEEB_CATALOG_DEBUG_DUMP", "true")
	Record(context.Background(), "ignore", "missing run")
	Record(ctx, "catalog_page", map[string]any{"empty": "", "null": nil})
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scan := bufio.NewScanner(f)
	if !scan.Scan() {
		t.Fatal("no dump written")
	}
	var got struct {
		RunID   string         `json:"run_id"`
		Stage   string         `json:"stage"`
		Payload map[string]any `json:"payload"`
	}
	if err := json.Unmarshal(scan.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.RunID != "run-1" || got.Stage != "catalog_page" || got.Payload["empty"] != "" {
		t.Fatalf("unexpected file entry: %+v", got)
	}
	if value, ok := got.Payload["null"]; !ok || value != nil {
		t.Fatalf("null not preserved: %+v", got.Payload)
	}
	if scan.Scan() {
		t.Fatal("unexpected extra entry")
	}
	if err := scan.Err(); err != nil {
		t.Fatal(err)
	}
}
