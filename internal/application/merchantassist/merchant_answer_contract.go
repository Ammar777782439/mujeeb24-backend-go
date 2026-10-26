package merchantassist

// Merchant answer conventions are owned by Go and shared with DeepSeek as
// verified tool evidence. They are NOT an intent engine or a replacement for
// native function-calling schemas.
const merchantReplyStyleAR = "عقد إجابة مجيب للتاجر: أجب بالعربية الطبيعية المناسبة للسؤال. ابدأ بالنتيجة أو الجواب مباشرة. الرد الافتراضي قصير وواضح (عادة جملتان أو ثلاث)، وإذا طلب التاجر شرحًا أو مقارنة أو تقريرًا مفصلًا فتوسع بقدر الحاجة. استعمل المصطلحات العربية ومعاني الحالات الواردة في نتائج الأدوات؛ لا تعرض أسماء الحالات الإنجليزية أو المعرّفات التقنية إلا إذا طلبها التاجر. لا تعتبر فرصة مكسوبة طلبًا مدفوعًا أو مسلّمًا، ولا تخترع حقائق أو توصيات غير مسندة. إذا غابت المعلومات فقل ما لا تعرفه واسأل سؤال توضيح واحدًا عند الحاجة. لا تنفذ أي كتابة أو تغيير في بيانات المتجر."

// statusDomainGlossary is intentionally domain-specific. A state such as
// "completed" in a transaction is not proof of payment, shipment or delivery.
var statusDomainGlossary = map[string]map[string]string{
	"lead": {
		"new": "جديدة: فرصة بيع مسجلة وتحتاج مراجعة التأهيل",
		"qualified": "مؤهلة: فرصة لديها معلومات تؤهلها للمتابعة",
		"contacted": "تم التواصل: سُجل تواصل مع العميل",
		"negotiating": "قيد التفاوض",
		"proposal": "تم تقديم عرض",
		"won": "مكسوبة: فرصة بيع ناجحة، وليس دليلًا على دفع أو تسليم طلب",
		"lost": "خاسرة: لم تنجح فرصة البيع؛ لا تفترض سبب الخسارة دون دليل",
		"open": "مفتوحة: لم يتم إغلاق الفرصة بعد",
	},
	"conversation": {
		"open": "مفتوحة: محادثة لم تُغلق، ولا تعني وحدها الحاجة إلى تدخل موظف",
		"active": "نشطة",
		"waiting_customer": "بانتظار رد العميل",
		"waiting_human": "بانتظار تدخل موظف",
		"waiting_ai": "بانتظار رد المساعد",
		"closed": "مغلقة",
		"resolved": "تمت معالجتها",
	},
	"transaction": {
		"draft": "مسودة طلب: لم يعتمد بعد",
		"pending": "معلّق: ينتظر إجراءً أو قرارًا",
		"pending_review": "بانتظار مراجعة موظف",
		"awaiting_review": "بانتظار المراجعة",
		"awaiting_confirmation": "بانتظار تأكيد العميل",
		"confirmed": "مؤكد: تم تسجيل التأكيد، وليس إثباتًا مستقلًا للدفع أو التسليم",
		"approved": "تمت الموافقة عليه",
		"rejected": "مرفوض",
		"completed": "مكتمل بحسب حالة المعاملة في النظام، وليس إثباتًا منفصلًا للدفع أو التسليم",
		"cancelled": "ملغى",
		"canceled": "ملغى",
		"failed": "فشل",
		"open": "مفتوح",
	},
	"customer": {
		"active": "نشط بحسب سجل العميل",
		"inactive": "غير نشط بحسب سجل العميل",
		"blocked": "محظور",
		"archived": "مؤرشف",
	},
	"catalog": {
		"active": "نشط",
		"inactive": "غير نشط",
		"draft": "مسودة",
		"published": "منشور",
		"archived": "مؤرشف",
	},
	"channel": {
		"active": "متصلة ونشطة",
		"connected": "متصلة",
		"disconnected": "غير متصلة",
		"pending": "قيد الربط",
		"connecting": "جارٍ الاتصال",
		"error": "يوجد خلل في الاتصال",
		"failed": "فشل الاتصال",
	},
	"subscription": {
		"active": "اشتراك نشط",
		"trialing": "فترة تجريبية",
		"past_due": "دفعة متأخرة",
		"expired": "منتهي",
		"canceled": "ملغى",
		"cancelled": "ملغى",
		"pending": "قيد التفعيل",
	},
}

var toolStatusDomains = map[string][]string{
	"find_customers": {"customer"},
	"customer_record": {"customer"},
	"customer_activity": {"customer", "conversation", "transaction"},
	"list_conversations": {"conversation"},
	"conversation_history": {"conversation"},
	"list_leads": {"lead"},
	"lead_evidence": {"lead"},
	"list_transactions": {"transaction"},
	"transaction_record": {"transaction"},
	"transaction_customer_details": {"transaction", "customer"},
	"list_catalogs": {"catalog"},
	"list_channels": {"channel"},
	"subscription": {"subscription"},
}

// decorateMerchantToolEvidence gives DeepSeek authoritative Arabic semantics
// with the same permission-filtered read result. Raw machine status values stay
// untouched for programmatic consumers and for verification.
func decorateMerchantToolEvidence(tool string, data any) any {
	domains := toolStatusDomains[tool]
	if len(domains) == 0 {
		return data
	}
	labels := make(map[string]map[string]string, len(domains))
	for _, domain := range domains {
		source := statusDomainGlossary[domain]
		terms := make(map[string]string, len(source))
		for state, meaning := range source {
			terms[state] = meaning
		}
		labels[domain] = terms
	}
	if evidence, ok := data.(map[string]any); ok {
		extended := make(map[string]any, len(evidence)+1)
		for key, value := range evidence {
			extended[key] = value
		}
		extended["status_meanings_ar"] = labels
		return extended
	}
	return map[string]any{"data": data, "status_meanings_ar": labels}
}
