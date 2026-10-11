# تعديل عقد Customer Sales — قدرات Gemini v12

التاريخ: 2026-10-11. الفرع: `feat/universal-catalog-ai-v3`.

هذا العقد يثبت حدود التشغيل الدلالية لقدرات Customer Sales، ويتقدم على وصف Customer Sales القديم في العقد ④ والـPrompt السابق: لا يصنّف مجيب نية العميل ولا يربطها بإجراء؛ Gemini يفهم الرسالة والسياق ويختار قدرة واحدة عبر Function Calling.

## القدرات المعروضة

| القدرة | التنفيذ المتوفر |
|---|---|
| `respond_without_action` | رد معلوماتي/توضيحي/اجتماعي فقط؛ لا يبدأ ولا يؤكد ولا يكمل أي عملية خارجية |
| `ask_customer` | سؤال Gemini عن معلومات ناقصة فعلًا من العميل |
| `handoff_to_human` | نص Gemini + إجراء human_request + حالة waiting_human، بعد السماح بالرد |
| `evaluate_full_catalog` | تقييم كل الكتالوج النشط للتاجر عبر دفعات التوكن؛ متاحة في مرحلة الفهم عند تركيب مسار الكتالوج |

لا تعرض `create_lead_draft` أو `create_order_draft`: مسار B2C السابق يحفظ اقتراحًا ويطلب مراجعة بشرية، ولا ينفذ إنشاء مسودة. هذه القدرات تضاف عند وجود منفّذ فعلي، ولا يعتمد عرضها على قطاع التاجر.

## حدود الفهم والمعرفة

- السياق الأول يتضمن الرسالة، السجل القريب، الملخص، الحالة، بيانات المنشأة والسياسات والـManifest والأدلة الحالية الموثوقة من التركيز السابق.
- الـManifest يشرح البنية فقط؛ لا يثبت وجود عنصر أو سعر أو توفر.
- إذا احتاج Gemini بيانات إضافية يطلب `evaluate_full_catalog` بلا query أو filters أو IDs أو business_id.
- مجيب يستخدم نطاق التاجر الموثوق ويقرأ صفحات الكتالوج الكاملة بعلاقاتها، ثم يقسمها حسب التوكن. Gemini يقيّم جميع الدفعات ويختار المرشحين.
- فشل دفعة أو تغير revision أثناء القراءة يمنع التقييم النهائي والإرسال. بعد التغطية الكاملة، التقييم النهائي يعرض القدرات النهائية فقط؛ لا إعادة دخول أو loop مفتوح.
- المراجع المختلقة تفشل التحقق؛ لا تؤدي إلى تشغيل الكتالوج أو توسيع نطاق الأدلة.
- لا بحث ولا مطابقة منتجات ولا keywords ولا شروط قطاعية داخل Go.
- `respond_without_action` لا يجوز استخدامها لصنع workflow تأكيد لإجراء غير منفذ. إذا كان هدف العميل هو المضي في عملية خارجية ولا توجد قدرة تنفيذية معروضة لها، يختار Gemini `handoff_to_human` مباشرة.
- الرد المختصر مثل «نعم» أو «تمام» يُفهم من سياق المحادثة؛ إذا كان يؤكد عملية خارجية غير مدعومة، يبقى المسار `handoff_to_human` ولا يتحول إلى ادعاء تنفيذ.
- لا يجوز لأي قدرة رسائلية الادعاء بأن عملية خارجية نُفذت أو تأكدت أو اكتملت ما لم توجد نتيجة تنفيذ موثوقة في السياق تثبت ذلك.

## العقد والتنفيذ

Function Calling يستخدم تعريفات ذات schema مغلق و`tool_choice=any`. يجب أن يعيد النموذج استدعاءً واحدًا معرفًا لأداة معروضة. يُرفض النص المنفرد، والاستدعاءات المتعددة، والأدوات غير المتاحة، والحقول الإضافية، والنص الفارغ. الفهم والرد من Gemini؛ المراجع والسياسات والتفويض والتنفيذ من مجيب.

