# هيكلة Mujeeb 24 Backend الحالية

هذه الوثيقة تصف التنفيذ الحالي على `feat/universal-catalog-ai-v3`. ليست خطة لإنشاء مجلدات أو خدمات إضافية. لا دمج إلى main أو فرع الإنتاج ضمن أعمال التنظيف.

## حدود المسؤولية

| المسار | المسؤولية |
| --- | --- |
| cmd | نقاط تشغيل API وworker وmigrations والأدوات التشغيلية الموجودة |
| internal/bootstrap | تركيب الاعتماديات ودورة التشغيل |
| internal/application/commands | أوامر الأعمال وعقود نتائجها |
| internal/application/queries | عقود القراءة |
| internal/application/ports | واجهات التخزين والمزودين وعقود السياق |
| internal/application/services | تنسيق التدفقات والتحقق والسياسات |
| internal/application/merchantcatalogai | تنسيق تأليف الكتالوج للتاجر |
| internal/domain | منطق المجال المنفذ فعليًا |
| internal/adapters/primary/http | تسجيل API وDTOs وhandlers وmiddleware |
| internal/adapters/secondary/persistence/postgres | التخزين والمعاملات |
| internal/adapters/secondary/providers/socialapi | نقل القنوات والأحداث والرسائل |
| internal/adapters/secondary/ai | مهايئات مزودي AI الحالية |
| internal/adapters/secondary/realtime | نشر أحداث المحادثات |
| internal/platform | المكونات التقنية المشتركة المنفذة |
| migrations | الترحيلات التراكمية المحفوظة |
| deploy | إعدادات التشغيل الفعلية |
| scripts | أدوات التحقق والتشغيل |
| api/openapi | عقد HTTP المولد من Go |
| contracts وdocs | العقود ومراجع التشغيل والتصميم |

## تدفق التشغيل

SocialAPI webhook → التحقق وتسجيل الحدث → حفظ العميل والمحادثة والرسالة → AutoReply الاختياري عبر بوابة التكلفة وWorker Pool → اقتراح AI → Validation والسياسة → OutboundMessage وOutbox → worker → SocialAPI.

PostgreSQL مصدر الحقيقة. لا شبكة داخل معاملات قاعدة البيانات، ولا إعادة إرسال تلقائية عند نتيجة مزود مجهولة. Gemini يقترح ولا يملك SQL أو الإرسال المباشر.

B2B يستخدم تأليف الكتالوج للتاجر عبر MerchantCatalogAuthoringAdapter. B2C يستخدم CustomerSalesDecisionPort عبر Gemini Interactions مع التقييم الكامل للكتالوج بالدفعات عند الحاجة. Intent وKnowledgeContext وAIDecision تبقى ضمن البنية الحالية.

## قواعد صيانة الشجرة

لا توجد Chatwoot أو Asynq ضمن الهيكلة الحالية. لا تحفظ ملفات placeholders أو مجلدات فارغة، ولا تنشئ طبقات قبل وجود تنفيذ يستخدمها. إضافة طبقة أو مجلد تحتاج مسؤولية فعلية واستخدامًا واضحًا.

لا تحذف واجهة أو حقلًا أو حالة مخزنة لمجرد وجود اسم قديم؛ تتبع المستدعين وHTTP والتخزين والمهاجرات والاختبارات أولًا. حذف الكود غير المستخدم يتبعه gofmt وبوابة التحقق وOpenAPI drift. تبقى migrations التاريخية لحماية قواعد البيانات القائمة.
