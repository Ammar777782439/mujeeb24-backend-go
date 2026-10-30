package merchantcatalogtrace

import (
	"bytes"
	"context"
	"log"
	"strings"
	"testing"
)

func TestTraceRequiresExplicitFlagAndMarkedSession(t *testing.T) {
	t.Setenv(EnvironmentVariable, "")
	if Enabled(WithSession(context.Background(), "test-session")) {
		t.Fatal("tracing must be disabled by default")
	}
	t.Setenv(EnvironmentVariable, "true")
	if Enabled(context.Background()) || Enabled(nil) || Enabled(WithSession(context.Background(), "")) {
		t.Fatal("unmarked requests must not be traced")
	}
	if !Enabled(WithSession(context.Background(), "test-session")) {
		t.Fatal("marked merchant session should be traced")
	}
}

func TestDumpPreservesCompleteUnicodePayloadInChunks(t *testing.T) {
	t.Setenv(EnvironmentVariable, "true")
	ctx := WithSession(context.Background(), "test-session")
	payload := []byte(strings.Repeat("ص", logChunkRunes+100))
	var output bytes.Buffer
	prior := log.Writer()
	log.SetOutput(&output)
	defer log.SetOutput(prior)
	Dump(ctx, "deepseek_http_raw_response", payload)

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected two log lines, got %d", len(lines))
	}
	var combined strings.Builder
	for i, line := range lines {
		if !strings.Contains(line, "session=\"test-session\"") ||
			!strings.Contains(line, "part="+string(rune('1'+i))+"/2") {
			t.Fatalf("missing session or chunk number: %s", line)
		}
		_, part, ok := strings.Cut(line, "payload=")
		if !ok {
			t.Fatalf("missing payload: %s", line)
		}
		combined.WriteString(part)
	}
	if combined.String() != string(payload) {
		t.Fatal("chunking modified model output")
	}
}
