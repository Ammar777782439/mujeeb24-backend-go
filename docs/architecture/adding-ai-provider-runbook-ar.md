# الدليل الهندسي لإضافة نموذج أو مزود AI جديد إلى مجيب 24

> **حالة المستند:** دليل تنفيذ ومراجعة لمطور Go، وليس إثباتًا بأن مزودًا جديدًا يعمل تلقائيًا.
>
> **مرجع الكود عند إعداد الدليل:** مستودع `Ammar777782439/mujeeb24-backend-go`، فرع `refactor/provider-agnostic-ai-runtime`، Commit `c635dc5cd4d8ed9cbc9871319fd8241b518dd403`، بتاريخ 2026-10-24.
>
> **النطاق:** ثلاثة استخدامات لنموذج واحد أو مزود واحد: (1) ردود العملاء واتخاذ قرارات القدرات Customer Sales، (2) تقييم كتالوج العميل Catalog Batch/Final Evaluation، (3) مساعد التاجر لإدارة الكتالوج Merchant Catalog Authoring. التلخيص النصي تابع لمسار العميل ويُختبر بصورة مستقلة.
>
> **خارج النطاق عمدًا:** محاسبة التوكنات والأسعار والفواتير، بناء Provider Registry عام، التحويل الديناميكي من لوحة التاجر، تعديل قاعدة البيانات أو ترحيلاتها، إعادة تصميم السياسات والصلاحيات والمبيعات، ودمج الفروع. التكامل الجديد لا يبرر تغيير هذه الأجزاء.

## 0. القرار الهندسي الذي لا نتجاوزه

**عند إضافة مزود جديد، نقرأ وثائقه ونكتب تكاملًا خاصًا به يفهم الطلبات الثلاثة لمجيب ويترجم طلباتها واستجاباتها إلى العقد الموجود.** لا نجعل `Customer AI` أو `Merchant AI` يفهمان API شركة خارجية. **النموذج هو الذي يختار استدعاء الأدوات** التي يعرضها مجيب؛ Go يحدد فقط القدرات المصرح بها وينفذها ويتحقق من النتائج. لا نكتب `if` لكلمات «السعر»، «الشراء»، «هاتف» لاختيار `evaluate_full_catalog`.

- ابدأ بأصغر تغيير يمكّن مزودًا مؤهلًا واحدًا. لا تضف واجهات أو مصانع أو طبقات عامة بلا حاجة مثبتة.
- يمكن إبقاء عقد Chat Completions الداخلي الحالي إذا ترجم Adapter المزود الجديد المعاني كاملة. **ليس شرطًا إعادة كتابة كل العقود لتصبح مستقلة شكليًا**.
- إذا ثبت أن بروتوكول المزود يحتاج معلومة لا يمكن تمريرها بالعقد الحالي (مثلاً حالة continuation لا يمكن تمثيلها بأمان)، وثّق الفجوة وأضف أصغر امتداد متوافق مع الاختبارات؛ لا تخترع بديلًا ولا تتجاهل حالة النموذج.
- Adapter **لا** يفتح اتصال PostgreSQL، ولا يستعلم عن تاجر، ولا يقرر منتجًا أو صفقة، ولا ينفذ أداة، ولا يعتمد اقتراح التاجر، ولا يرسل رسالة إلى العميل.
- لا تدع الاسم التسويقي `Pro` أو `Flash` يقود التنفيذ. اختر **معرّف النموذج المقبول في API وقت التنفيذ**، ثم أثبت قدراته باختبارات.

### تمييز لا بد منه: تغيير نموذج أم إضافة مزود؟

| الحالة | العمل الأدنى |
| --- | --- |
| **نموذج آخر عند مزود DeepSeek نفسه** | تحقق من معرّفه في الوثائق، ومن توافق tool calling وJSON Schema والقيود، ثم غيّر `DEEPSEEK_MODEL` واختبر جميع الاستخدامات الثلاثة. قد يحتاج Adapter تعديلًا محدودًا **إذا اختلف السلوك**. |
| **مزود جديد يشترك في بروتوكول Chat Completions** | اكتب Adapter فعليًا أو تنفيذًا مُراجعًا خاصًا به، حتى لو أعاد استخدام بعض دوال الترجمة؛ لا تفترض التطابق في أدوات التفكير، أو المصادقة، أو response format. اربطه في التكوين. |
| **مزود ببروتوكول مختلف مثل `functionCall` أو جلسات خاصة** | اكتب Adapter يفك ويمرر كل المعاني ويرجع صيغة مجيب الحالية؛ اختبر جولات Merchant الحقيقية. لا تلمس منطق الأعمال إلا إذا وجدت حدًا ناقصًا مثبتًا. |
| **نموذج لا يدعم قدرات أساسية للمسار** | **غير مؤهل لهذا المسار**؛ فشل واضح وآمن. لا تستبدل الأدوات بتحليل كلمات أو JSON تخميني، ولا تدّعِ التوافق. |

## 1. خريطة الكود الحقيقية: اقرأ هذه الملفات بهذا الترتيب

