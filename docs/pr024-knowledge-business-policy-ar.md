# PR-024 — Knowledge Repository وBusiness Policy Extension

## الحكم التنفيذي

تم تنفيذ Knowledge Repository وBusiness Policy Extension فوق schema موجودة، مع تصحيح مهم قبل الإغلاق: كان جدول `business_policies` موجودًا أصلًا منذ migration `000002` ويملك إعدادات business/AI one-to-one مثل `ai_mode` و`allow_auto_reply`. لذلك لم يتم إنشاء جدول ثانٍ بالاسم نفسه، ولم يتم تعديل migration قديمة.

أُضيفت migration forward-only رقم **000036** تنشئ:

- `knowledge_documents` لمحتوى المعرفة المملوك للنشاط.
- `business_policy_versions` لإصدارات السياسات التجارية التفصيلية.

ويبقى `business_policies` القديم مصدر إعدادات business-level الموجودة مسبقًا، بينما `business_policy_versions` هو مصدر policy evidence التفصيلي القابل للإصدار والانتهاء.

## الملكية والعزل

كل Knowledge document وكل Policy version يحمل `business_id`، ويرتبط بـ`businesses(id)` مع `ON DELETE RESTRICT`. جميع repository reads تقيد business scope، وContextBuilder يعيد فحص business وparent references قبل إرسال evidence إلى AIRuntime.

> **قاعدة الملكية:** Dashboard/Application يملكان policy authoring لاحقًا، Mujeeb/PostgreSQL هو مصدر الحقيقة، والـLLM لا يملك أي صلاحية كتابة أو تنفيذ.

## Knowledge contract

`KnowledgeDocumentRecord` typed record يحتوي `knowledge_key` و`title` و`content` و`content_type` و`source_reference` و`authority` و`status` و`version` و`valid_from` و`valid_until` وtimestamps. أنواع المحتوى الحالية محدودة إلى `faq` و`hours` و`location` و`service` و`general`، والـauthority هي `merchant` أو `system`.

Repository surface الحالية read-only ومقصودة للسياق: `ListPublished(ctx, businessID, search, now, limit)`. تُرجع published records التي بدأت صلاحيتها قبل `now`، بما فيها المنتهية، حتى يستطيع ContextBuilder وسمها `stale` بدل خلطها مع `missing`. Drafts والمستقبلية لا تدخل.

## Business policy contract

`BusinessPolicyRecord` typed record يحتوي `policy_key` و`category` و`title` و`summary` و`rules` كـraw JSON object متحقق منه، إضافة إلى `authority=merchant` و`status` و`version` وvalidity timestamps.

الفئات الحالية هي `hours` و`returns` و`delivery` و`pricing` و`availability` و`ai_mode` و`channel` و`general`. توجد uniqueness لكل `(business_id, policy_key, version)`، وpartial unique index يمنع أكثر من نسخة published واحدة لنفس policy key داخل business.

## Context Builder

تم ربط `KnowledgeDocumentRepository` و`BusinessPolicyRepository` داخل ContextBuilder. يتم ranking محلي deterministic على `knowledge_key/title/content` أو `policy_key/category/title/summary/rules`، مع limits، tenant checks، evidence references، schema version، retrieved time، وvalidity.

حالة context أصبحت أكثر دقة:

| الحالة | المعنى |
|---|---|
| `fresh` | evidence حالية وصالحة وقت البناء |
| `stale` | evidence منشورة لكن انتهت صلاحيتها أو availability غير موثوقة |
| `partial` | يوجد بعض grounding لكن مصدرًا مطلوبًا غير موجود أو stale |
| `missing` | لا يوجد catalog/knowledge match صالح للسؤال |
| `grounded` | يوجد Knowledge أو Policy evidence صالحة مرتبطة بالسؤال |

الـLLM يستلم Knowledge وBusiness Policy evidence المحددة فقط. لا يستلم Customer profile/contact raw، ولا يستطيع استدعاء SQL أو provider API.

## Policy Engine المحدود

تمت إضافة `AIPolicyEvaluator` و`GroundedPolicyEngine`. هذا ليس Policy Engine مؤسسيًا كاملًا، لكنه gate deterministic آمن قبل persistence:

- سؤال availability أو price بلا Offer evidence صالحة، أو مع evidence stale، يتحول إلى `requires_approval` و`RequiresHuman=true`.
- سؤال hours/returns/delivery بلا merchant policy منشورة يتحول إلى `requires_approval`.
- الإجابة factual يجب أن تحمل reference يطابق evidence موجودة في Mujeeb.
- التحية أو الرد غير factual لا تُحجب لمجرد غياب catalog.
- القرار لا يُسمح له بتجاوز authorization أو transaction invariant أو connection capability.

عند تحويل proposal إلى `requires_approval` لا ينشئ AutoReply outbound أو Outbox؛ يمكن حفظ AIDecision كسجل مقترح للمراجعة وفق المسار الموجود.

## الاختبارات الفعلية

نجحت الاختبارات التالية:

| الاختبار | النتيجة |
|---|---|
| `go test ./...` | **PASS** |
| `go vet ./...` | **PASS** |
| OpenAPI generation وdrift check | **PASS** |
| schema runner على PostgreSQL 16: applied 36 ثم 0 | **PASS** |
| Policy Engine unit tests | **PASS** |
| ContextBuilder knowledge/policy evidence tests | **PASS** |
| Knowledge/Policy repository integration tests على PostgreSQL 16 حقيقية | **PASS** |
| ContextBuilder Catalog + Knowledge + Policy grounding على PostgreSQL 16 حقيقية | **PASS** |
| Tenant isolation وfuture/draft/expired validity | **PASS** |
| Published policy uniqueness constraint | **PASS** |
| `git diff --check` | **PASS** |

في أثناء الاختبار ظهرت أخطاء حقيقية وتم إصلاحها: تعارض إنشاء `business_policies` القديمة، category `availability` الناقصة في constraint، وتمثيل JSONB/NUMERIC في assertions. لم يتم تجاهل هذه الحالات.

## Git state

التغييرات الحالية اجتازت validation، وسيتم دفعها في commit مستقل بعد التدقيق النهائي. لم يتم تعديل migrations `000001` إلى `000035`، ولم يتم لمس SocialAPI أو Chatwoot أو provider live.

## ما لم يُغلق

لا يوجد بعد semantic/vector search، ولا supplier availability adapter، ولا authoring HTTP API كامل للـKnowledge/Policy، ولا policy rule language متقدم. كما لم يتم تشغيل Chatwoot live أو SocialAPI live أو Facebook أو LLM live مع بيانات knowledge حقيقية.

هذه الدفعة تغلق **read-side Knowledge/Policy grounding وPolicy safety gate المحلي** فوق PostgreSQL حقيقية، ولا تعني أن التشغيل الخارجي الحي أصبح مفعلًا.
