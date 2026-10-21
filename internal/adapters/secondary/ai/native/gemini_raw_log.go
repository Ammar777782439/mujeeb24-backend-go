package native

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync/atomic"
)

// Gemini raw HTTP response logging is explicitly opt-in because Google may
// include customer text, tool arguments and provider metadata in the body.
// Only the response body is logged; credentials and request headers are not.
const geminiRawResponseLogEnv = "GEMINI_RAW_RESPONSE_LOG"
const geminiRawResponseChunkBytes = 1024

var geminiRawResponseSequence atomic.Uint64

func geminiRawResponseLoggingEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv(geminiRawResponseLogEnv)), "true")
}

// readGeminiHTTPBody preserves the existing 4 MiB + 1 byte parser limit.
// In opt-in mode it reads *all* provider bytes through EOF while retaining
// only the bounded prefix for ordinary parsing/error handling. The log uses
// Go-quoted byte chunks, so even line breaks and invalid UTF-8 can be
// reconstructed byte-for-byte; it does not transform model decisions.
func readGeminiHTTPBody(body io.Reader, status int) ([]byte, error) {
	if !geminiRawResponseLoggingEnabled() {
		return io.ReadAll(io.LimitReader(body, nativeResponseLimit+1))
	}
	return captureGeminiRawHTTPBody(body, status, func(line string) { log.Print(line) })
}

func captureGeminiRawHTTPBody(body io.Reader, status int, emit func(string)) ([]byte, error) {
	id := geminiRawResponseSequence.Add(1)
	emit(fmt.Sprintf("[Gemini] RAW_RESPONSE_BEGIN id=%d status=%d encoding=go-quoted chunk_bytes=%d", id, status, geminiRawResponseChunkBytes))
	retained := make([]byte, 0, geminiRawResponseChunkBytes)
	chunk := make([]byte, geminiRawResponseChunkBytes)
	var total int64
	parts := 0
	emptyReads := 0
	for {
		n, err := body.Read(chunk)
		if n > 0 {
			emptyReads = 0
			parts++
			total += int64(n)
			emit(fmt.Sprintf("[Gemini] RAW_RESPONSE_CHUNK id=%d part=%d raw=%q", id, parts, chunk[:n]))
			if remaining := nativeResponseLimit + 1 - len(retained); remaining > 0 {
				keep := n
				if keep > remaining {
					keep = remaining
				}
				retained = append(retained, chunk[:keep]...)
			}
		} else if err == nil {
			emptyReads++
			if emptyReads >= 100 {
				err = io.ErrNoProgress
			}
		}
		if err == io.EOF {
			emit(fmt.Sprintf("[Gemini] RAW_RESPONSE_END id=%d status=%d parts=%d bytes=%d complete=true", id, status, parts, total))
			return retained, nil
		}
		if err != nil {
			emit(fmt.Sprintf("[Gemini] RAW_RESPONSE_END id=%d status=%d parts=%d bytes=%d complete=false read_error=%q", id, status, parts, total, err.Error()))
			return retained, err
		}
	}
}