1. `internal/adapters/secondary/ai/native/router.go` — العقد `Request/Response/Provider`، `Router`، tracing، التثبيت على مزود الجولة، والعد الاختياري للتوكنات.
2. `internal/adapters/secondary/ai/native/completion.go` — `CompletionClient.Complete` و`CompletePinned` و`CountTokens`؛ يعيد `json.RawMessage` بصيغة Canonical Chat Completions الحالية.
3. `internal/adapters/secondary/ai/native/deepseek.go` و`deepseek_schema.go` — **مثال تنفيذ مزود حقيقي، وليس بروتوكولًا ملزمًا لكل مزود**؛ راجع تحويل الأدوات، إرسال HTTP، التحقق والتطبيع.
4. `internal/adapters/secondary/ai/gateway/interaction_contract.go` — `chatCompletionRequest`، `capabilityChatRequest`، وفحص schema/arguments الصارم.
5. `internal/adapters/secondary/ai/gateway/native_response.go` — محلل `choices[].message.tool_calls` و`finish_reason` وJSON.
6. `internal/adapters/secondary/ai/gateway/native_customer_adapter.go` — `Decide` و`Summarize`.
7. `internal/adapters/secondary/ai/gateway/customer_sales_capabilities.go` و`internal/application/ports/customer_sales_capabilities.go` — **الأدوات الفعلية** ووقت إتاحتها.
8. `internal/adapters/secondary/ai/gateway/catalog_request_builder.go` و`native_catalog_adapter.go` — طلبات دفعات الكتالوج وإرجاع قرار نهائي من النموذج.
9. `internal/application/services/catalog_batch_controller.go` — تقسيم الدفعات والتحقق من التغطية والمراجع/الإصدار؛ **ليس مسؤولية Adapter المزود**.
10. `internal/adapters/secondary/ai/gateway/native_merchant_catalog_adapter.go` و`merchant_catalog_schema.go` و`merchant_catalog_prompt.go` — دورة أدوات التاجر وجولات الاستكشاف ثم الاقتراح المنظم.
11. `internal/application/merchantcatalogai/agent.go` و`ports.go` و`discovery_registry.go` و`execution.go` — نطاق التاجر، صلاحيات القراءة، صحة الاقتراح، المراجعة والتنفيذ.
12. `internal/bootstrap/integrations.go` و`internal/bootstrap/merchant_catalog_ai.go` و`internal/platform/config/config.go` — توصيل التنفيذ بإعدادات التشغيل، دون نسخ المنطق.
13. `.env.example` و`deploy/local/.env.example` و`render.yaml` — أمثلة الإعداد والنشر.
14. `internal/adapters/secondary/ai/native/*_test.go` و`internal/adapters/secondary/ai/gateway/*_test.go` و`.github/workflows/native-ai-verify.yml` — اختبارات الوحدة والعقود والتكامل.

> لا تستخدم اسم `mujeeb-sales` أو `mujeeb-catalog` أو `mujeeb-merchant` على أنه معرّف API لثلاثة نماذج مختلفة. هذه أسماء طلب داخلية في `bootstrap`، و`DeepSeek.Generate` يرسل `c.model` الحقيقي حاليًا. عند بناء مزود جديد حدّد صراحةً كيف يتعامل مع `request.model` مقارنة بالنموذج المضبوط؛ لا تُصدر اسمًا داخليًا غير معروف إلى API بالخطأ.

## 2. عقد Go الموجود — لا تُنشئ طبقة جديدة افتراضيًا

الكود الفعلي في `native/router.go`:

```go
type Request struct {
    Payload        json.RawMessage
    Operation      string
    PinnedProvider string
}
type Response struct {
    Payload         json.RawMessage
    Model           string
    InputTokens     int64
    OutputTokens    int64
    CachedTokens    int64
    ProviderCostUSD float64
    CostKnown       bool
}
type Provider interface {
    Name() string
    Generate(context.Context, Request) (Response, error)
}
```

الـ`Request.Payload` **ليس رسالة العميل مباشرة**؛ إنه JSON مكتمل لمحادثة أو استدعاء أدوات أنتجته طبقة Gateway. `Response.Payload` ليس JSON الخام العشوائي للمزود؛ يجب أن يكون JSON الذي تفهمه طبقة Gateway **في الصيغة الداخلية الحالية**. `Name()` هو هوية التكامل، وليس اختيار النموذج بواسطة العميل.

يمكن للمزود أن يدعم اختياريًا `CountTokens(context.Context, Request) (int64, error)`؛ يستعمله `Router.CountTokens` عندما لا يوجد Standby. إن لم يدعمه فلا تخترع عدادًا دقيقًا؛ يعتمد `NativeCatalogAdapter` تقديرًا محافظًا بحجم الطلب؛ افحص ميزانية السياق ورفض الطلبات الكبيرة قبل الإرسال.

### السلسلة المشتركة

`Gateway request → CompletionClient → Router → Provider.Generate → Provider API → canonical Response → Gateway validation → application decision/Proposal`

يملك `Router` خيار مزود احتياطي لكنه معطّل في تركيب الإنتاج الحالي `NewRouter(provider, nil, native.Policy{})`. **إضافة مزود جديد لا تعني تشغيل Fallback تلقائيًا**. إن طُلِب التحويل لاحقًا فله قرار واختبارات مستقلة، بما فيها حفظ تاريخ الأدوات وعدم تبديل المزود وسط الجولة دون دعم صحيح.

### ما الذي يُسمح للـAdapter بفعله؟

