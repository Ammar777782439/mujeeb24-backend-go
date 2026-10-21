package native

import (
	"bytes"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"
)

func TestGeminiRawBodyPreservesSuccessAndErrorVerbatim(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"success", 200, "{\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"مرحباً\\nيا عميل\"}]}}],\"usageMetadata\":{\"promptTokenCount\":3}}"},
		{"google_error", 503, "{\n  \"error\": {\"code\": 503, \"message\": \"Google says overloaded\", \"status\": \"UNAVAILABLE\"}\n}"},
		{"large_multichunk", 200, strings.Repeat("رد عربي من Google\n", 500)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var lines []string
			parsed, err := captureGeminiRawHTTPBody(strings.NewReader(tc.body), tc.status, func(s string) { lines = append(lines, s) })
			if err != nil {
				t.Fatal(err)
			}
			if string(parsed) != tc.body {
				t.Fatal("parser body differed from Google bytes")
			}
			if !strings.Contains(lines[0], "RAW_RESPONSE_BEGIN") || !strings.Contains(lines[len(lines)-1], "complete=true") {
				t.Fatal("missing boundaries")
			}
			var assembled bytes.Buffer
			for _, line := range lines {
				if !strings.Contains(line, "RAW_RESPONSE_CHUNK") {
					continue
				}
				p := strings.SplitN(line, " raw=", 2)
				if len(p) != 2 {
					t.Fatalf("missing raw chunk: %s", line)
				}
				decoded, err := strconv.Unquote(p[1])
				if err != nil {
					t.Fatal(err)
				}
				assembled.WriteString(decoded)
			}
			if !bytes.Equal(assembled.Bytes(), []byte(tc.body)) {
				t.Fatal("log chunks did not reconstruct the exact Google body")
			}
		})
	}
}

func TestGeminiRawBodyDoesNotLogWhenDisabled(t *testing.T) {
	t.Setenv(geminiRawResponseLogEnv, "false")
	input := strings.NewReader("{\"error\":{\"code\":503}}")
	got, err := readGeminiHTTPBody(input, 503)
	if err != nil || string(got) != "{\"error\":{\"code\":503}}" {
		t.Fatalf("disabled read mismatch: %q %v", got, err)
	}
}

type interruptedBody struct{ called bool }

func (r *interruptedBody) Read(buf []byte) (int, error) {
	if !r.called {
		r.called = true
		return copy(buf, []byte("partial response")), nil
	}
	return 0, io.ErrUnexpectedEOF
}

func TestGeminiRawBodyMarksIncompleteRead(t *testing.T) {
	var lines []string
	raw, err := captureGeminiRawHTTPBody(&interruptedBody{}, 200, func(s string) { lines = append(lines, s) })
	if !errors.Is(err, io.ErrUnexpectedEOF) || string(raw) != "partial response" || !strings.Contains(lines[len(lines)-1], "complete=false") {
		t.Fatalf("incomplete body not reported: %q, %v, %v", raw, err, lines)
	}
}

func TestGeminiRawBodyLogsBeyondParserLimitWithoutOmittingBytes(t *testing.T) {
	large := bytes.Repeat([]byte("x"), nativeResponseLimit+23)
	var reconstructed int
	var chunks int
	parsed, err := captureGeminiRawHTTPBody(bytes.NewReader(large), 200, func(line string) {
		if !strings.Contains(line, "RAW_RESPONSE_CHUNK") {
			return
		}
		parts := strings.SplitN(line, " raw=", 2)
		x, decodeErr := strconv.Unquote(parts[1])
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}
		reconstructed += len(x)
		chunks++
	})
	if err != nil || len(parsed) != nativeResponseLimit+1 || reconstructed != len(large) || chunks < 2 {
		t.Fatalf("lost bytes: parsed=%d captured=%d original=%d, err=%v", len(parsed), reconstructed, len(large), err)
	}
}