يبقى `CapabilityDecision` مصدر الحقيقة في المهايئ والتقييم النهائي وValidation وPolicy وAuthorization وExecution. Registry موحد يعرف القدرات المنفذة وخصائصها التشغيلية. التحويل إلى `answer/clarification/human_request` يحدث في حد التوافق للحفظ وAPI فقط؛ لا يقود runtime. لا يطلب العقد الجديد `status/action/routing_reason` من Gemini. لم تعد subscription_activation تتحكم بالتنفيذ، وحُذفت الرسالة الثابتة واستبدال human_request بـanswer.

رسالة handoff تخضع لـallow_auto_reply والمراجعة البشرية كأي رسالة للعميل. التفويض الناجح يسجل `VALIDATING → AUTHORIZED → EXECUTING`؛ فشل الانتقال يمنع الإرسال. AUTHORIZED وEXECUTING داخل معاملة القرار وOutbox نفسها، بلا اتصال شبكة داخلها.

قرارات القدرات stateless (`store=false`) وتعتمد على سجل مجيب. لا نربط الدور التالي باستدعاء دالة غير مكتمل لدى المزود، ولا نضيف طلب LLM لمجرد إغلاق ذلك الاستدعاء. تبقى حقول Gemini continuity للتوافق مع سجلات سابقة؛ يُمسح المعرّف السابق في المسار الجديد. اختبارات B2B ومساره مستقلان.

عداد ToolCalls يسجل اختيارًا واحدًا في المسار المباشر، وطلب الكتالوج مع الاختيار النهائي في المسار الكامل. تبقى tokens واستخدام كل دفعة والتقييم النهائي على مسار التسجيل الموجود.

## التحقق

اختبارات محلية تغطي عقود الأدوات، حقول النطاق والتفويض غير المسموحة، الخيارات المتعددة، التوكن وتعريفات الدوال، كتالوجًا على صفحتين وأربع دفعات بكل علاقاته، اختيار العنصر الأخير، فشل دفعة، تغير revision، التحية دون قراءة الكتالوج، المراجع غير المعروضة، منع الإرسال دون انتقال التفويض، وحفظ نص وحالة التحويل للبشر.

أُصلح اختبار OpenAPI سابق يستخدم lookahead غير مدعوم في Go ولا يتعامل مع اقتباس wildcard في YAML. تحقق الصلاحيات ما زال إلزاميًا، وملفات DTOs خضعت لتنسيق gofmt فقط دون تغيير API.

التقييمات الدلالية الحية منفصلة ومقيدة بـ`gemini_live` و`MUJEEB_CAPABILITY_LIVE_EVALS=1`. تشمل لهجات وأخطاء إملائية وقطاعات مختلفة، وتفحص اختيار القدرة دون تطابق حرفي لنص الرد. تشغيلها يحتاج تصريحًا حيًا وGEMINI_API_KEY وGEMINI_MODEL في البيئة، ولا تنفذ ضمن الاختبارات المحلية أو push.

تصحيح بقاء القدرات يضيف migration أمامية `000073_customer_sales_capability_audit.up.sql`: requested_capability/effective_capability اختياريان للسجلات التاريخية وB2B؛ لا تخمين أو backfill لأسماء قديمة. يتيح intent_base فارغًا لقرارات القدرات ويتيح action فارغًا فقط للقدرات المرفوضة دون mapping. DTOs تعرض حقلي القدرة، وOpenAPI يولد من الكود. لا أسرار. نجاح الاختبارات المحلية لا يثبت صحة الاختيارات الدلالية مع النموذج الحي أو تكامل PostgreSQL دون DSN.