- التحقق من صلاحية الإعدادات والمفتاح، وبناء HTTP Client أو SDK، ومعايرة المهل.
- تفكيك الطلب الداخلي، وتحويل أدوار الرسائل، وقواعد `tool_choice`، وتعريفات الأدوات والـSchemas، وسجل assistant/tool والردود المنظمة.
- ترجمة `functionCall` / `tool_calls` / أرقام الاستدعاءات / محتوى الأدوات ونهاية الاستجابة إلى الشكل المتوقع **دون تغيير اسم قدرة مجيب أو حججها التجارية**.
- التعامل مع متطلبات المزود الخاصة بالتفكير وسياق الجولات وتوقيعاته، متى كانت ضرورية وموثقة، مع الحفاظ على الترابط الصحيح بين استدعاء الأداة ونتيجتها.
- رفض حالة غير مدعومة قبل الإرسال أو بعدها؛ وعدم تحويل رفض النموذج إلى نجاح صوري.
- إرجاع `StatusError` للمواقف المناسبة مع كود HTTP بدون كشف محتوى حساس، وتطبيق `context.Context` على HTTP.
- إرجاع payload منظّم فعليًا، لا تمثيلًا متخيلًا لاستدعاء لم يختره النموذج.

### ما الذي لا يُسمح له بفعله؟

- لا يستنتج نية عميل أو تاجر من كلمات الرسالة؛ لا يختار `evaluate_full_catalog` أو أداة استكشاف من Go.
- لا يقرأ كتالوج أو سعرًا أو مخزونًا من DB؛ لا ينشئ Lead أو Draft أو Offer.
- لا يختصر evidence صامتًا إذا تجاوز حد الـContext؛ لا يغيّر مرجع عنصر أو `variant_id` أو `offer_id`.
- لا يتجاوز الموافقة البشرية أو صلاحيات التاجر، ولا يعالج أخطاء المزود بإرجاع «تم التنفيذ» دون نتيجة موثوقة.
- لا يدوّن المفتاح أو raw prompt أو الرد الخام في الـlogs العادية. راجع مسارات debug قبل تفعيلها.

## 3. فحص رسمي قبل كتابة سطر تكامل: بطاقة تأهيل المزود/النموذج

أنشئ في PR مذكرة صغيرة من صفحة أو جدول لكل معرّف نموذج، بروابط وثائقه **المؤرخة**، وأجب بدليل وليس بتخمين:

| سؤال يجب الإجابة عنه | لماذا يؤثر في مجيب؟ |
| --- | --- |
| ما اسم نموذج API الفعلي وإصداره وendpoint ومنطقة الخدمة؟ | الاسم التسويقي لا يضمن أن API يقبل المعرّف. |
| ما طريقة المصادقة والحدود والصلاحيات؟ | المفتاح لا يصل للمتصفح أو للعميل. |
| هل يدعم رسائل System/User واستكمال الحوار؟ | يحتاج Customer وMerchant إلى عزل التعليمات عن مدخلات المستخدم. |
| هل يدعم Function/Tool Calling؟ وهل يدعم `required` و`auto`؟ | Customer يستخدم `required`، وMerchant discovery يستخدم `auto`. |
| هل يدعم JSON Schema صارمًا؟ ما الكلمات/الأنواع غير المدعومة؟ | قرارات العملاء، مرشحو الكتالوج والاقتراح النهائي تتطلب بيانات منظمة. |
| هل يدعم جولات أدوات متعددة وربط request ID بنتيجة الأداة؟ | Merchant يعيد `assistant tool_calls` ثم `tool` ويكرر الاستكشاف. |
| هل يحتاج `reasoning_content` أو `thought_signature` أو session state؟ | ضياع تاريخ التفكير قد يسبب 4xx أو فقد السياق. حافظ عليها بالطريقة التي تفرضها الوثائق. |
| هل يستطيع إخراج النص المنظم منفصلًا عن tools عند إنهاء Merchant؟ | الكود الحالي يفصل مرحلة الاستكشاف عن مرحلة الاقتراح النهائي. |
| هل يسمح بطلب `tools` و`response_format` معًا؟ | الكود الحالي لا يجمعهما في Merchant؛ لا تفترض الدعم. |
| هل يدعم `CountTokens` دقيقًا؟ وما نافذة السياق والحد الأقصى للإخراج؟ | تقسيم الكتالوج يعتمد على حد متحقق، وإلا يقدر محافظًا. |
| ما سلوك `finish_reason`، الأخطاء، `429/503`، المهلة، رفض المحتوى، حدود حجم الرد؟ | لا تقبل نتيجة مقطوعة أو Tool Call غير مكتمل. |
| هل يدعم Non-Streaming؟ | الطلبات الحالية ترسل `stream=false`. |
| ما سياسة الاحتفاظ بالبيانات؟ وأي معلومات سترسل إلى المزود؟ | بيانات التاجر والعميل حساسة؛ لا توسع payload بلا ضرورة. |

**لا تعتمد أي نموذج لمجرد أنه يظهر في قائمة الأسعار.** وثائق DeepSeek مثلًا تفرق بين Non-Thinking / Thinking في `tool_choice=required`، وتفرض Strict Mode عبر `/beta` وSchemas محدودة. يجب تأهيل النموذج المختار تحديدًا؛ لا يُسقط ذلك على مزود آخر.

## 4. خطوات تنفيذ Adapter جديد، بالترتيب

### 4.1 سجل الحالة الحالية وحافظ على نطاق العمل

