package merchantassist

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestConservativeSanitizerBlocksSecretsAndRedactsContacts(t *testing.T) {
	s := ConservativeSanitizer{}
	cases := []struct {
		raw     string
		blocked bool
		hide    string
	}{
		{"أريد خدمة اتصل على +967 777 888 999", false, "777"},
		{"البريد test@example.com والرابط https://example.com/path", false, "example.com"},
		{"OTP: 554433", true, ""},
		{"كلمة المرور: confidential123", true, ""},
		{"رقم الهوية 1234567890", true, ""},
		{"المعرف 123456789012345", false, "123456789012345"},
	}
	for _, tc := range cases {
		got, err := s.Sanitize(context.Background(), ContentMessage, tc.raw)
		if tc.blocked {
			if !errors.Is(err, errSensitiveContent) {
				t.Fatalf("should reject sensitive phrase %q: %v", tc.raw, err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("unexpected reject: %v", err)
		}
		if strings.Contains(got, tc.hide) && tc.hide != "" {
			t.Fatalf("sensitive value leaked in %q", got)
		}
	}
	if _, err := s.Sanitize(context.Background(), ContentKind("other"), "abc"); !errors.Is(err, errSensitiveContent) {
		t.Fatal("unknown content kind")
	}
}