المراجع: [Gemini function calling](https://ai.google.dev/gemini-api/docs/function-calling)، [Interactions API](https://ai.google.dev/api/interactions-api).

## نتيجة التحقق المحلي لهذه الدفعة

- `go test ./...`: نجح.
- `go vet ./...`: نجح.
- `go test -race ./...`: نجح.
- بوابة `scripts/verify-repository.sh` وتوليد OpenAPI دون drift: نجحت.
- بناء تقييمات `gemini_live` مع تعطيل opt-in: نجح البناء، وتخطّي التشغيل الحي مسجل بوضوح.
- تكامل PostgreSQL: لم يُشغّل لعدم وجود `POSTGRES_TEST_DSN`.
- أدوات codebase-memory-mcp وgopls-mcp-server غير متاحة في هذه الجلسة؛ التحقق هنا من قراءة المصدر واختبارات Go وvet.

دور جديد أثناء `waiting_human` ينتهي بـ`CANCELLED` قبل استدعاء النموذج؛ لا يبقى Run في RECEIVED بسبب محاولة انتقال غير مسموح إلى COMPLETED. هذا لا يغيّر حالة المحادثة أو يعيد تشغيل الرد الآلي.

## تصحيح Source of Truth — 2026-10-11

- أول اختيار والتقييم النهائي يعيدان CapabilityDecision نفسه؛ لا تحويل إلى Proposal أو status/action/routing_reason في Gemini.
- BatchClient لا يمرر القرار عبر JSON legacy. FinalEvaluation لا يعرض retrieval capability ولا يقبل قدرة غير نهائية.
- Registry مشترك للعرض والتحقق ومتطلبات سياسة رسالة العميل وآثار المنفذ. لا switch على legacy action داخل AutoReply أو PolicyEvaluator.
- RequestedAction/EffectiveAction/AutoReplyResult.Action تبقى للتوافق فقط. RequestedCapability/EffectiveCapability وAutoReplyResult.Capability وrealtime capability تحمل الاسم الحقيقي.
- اللوج يسجل capability بلا تسجيل نص رد العميل في GEMINI_OK. IntentBase فارغ. السياسة والتوقيت وحالة Run منفصلة عن القدرة المختارة، حتى إذا منع التنفيذ.
- Ready يفحص Catalog repo وGemini وRunRepo وclock/IDs وtoken counter قبل عرض retrieval، ومعه service RunRepo ومعرف Run وEntity Contract صالح. هذه جاهزية تركيب محلية، لا ضمان توفر المزود لاحقًا.
- عقدا ④ و⑥ الحاليان يوجهان التطوير؛ الوصف السابق نقل إلى archive وموسوم HISTORICAL/SUPERSEDED. تعليمات AGENTS تشير للقرار الحالي.

## تحقق تصحيح بقاء القدرة

نجحت بوابة المستودع (go test ./... وgo vet وOpenAPI drift) واختبار السباق لكل المستودع، ثم إعادة السباق لحزمة services بعد إضافة اختبارات التفويض والسياسة. الاختبارات الجديدة تتبع القدرات النهائية الثلاث بعد أربع دفعات عبر policy/execution/audit، وتمنع التنفيذ عند denied/requires_approval، وتمنع تبديل القدرة أو تجاوز policy في Authorization.

تم بناء اختبارات PostgreSQL integration وGemini live. التشغيل الحي متخطى: POSTGRES_TEST_DSN غير موجود، وGemini opt-in معطل. اختبار PostgreSQL الجديد يفحص Get/List round-trip لحقلي القدرة وIntentBase الفارغ، لكنه لا يُعد ناجحًا قبل تشغيله على قاعدة فعلية. لا ينسب هذا التقرير نجاح CI إلى GitHub دون run مستقل.

يجب تطبيق migration 000073 قبل تشغيل إصدار الكود الجديد. لا تشغيل migration على بيئة التاجر ضمن تعديل المصدر، ولا دمج إلى main.

اختبار PostgreSQL الأول على GitHub كشف أن CreateProposed ما زال يشترط intent_base غير فارغ. حُدّث حد الحفظ ليقبل CapabilityDecision دون intent، ويقبل سجل expired للقدرة المرفوضة مع denied فقط، ويحافظ على شروط legacy. اختبارات boundary ترفض تبديل effective_capability، وSQL integration يفحص حفظ القدرة الناجحة والمرفوضة مع rollback الإرسال.