```bash
git fetch origin
git switch refactor/provider-agnostic-ai-runtime
git pull --ff-only origin refactor/provider-agnostic-ai-runtime
git status --short
git log -1 --oneline
```

هذه أوامر للمطور في نسخة العمل؛ **لا تعني أن هذه الأوامر نُفذت عند كتابة المستند**. لا تعمل على `main`، ولا تدمج `feat/gemini-deepseek`، ولا تعدّل فرع المصدر دون أمر صريح.

### 4.2 أضف تنفيذًا ملموسًا داخل `native/`

ملف مقترح عند اختيار مزود فعلي: `internal/adapters/secondary/ai/native/<provider>.go`، ومعه `<provider>_test.go`. إن احتاج تحويل Schema كبيرًا، ضعه في ملف مجاور **فقط عند وجود مبرر حقيقي**.

مثال **هيكل توضيحي غير قابل للتجميع وحده** (أسماء `decode...` و`encode...` ليست دوال موجودة):

```go
type ExampleProvider struct {
    client   *http.Client
    endpoint string
    apiKey   string
    model    string
}
func (p *ExampleProvider) Name() string { return "example" }
func (p *ExampleProvider) Generate(ctx context.Context, req native.Request) (native.Response, error) {
    canonical, err := decodeMujeebRequest(req.Payload)
    if err != nil { return native.Response{}, err }

    wireBody, err := translateToExampleProtocol(canonical, p.model)
    if err != nil { return native.Response{}, err }

    // Construct HTTP request with ctx, correct auth, and bounded response body.
    vendorReply, err := callExampleAPI(ctx, p.client, p.endpoint, p.apiKey, wireBody)
    if err != nil { return native.Response{}, err }

    // Preserve model-selected tool names/arguments/history and completion reason.
    return translateToMujeebResponse(vendorReply)
}
```

في التنفيذ الحقيقي داخل `package native` تستخدم الأنواع مباشرة `Request/Response` لا `native.Request`؛ المثال يشرح **مسؤوليات الترجمة فقط** ولا يدعي وجود دوال مساعدة جاهزة.

ملاحظة دقيقة: `canonical.Request.Operation` موجود لكنه لا يعني أن كل المسارات تضبطه. **لا تعتمد على Operation لتفريع Customer/Merchant دون التحقق من مواضع تعبئته**؛ استعمل محتوى الطلب المدعوم أصلًا. لا تُدخل استنتاجًا للنوايا في Adapter.

### 4.3 حوّل طلبات العملاء

الكود `native_customer_adapter.go` يبني `capabilityChatRequest`: رسائل `system/user`، قائمة `customerSalesTools` بتعريفات Strict، `tool_choice=required`، `parallel_tool_calls=false`، `max_tokens`، `stream=false`.

يلزم الـAdapter الجديد أن:
1. يرسل system والرسالة بسياقهما الصحيح، لا يدمج تعليمات العميل بنص system موثوق.
2. يعرّف **كل القدرات المعروضة فقط**، ويحافظ على أسماء الأدوات وحججها والـSchema.
3. يجعل النموذج يختار **واحدة من القدرات**؛ `required` لا يعني إجبار `evaluate_full_catalog` تحديدًا. يوجد `respond_without_action` كأداة نهائية.
4. يرجع `finish_reason=tool_calls` واستدعاء واحدًا صالحًا، مع `function.name` و`function.arguments` JSON صالح وID، بصيغة Gateway الحالية. لا يُدخل نصًا حرًا مختلطًا إذا اختار أداة.
5. يترك `parseCustomerSalesCapability` والتحقق/الترخيص لمنطق مجيب.

القدرات موجودة وتُفتح حسب أهلية السياق، ومنها `respond_without_action` و`ask_customer` و`handoff_to_human` و`evaluate_full_catalog`، وقدرات الفرص/مسودات المعاملات عند تفعيل Sales. **لا تستخدم نمط keyword/intent classifier داخل Go**.

### 4.4 حوّل طلبات تقييم الكتالوج

مسار `CatalogBatchController` يقرأ الكتالوج ويقسّمه قبل دعوة `NativeCatalogAdapter.EvaluateBatch`. هذا الـGateway يرسل Schema للمرشحين `candidates[]` ويقبل عناصر `item_id, variant_ids, offer_ids, reason`، ويفحص IDs مقابل الدفعة. بعض النماذج تحتاج تحويل الـStructured Output إلى أداة تركيبية (كما يفعل DeepSeek مع `submit_catalog_batch`)؛ **هذا التحويل تقني، وليس قرارًا تجاريًا**.

يلزم الـAdapter:
1. ترجمة `response_format/json_schema` إلى وسيلة مضمونة في API إن كانت مدعومة.
2. إعادة النتيجة إلى `choices[0].message.content` على هيئة **نص JSON يطابق Schema**، مع `finish_reason=stop` (أو إرجاع canonical مكافئ تتعرف عليه الطبقة الحالية).
3. عدم اختراع مرشحين أو تقليص الكتالوج بصمت.
4. دعم طلب `FinalEvaluateWithDetails` الذي يعيد **قدرة نهائية** عبر tools بعد اكتمال التغطية؛ لا تسمح بإرجاع `evaluate_full_catalog` كتكرار غير نهائي في هذه المرحلة.
5. مراعاة `CountTokens` الاختياري والحد المحافظ: `NATIVE_AI_MAX_CONTEXT_TOKENS` و`NATIVE_AI_MAX_OUTPUT_TOKENS`. تأهيل هذه القيم **مرتبط بالنموذج** ولا يُستنتج من اسم الشركة.

