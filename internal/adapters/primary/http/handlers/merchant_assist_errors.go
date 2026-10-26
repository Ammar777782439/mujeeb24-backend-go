package handlers

import (
	"context"
	"errors"
	"log"
	"fmt"

	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/merchantassist"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/domain/ai/contracts"
)

// Never expose a database error, provider response body, credential, prompt,
// or customer text through HTTP errors or operational logs. Clients get a
// stable error class and actionable Arabic message.
func merchantAssistPublicError(err error) *appErrors.Error {
	var result *appErrors.Error
	switch {
	case errors.Is(err, merchantassist.ErrReadDenied), errors.Is(err, contracts.ErrDeniedProjection):
		result = appErrors.New(appErrors.CodeForbidden, "لا توجد صلاحية كافية لقراءة هذا المصدر، أو أن تشغيل المساعد معطّل لهذه المنشأة.")
	case errors.Is(err, merchantassist.ErrInvalidReadRequest), errors.Is(err, contracts.ErrUnknownProjection), errors.Is(err, merchantassist.ErrInvalidModelPlan):
		result = appErrors.New(appErrors.CodeValidation, "طلب المساعدة غير صالح؛ راجع مصدر البيانات ومعرّف السجل أو موضوع البحث.")
	case errors.Is(err, merchantassist.ErrFullScanTooLarge):
		result = appErrors.New(appErrors.CodeExternalDependency, "عدد السجلات كبير ولا يمكن إكمال التقرير الشامل ضمن حدود الطلب الواحد؛ لا يوجد إجمالي مؤكد من هذا المسار.")
	case errors.Is(err, merchantassist.ErrInvalidSourceEvidence):
		result = appErrors.New(appErrors.CodeExternalDependency, "بيانات المصدر غير مكتملة أو غير صالحة للاستخدام مع المساعد. تحقق من السجل أو مصدر المعرفة.")
	case errors.Is(err, merchantassist.ErrReadUnavailable):
		result = appErrors.New(appErrors.CodeExternalDependency, "تعذر قراءة البيانات من الخادم حاليًا. أعد المحاولة لاحقًا.")
	case errors.Is(err, merchantassist.ErrAssistantUnavailable):
		result = appErrors.New(appErrors.CodeExternalDependency, "خدمة نموذج مساعد مجيب غير مهيأة أو غير متاحة على الخادم.")
	case errors.Is(err, context.DeadlineExceeded):
		result = appErrors.New(appErrors.CodeExternalDependency, "انتهت مهلة طلب مساعد مجيب. حاول مرة أخرى.")
	default:
		result = appErrors.New(appErrors.CodeExternalDependency, "تعذر توليد إجابة المساعد. تحقق من توفر مزود الذكاء الاصطناعي على الخادم.")
	}
	stage := "provider_or_unknown"
	switch {
	case errors.Is(err, merchantassist.ErrModelRouting):
		stage = "model_planning"
	case errors.Is(err, merchantassist.ErrModelAnswer):
		stage = "model_answer"
	case errors.Is(err, merchantassist.ErrInvalidModelPlan):
		stage = "invalid_model_plan"
	case errors.Is(err, merchantassist.ErrInvalidSourceEvidence):
		stage = "evidence_validation"
	case errors.Is(err, merchantassist.ErrFullScanTooLarge):
		stage = "full_scan_limit"
	case errors.Is(err, merchantassist.ErrReadUnavailable):
		stage = "source_read"
	case errors.Is(err, merchantassist.ErrAssistantUnavailable):
		stage = "not_configured"
	case errors.Is(err, context.DeadlineExceeded):
		stage = "timeout"
	case errors.Is(err, merchantassist.ErrReadDenied):
		stage = "permission"
	}
	// Categorize the wrapped error without logging customer messages, SQL,
	// provider payloads or credentials. HTTP code is carried by StatusError.
	httpStatus := 0
	var providerStatus interface{ ProviderHTTPStatus() int }
	if errors.As(err, &providerStatus) { httpStatus = providerStatus.ProviderHTTPStatus() }
	cause := "unclassified"
	switch {
	case httpStatus > 0: cause = fmt.Sprintf("provider_http_%d", httpStatus)
	case errors.Is(err, context.DeadlineExceeded): cause = "deadline_exceeded"
	case errors.Is(err, context.Canceled): cause = "context_canceled"
	case errors.Is(err, merchantassist.ErrInvalidModelPlan): cause = "tool_contract_validation"
	case errors.Is(err, merchantassist.ErrInvalidSourceEvidence): cause = "invalid_source_evidence"
	case errors.Is(err, merchantassist.ErrAssistantUnavailable): cause = "model_unconfigured"
	case errors.Is(err, merchantassist.ErrReadUnavailable): cause = "source_unavailable"
	}
	log.Printf("[MerchantAssist] request_failed class=%s stage=%s cause=%s provider_http_status=%d",
		result.Code, stage, cause, httpStatus)
	return result
}
