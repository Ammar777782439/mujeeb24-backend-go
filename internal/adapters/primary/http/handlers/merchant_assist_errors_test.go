package handlers

import (
	"errors"
	"strings"
	"testing"

	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/merchantassist"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/domain/ai/contracts"
)

func TestMerchantAssistErrorsDistinguishFailuresWithoutLeakingCause(t *testing.T) {
	tests := []struct {
		name string
		err error
		code appErrors.Code
		messagePart string
	}{
		{"permission", merchantassist.ErrReadDenied, appErrors.CodeForbidden, "صلاحية"},
		{"projection permission", contracts.ErrDeniedProjection, appErrors.CodeForbidden, "صلاحية"},
		{"invalid request", merchantassist.ErrInvalidReadRequest, appErrors.CodeValidation, "غير صالح"},
		{"unknown projection", contracts.ErrUnknownProjection, appErrors.CodeValidation, "غير صالح"},
		{"bad evidence", merchantassist.ErrInvalidSourceEvidence, appErrors.CodeExternalDependency, "بيانات المصدر"},
		{"source unavailable", merchantassist.ErrReadUnavailable, appErrors.CodeExternalDependency, "قراءة البيانات"},
		{"not configured", merchantassist.ErrAssistantUnavailable, appErrors.CodeExternalDependency, "غير مهيأة"},
		{"provider failure", errors.New("provider token SECRET-DO-NOT-LOG"), appErrors.CodeExternalDependency, "مزود"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			public := merchantAssistPublicError(test.err)
			if public.Code != test.code || !strings.Contains(public.Message, test.messagePart) {
				t.Fatalf("incorrect category/message: %+v", public)
			}
			if strings.Contains(public.Message, "SECRET-DO-NOT-LOG") {
				t.Fatal("raw provider error leaked to UI")
			}
		})
	}
}