### 4.5 حوّل جولات Merchant Catalog AI كاملة — **أهم اختبار**

المسار الموجود هو `NativeMerchantCatalogAuthoringAdapter.Propose` فوق `merchantcatalogai.Agent`. يختار Agent الكتالوج المستهدف ويوثق هوية التاجر وصلاحياته؛ النموذج **يختار أدوات القراءة** من `input.Capabilities.Definitions()`، ومجيب ينفذها من `input.Capabilities.Execute(...)`.

دورة واحدة ممكنة:

```text
Merchant message + selected catalog + history + entity contract
  → Gateway يعرض read-only discovery tools (tool_choice=auto)
  → Provider Adapter يترجم request
  → Model يختار read tool ويرجع tool_call id/name/arguments
  → Gateway يفحص الأداة والوسائط والصلاحية ولا ينفذ أي mutation
  → Merchant discovery executor ينفذ القراءة ويعيد data/has_more/next_cursor
  → Gateway يضيف assistant tool call + tool result إلى history
  → Provider Adapter يترجم الجولة التالية ويحافظ على ربط الـcall والنتيجة
  → Model قد يختار أداة قراءة أخرى
  → عند انتهاء القراءة: Gateway يلغي tools وينتقل إلى طلب proposal منظم مستقل
  → Model ينتج canonical_proposal_json
  → parseMerchantCatalogProposal → Normalize → ValidateReferences → Validate
  → Proposal فقط؛ الحفظ/التنفيذ لهما مسار اعتماد وصلاحيات منفصل
```

تفاصيل عقد Merchant الجاري والتي **لا يصح كسرها**:
- **أداة واحدة لكل جولة** في Gateway الحالي، و`nativeMaxMerchantToolRounds = 12` و`nativeMaxMerchantToolCalls = 32`. لا تفعّل parallel calls بلا تغيير مدروس واختبارات.
- `tool_choice=auto` أثناء الاستكشاف، و`response_format=nil` في تلك الجولات. بعدها تُزال الأدوات ويُطلب Structured Output.
- تتابع الرسائل: `system → user → assistant(tool_calls) → tool(tool_call_id, content) → ...`، وربط ID أداة القراءة بنتيجتها **ضروري**.
- نتيجة القراءة تحمل `data` و`has_more` و`next_cursor`. لا تسقط علامة الصفحات؛ التاجر قد يحتاج بقية العناصر.
- لا يقرر Adapter هل «التاجر يقصد تعديل السعر»؛ النموذج يحدد ما يحتاجه من قراءات ومقترح، ضمن أدوات وصلاحيات مجيب.
- **النص النهائي ليس تنفيذًا**: `canonical_proposal_json` يحتوي اقتراحًا مطابقًا لـ`merchantCatalogProposalSchema()`، ويتم فحصه في Go. يوجد حقول ديناميكية لا يمكن إرسالها مباشرة ضمن Strict Schema لكل مزود؛ الغلاف النصي الحالي حل نقل لا يلغي التحقق.
- الحالة التي تطلب من التاجر معلومات ناقصة مقبولة عندما يسمح بها العقد، ولا ينبغي إجبار النموذج على اختراع سعر.
- توجد `thought_signature` ضمن `nativeMerchantChatCall` الحالي؛ بعض المزودين يحتاج بيانات تفكير إضافية لإعادة تشغيل سجل الأدوات. **عالجها وفق وثائقه**، لكن لا تخزن أسرار تفكير حساسة بشكل غير مقصود ولا تمحُ metadata لازمة لاستمرار الجلسة. إن لم يحمل العقد الحالي ما يحتاجه المزود، فهذا **عدم توافق مثبت** يجب معالجته بأصغر امتداد، لا بخدعة داخل الـAdapter.
- إعداد `merchant_catalog_ai.go` لا يستدعي مزودًا خاصًا؛ يتلقى `MerchantCatalogAuthoringPort` من `bootstrap`. لا تغيّر منطق التنفيذ أو مراجعة التاجر لتبديل شركة AI.

### 4.6 دعم التلخيص

`NativeCustomerSalesAdapter.Summarize` يرسل رسالتي `system/user` دون أدوات ويشترط نصًا نهائيًا غير فارغ. اختبره مستقلًا: لا تعيد tool call أو اقتراح مبيعات بدل الملخص.

### 4.7 ربط مزود واحد في التكوين

الموضع الفعلي الحالي: `internal/bootstrap/integrations.go` ينشئ `native.NewDeepSeek(...)` ثم `native.NewRouter(provider, nil, native.Policy{})` ثم `native.NewCompletionClient(router)` ثم يمرر العميل المشترك إلى Customer/Catalog/Merchant. هذه هي نقطة الاستبدال المحدودة.

