# هيكلة Mujeeb 24 Backend الحالية

هذه الوثيقة فهرس حدود مجلدات Go. **مرجع مسار مساعد كتالوج التاجر الحالي:** فرع `fix/merchant-catalog-strict-wire-v3`، [عقد المقترح](../contracts/ai/12-merchant-catalog-ai-proposal-contract-closed.md) و[عقد القوالب العربية](architecture/merchant-catalog-attribute-schema-ai-contract-ar.md). لا يعني ذلك دمجًا في main أو نشرًا إنتاجيًا. لا تستخدم أوصاف فروع سابقة لتحديد سلوك النقل أو تخزين المواصفات.

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

PostgreSQL مصدر الحقيقة. لا شبكة داخل معاملات قاعدة البيانات، ولا إعادة إرسال تلقائية عند نتيجة مزود مجهولة. نموذج AI يقترح ولا يملك SQL أو الإرسال المباشر. وبالنسبة لمساعد التاجر، النموذج وحده يؤلف `label` العربي و`validation_rules`؛ Go يتحقق ويربط ويولّد `attr_...` ويحفظ بعد اعتماد التاجر.

B2B يستخدم `MerchantCatalogAuthoringAdapter` للاستكشاف وأخذ مقترح `Proposal V3` عبر Strict Wire مع دعم قوالب المواصفات وتعريفاتها. B2C مسار منفصل يعتمد `CustomerSalesDecisionPort`، وأدوات التقييم له مستقلة. لا تخلط عقد Merchant بعقد التقييم أو تستخدم محرك نوايا بالكلمات المفتاحية داخل Go.

## قواعد صيانة الشجرة

لا توجد Chatwoot أو Asynq ضمن الهيكلة الحالية. لا تحفظ ملفات placeholders أو مجلدات فارغة، ولا تنشئ طبقات قبل وجود تنفيذ يستخدمها. إضافة طبقة أو مجلد تحتاج مسؤولية فعلية واستخدامًا واضحًا.

لا تحذف واجهة أو حقلًا أو حالة مخزنة لمجرد وجود اسم قديم؛ تتبع المستدعين وHTTP والتخزين والمهاجرات والاختبارات أولًا. حذف الكود غير المستخدم يتبعه gofmt وبوابة التحقق وOpenAPI drift. تبقى migrations التاريخية لحماية قواعد البيانات القائمة.