عند إضافة مزود جديد:
1. أضف إعدادات مفتاحه ومعرّف النموذج والـTimeout اللازمة إلى `internal/platform/config/config.go` ووثائق `.env.example`، مع Fail-closed عند النقص.
2. بدّل أو اختر إنشاء الـConcrete Adapter في `bootstrap/integrations.go` فقط حسب قرار النشر. **لا تبنِ نظام اختيار من لوحة التحكم** بدون طلب.
3. تأكد أن جميع المسارات الثلاثة تستخدم الـ`CompletionClient` نفسه إذا كان المطلوب تشغيل نموذج واحد. اختيار نموذج منفصل لكل مسار **قرار جديد** وليس نتيجة تلقائية لثلاثة أسماء `mujeeb-*`.
4. حدّث `NativeAISettings.Provider/Model/Configured` حتى لا تعرض لوحة الإدارة مزودًا مختلفًا عن الذي يستخدمه النظام.
5. حدّث `render.yaml` وملفات البيئة؛ **لا تحفظ أسرارًا حقيقية في Git**. لا تحذف مفاتيح تشفير تاريخية أو تغيّر قاعدة البيانات لأجل استبدال النموذج.
6. لا تعلن أن Render انتقل للفرع الجديد لمجرد تغيير `render.yaml`: `autoDeployTrigger: off` في الملف الحالي؛ يلزم نشر فعلي وتحقق.

### 4.8 قواعد HTTP والأمان المهمة

- استخدم `http.NewRequestWithContext(ctx, ...)`؛ احترام Cancel/Deadline؛ وأضف مهلة قابلة للضبط.
- عالج `resp.StatusCode` بنفسك؛ `http.Client.Do` لا يعتبر HTTP 429 أو 503 خطأ تلقائيًا. أغلق `resp.Body` دائمًا، واقرأ الرد بحد حجم لا يستهلك الذاكرة بلا سقف.
- افصل 401/403/429/503/502/504/timeout عن JSON مرفوض أو Schema غير مدعوم. لا تعيد محاولة تنفيذ عملية تجارية من Adapter؛ Adapter مسؤول عن **توليد النموذج فقط**.
- تحقّق من الوجهة وHTTPS والتحويلات redirects؛ لا تقبل endpoint يحدده العميل ولا ترسل Authorization إلى دومين غير موثوق.
- لا تُمرّر `API_KEY` أو نص العميل الخام في رسائل الخطأ أو logs. أزل تفاصيل المزود التي قد تسرب بيانات حساسة قبل تسجيلها.
- تحقّق من `finish_reason` وعدد `choices` ومنع tool calls غير المعروضة واستجابات الـRefusal والـTruncation والـJSON غير الصحيح.
- لا تخلط الـAssistant tool call مع رد العميل النهائي، ولا تعتبر اقتراح النموذج تنفيذًا مؤكدًا.
- إن رجع المزود بتنسيق `reasoning` أو `thought` خاص، فلا تعيده كنص محادثة أو تتركه يتسرب إلى جهة لا تحتاجه.

## 5. مصفوفة الاختبارات المطلوبة — ممنوع اعتماد المزود بمجرد بناء Go

### 5.1 اختبارات Adapter باستخدام httptest

اكتب اختبارات `<provider>_test.go` بخادم محلي من `net/http/httptest` لتثبت **شكل الطلب الخارج** إلى المزود و**شكل الرد الداخل** إلى مجيب:
- الموديل والـEndpoint والـHeaders والتوثيق وإخفاء المفتاح.
- System/User منفصلان؛ جميع الأدوات المسموحة مع Description/Schema؛ `required` للعميل و`auto` لاستكشاف Merchant.
- رفض أداة غير موجودة أو حجج schema خاطئة أو ID مكرر أو عدة أدوات عندما المسار يتطلب واحدة.
- رسالة tool-result مع `tool_call_id` الصحيح؛ حفظ الحالة/التوقيع الذي يتطلبه المزود عند كل جولة.
- استجابة مرشحي كتالوج Structured JSON صحيحة؛ ثم قرار Catalog Final.
- Merchant proposal envelope والنص الداخلي الكامل دون إسقاط خصائص تجارية.
- رفع 400/401/403/429/503 وإلغاء Context وTimeout وresponse أكبر من الحد، ورفض اختلاط text/tool أو رد ناقص.
- Missing/unsupported vendor functionality → **خطأ واضح**، لا اختيار أداة بديل بالـKeywords.
- اختبارات عدم وصول أسرار أو خام الـPrompt إلى logs العادية.

### 5.2 اختبارات العقود باستخدام Provider وهمي

أعد استخدام اختبارات العقود الموجودة، أو أضف اختبارات مماثلة في:
- `gateway/native_customer_adapter_test.go` و`gateway/capability_contract_test.go`: قدرات العملاء والمخرجات المصرّح بها.
- `gateway/deepseek_direct_integration_test.go`: مثال اختبار الـHTTP wire الفعلي لمسار العميل عبر محاكاة؛ ليس اختبار خدمة حية.
- `gateway/native_catalog_adapter_test.go` و`batch_candidate_scope_test.go`: دلائل Catalog والدفعات.
- `gateway/native_adapters_contract_test.go` و`native_merchant_listing_test.go`: Merchant tool rounds، Pagination، Proposal validation.
- `merchantcatalogai/discovery_test.go` و`execution_test.go`: عدم تسرب صلاحيات أو تنفيذ بلا اعتماد.
- **نقطة نقص حالية:** اختبار `provider_live` الموجود يركز على العميل والتلخيص، وليس دورة Merchant كاملة. لذلك أضف اختبار قبول Merchant مستقلًا عند إضافة مزود، ثم نفّذه بالمفتاح الحقيقي في بيئة آمنة.

### 5.3 اختبار مصفوفة السلوك من البداية للنهاية

| الحالة | الإخراج المطلوب | ما يجب ألا يحدث |
| --- | --- | --- |
| «أهلاً، متى تفتحون؟» مع دليل سياسة | قدرة رد مناسبة بحسب النموذج والدليل | إجبار Full Catalog من Go |
| «كم سعر سامسونج 256؟» بدون دليل مكتمل | النموذج يستطيع اختيار `evaluate_full_catalog` وعرض المرشحين | اختيار المنتج/السعر اختراعًا |
| طلب معلومات ناقصة | `ask_customer` عندما يكون النقص حقيقيًا | سؤال العميل مرة أخرى عن معلومة موجودة |
| طلب خدمة غير قابلة للتنفيذ | `handoff_to_human` عند الحاجة وبحسب السياسة | ادعاء تنفيذها |
| Full Catalog كبير | تقسيم صحيح وتغطية وعدم تجاوز budget | إسقاط عناصر صامتًا |
| طلب «أضف منتجًا مع سعر واضح» من التاجر | اقتراح إنشاء مطابق للعقد | تحديث DB من Adapter |
| «غيّر سعر العرض الموجود» | Merchant يستدعي قراءة صحيحة ثم Update Proposal | إنشاء عرض جديد بدل تحديثه دون طلب |
| «اعرض كل المنتجات» مع Pagination | متابعة `has_more/next_cursor` عند الحاجة | الاكتفاء بصفحة واحدة باعتبارها الكل |
| «احذف المنتج» من التاجر | اقتراح Delete ضمن الصلاحيات ومراجعة قبل التنفيذ | حذف مباشر من النموذج |
| رد API غير صالح/انقطاع جولة | خطأ آمن وسجل تشخيص محدود | ادعاء نجاح أو إرسال جواب ملفق |
| تلخيص محادثة | نص ملخص واضح | استدعاء أداة بيع |

### 5.4 أوامر Go وحالة CI

```bash
gofmt -l internal/adapters/secondary/ai/native/*.go
go test -count=1 ./internal/adapters/secondary/ai/native/... ./internal/adapters/secondary/ai/gateway
go test -count=1 -race ./internal/adapters/secondary/ai/native/... ./internal/adapters/secondary/ai/gateway
go test ./...
go build ./...
go vet ./...
```

هذه **أوامر تحقق مطلوبة للمطور** وليست نتائج تنفيذ توثيقي. `.github/workflows/native-ai-verify.yml` الحالية تفحص البناء والـRace Tests و`go test ./...` و`go vet ./...`. اختبارات `httptest` تختبر ترجمة البروتوكول دون دفع تكلفة API؛ **ولا تثبت وحدها التوافق الحي**.

### 5.5 اختبار حي محدود قبل الإنتاج

- وفر Secret في بيئة آمنة، ولا تدخل المفتاح في المستودع أو مستندات أو Logs.
- نفذ اختبار عميل يسأل عن منتج، ثم تجربة تقييم كتالوج، ثم اختبار Merchant متسلسل بثلاث جولات تقريبًا وأداة قراءة فعلية على بيانات تجريبية، ثم إخراج الاقتراح.
- قارن **القرار ودقته وليس نجاح HTTP 200 فقط**؛ راجع خروج نفس ID والسياق عبر الأدوات.
- اختبر حالات null، pagination، أسماء عربية، حالات missing fields، timeout، rate limiting، schema differences.
- اختبر حجم كتالوج قريبًا من الإنتاج بالحدود الحقيقية، و`NATIVE_AI_MAX_CONTEXT_TOKENS` مضبوطًا على **قدرة النموذج المتحقق منها**.
- أثبت أن Tenant A لا يستطيع قراءة Tenant B وأن Proposal لا ينفَّذ قبل موافقة صحيحة. يمكن استخدام DB تجريبية، وليس بيانات تاجر حقيقية بلا موافقة.
- سجّل Model ID وتاريخ الوثائق ونتائج كل سيناريو، وليس API Key أو محتوى حساسًا.
- إن فشل اختبار أساسي: **لا تنشر النموذج** ولا تغطِّ الفشل بكلمات مفتاحية أو إرسال نص غير موثق.

## 6. قبول التنفيذ ونشره/التراجع عنه

**ملفات PR المتوقعة (الحد الأدنى، بحسب حاجة المزود):**
- ملف تنفيذ `native/<provider>.go` وربما مساعد Schema واحد إذا لزم.
- `native/<provider>_test.go`.
- تعديل محدود بـ`config/config.go` و`bootstrap/integrations.go`.
- تحديث `.env.example` و`render.yaml` ووثيقة توافق النموذج.
- اختبارات تكامل Customer/Catalog/Merchant تُثبت النجاح دون تعديل منطق الأعمال.

**قبل الدمج أو النشر، راجع التالي:**

- [ ] اسم موديل API وطريقة المصادقة مستمدان من وثائق المزود وليس الذاكرة.
- [ ] استخدامات مجيب الثلاثة وSummarize مؤهلة على هذا النموذج.
- [ ] Adapter يعيد العقود الصحيحة ولا يقرر Intent أو Tool نيابة عن النموذج.
- [ ] لا اختلاف غير مبرر في `application/services` أو `merchantcatalogai/agent.go` أو `merchantcatalogai/execution.go` أو `ports` التجاري.
- [ ] محفوظة: Tenant Scope، سياسات البيع، موافقة الموظف، صحة Item/Variant/Offer.
- [ ] تم فحص tool IDs والتاريخ وSchema/Pagination/Proposal.
- [ ] تم اختبار غياب الإمكانيات، Timeout، 429/503، response malformed، context limits.
- [ ] نجحت اختبارات Go كاملة وRace Tests وBuild وVet.
- [ ] نجحت الاختبارات الحية لدى الحساب المستهدف، ومنها Merchant.
- [ ] لا أسرار في Commit أو Debug Dumps أو لقطات شاشة منشورة.
- [ ] CI قبل الدمج، ونشر مضبوط، وخطة رجوع إلى آخر إعداد مُختبَر إذا فشل التشغيل.
- [ ] لا تُغيّر فرع الإنتاج أو إعداد Render أو تخزين البيانات **ضمن PR توثيق فقط**.

**خطة الرجوع عند فشل نشر مزود جديد:** أعد نشر آخر Commit وتشغيل إعدادات المزود/النموذج المعروفة بالنجاح، مع استرجاع الـSecrets اللازمة عبر إدارة بيئة النشر، ثم افحص سجل التشغيل؛ لا تغيّر حالة المحادثات أو ترحيلات قاعدة البيانات للتراجع عن تكامل نموذج.

## 7. أخطاء شائعة نمنعها صراحةً

1. إنشاء Intent Engine في Go يختار الأدوات من كلمات العملاء بدل النموذج.
2. الاعتقاد بأن `Model` في `bootstrap` باسم `mujeeb-sales` نموذج حقيقي لدى المزود.
3. استخدام `tool_choice=required` بلا التحقق من توافق نموذج/وضع Thinking.
4. إرسال Structured JSON النهائي في نفس جولة Merchant التي تحمل أدوات، خلافًا للعقد الحالي.
5. قبول `assistant tool_call` دون Result مرتبط بـID الصحيح، أو فقد State/Signature المطلوبة.
6. إرجاع Tool Call مصطنع إذا لم يرجعه النموذج؛ هذا يزيّف قرار النموذج.
7. افتراض أن `httptest` يساوي نجاح API الحقيقي، أو أن الاختبار الحي لعميل واحد يثبت Merchant.
8. تفعيل مفاتيح/النشر قبل تأهيل النموذج وتحديد حدود السياق.
9. الإفراط في التجريد: Registry/Framework متعدد مزودين في لوحة الإدارة دون حاجة.
10. ترجمة أخطاء المزود إلى «تم تنفيذ طلبك» بدل Fail-closed.
11. توسيع مسؤولية Adapter ليكتب منتجات أو يقر صفقات.
12. إلغاء صفحات الكتالوج أو نتائج الأدلة لمجرد أنها كبيرة.
13. خلط نص مستخدم غير موثوق مع System Instructions.
14. تغيير ملفات SQL مطبقة سابقًا أو التلاعب ببيانات الإنتاج لمجرد تغيير مزود AI.

## 8. توثيق المصادر: ما هو حقيقة كود وما هو إرشاد عام؟

**حقائق من الكود عند Commit المحدد:** الواجهات وأسماء الملفات، `required/auto`، جولات Merchant القصوى، مسار `canonical_proposal_json`، أسماء `bootstrap`، ومفاتيح التكوين؛ يمكن مطابقتها بروابط الملفات في القسم (1). تتغير هذه الحقائق فقط عندما يتغير الكود.

**أفضل ممارسات خارجية (مصادر رسمية، راجع آخر إصدار عند اختيار المزود):**

- Go Code Review Comments — Context/Interfaces/Error Handling: https://go.dev/wiki/CodeReviewComments
- Google Go Best Practices — تجنب interfaces/abstractions غير الضرورية: https://google.github.io/styleguide/go/best-practices.html
- Google Go Guide — البساطة والقابلية للصيانة: https://google.github.io/styleguide/go/guide.html
- Go HTTP — `NewRequestWithContext`، `Client.Do`، `Body.Close` وعدم اعتبار non-2xx خطأ Go تلقائيًا: https://pkg.go.dev/net/http
- Go `httptest` لاختبار البروتوكول محليًا: https://pkg.go.dev/net/http/httptest
- DeepSeek — Tool Calls/strict Beta: https://api-docs.deepseek.com/guides/tool_calls/
- DeepSeek — Chat Completion/tool_choice: https://api-docs.deepseek.com/api/create-chat-completion/
- DeepSeek — Thinking Mode/history: https://api-docs.deepseek.com/guides/thinking_mode/
- Google Gemini — Function Calling: https://ai.google.dev/gemini-api/docs/function-calling
- Google Gemini — Thought Signatures: https://ai.google.dev/gemini-api/docs/generate-content/thought-signatures

**الوثائق الرسمية لا تضمن جاهزية نموذج معيّن داخل مجيب**. الوثائق تحدد ما ينبغي على Adapter فعله؛ **اختبارات العقود وAPI الحقيقي** تثبت ما نفذه فعلًا.

---

## 9. معيار القبول النهائي بجملة واحدة

> **إضافة مزود جديد مؤهل تستلزم كتابة Adapter له وفق وثائقه وربطه بإعداد محدود واختباره عبر Customer Sales + Full Catalog + Merchant Catalog AI، دون تغيير منطق الأعمال، ودون أن يخترع Go نية العميل أو التاجر أو ينفذ المزود أدواته بنفسه.**
