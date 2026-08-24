# حالة مشروع Mujeeb 24 Backend Go

> هذه الوثيقة هي نقطة الرجوع الأولى عند فتح المستودع. آخر تحديث: 2026-08-25.

## الخلاصة التنفيذية

`mujeeb24-backend-go` هو Backend مستقل ونظيف لمجيب 24. أُنشئ بعيدًا عن Prototype القديم المرتبط بـPostiz/Facebook. المشروع أنهى تصميم **Domain Contracts وApplication Ports وPersistence Boundary**، وثبّت SQL migrations من 000001 إلى 000029 مع اختبارات PostgreSQL وFull Schema ناجحة. كما نفّذ DTO-first HTTP Contract: Go DTOs وoperation registration هي مصدر الحقيقة، وOpenAPI v1 يُولد تلقائيًا منها. أُنجزت CommunicationMessage وChannel Capabilities وCatalog read/write persistence، بينما بقية Repositories التجارية وProvider integrations وAI runtime لم تبدأ.

التاجر سيستخدم Dashboard مجيب 24 فقط. SocialAPI.ai سيكون Provider لنقل رسائل Facebook وInstagram وWhatsApp، وChatwoot سيكون Communication Workspace داخليًا عبر API Channel/Adapter. أما Go فهو مالك Sales Intelligence وBusiness Knowledge وCatalog وLeads وCommercial Transactions وAI Decisions.

## الحالة الحالية

| المجال | الحالة | الملاحظة |
|---|---|---|
| GitHub repository | مكتمل | Private، branch `main` |
| Go module | مكتمل | Foundation module قابل للبناء |
| Baseline Architecture | مغلق | Modular Monolith: Bootstrap، Domain، Application، Primary/Secondary Adapters، Platform |
| Chatwoot Feasibility Spike | مكتمل خارج هذا المستودع | نتائج موثقة في ملفات العمل المحلية؛ Chatwoot Sidecar قرار مشروط |
| Domain/shared وbusiness | مغلق كتصميم | لم يتحول بعد إلى Go implementation |
| Domain/channel | مغلق كتصميم | Provider-neutral، Inbound/Outbound/Delivery |
| Domain/identity وcommunication | مغلق كتصميم | Customer/ExternalIdentity/Conversation References |
| Domain/catalog وsales | مغلق كتصميم | CatalogItem/Offer/Variant/Transactions متعددة الأنواع |
| Domain/ai وaudit | مغلق كتصميم | Structured Decision، Evidence، Policy، Audit |
| application/ports | مغلق كتصميم | Go interfaces التنفيذية تحتاج ضبطًا نهائيًا أثناء Adapter work |
| HTTP API Dashboard V1 Contract | مغلق كتصميم | Dashboard-only، JWT، Webhooks منفصلة، Auth/Tenant/Error/Pagination/Idempotency موثقة |
| Auth transport | JWT معتمد تصميميًا؛ middleware boundary منفذ | `RequireAccessToken` قابل للحقن ويضع Principal فقط؛ JWT issuer/key storage وMembership غير منفذين |
| PostgreSQL Persistence Contract | مغلق كتصميم | Composite Tenant FKs وLeases وIdempotency موثقة |
| PostgreSQL schema/migrations | مكتملة 000001–000029 | Migration Runner مضمّن ويطبقها من قاعدة فارغة؛ لا AutoMigrate؛ 000028 forward-only لـCommunicationMessage و000029 forward-only لـCatalog resource versions؛ Catalog migrations 000013–000018 مثبتة |
| PostgreSQL constraint tests | Foundation وFull Schema ناجحة | تشمل Tenant Isolation وIdempotency وLeases وSnapshots |
| Event Ledger/Idempotency/Outbox | تصميم + SQL tables | Go implementation لم يبدأ؛ يأتي بعد إكمال Repository surfaces بما فيها CommunicationMessage وChannel Capabilities وCatalog read/write |
| Provider Simulator | لم يبدأ | مطلوب قبل أي credentials حقيقية |
| SocialAPI.ai integration | غير مثبت | لا توجد credentials إنتاجية أو test credentials |
| Chatwoot Adapter | غير منفذ | Feasibility مثبتة، Adapter لم يُكتب |
| Facebook/Instagram/WhatsApp الحقيقي | غير مثبت | لا ندّعي نجاحًا قبل اختبار فعلي آمن |

## ما تم رفضه

لا نستخدم المستودع القديم `yemen-social-reply-engine` كقاعدة تطوير. لا نعيد Facebook adapter القديم. لا نجعل Chatwoot أو SocialAPI مصدر Sales Truth. لا نضع Provider DTOs داخل Domain. لا نستخدم `float64` للأموال ولا `AutoMigrate` للإنتاج.

## معيار إغلاق الـMigrations

أُغلق تنفيذ الـFull Schema بعد تطبيق migrations من 000001 إلى 000029 على PostgreSQL فارغة، ونجاح اختبارات Tenant Isolation وProvider Event Dedupe وScoped Outbound Idempotency وUnresolved Event وOutbox Lease وSnapshots، ونجاح CommunicationMessage وCatalog constraints، ونجاح تشغيل Runner مرتين (`applied=29` ثم `applied=0`). هذا لا يعني أن Event Ledger أو Outbox Go implementation أصبحا منفذين.

## الحالة الحالية: PR-009 — PostgreSQL Adapter وTransactionManager

أُغلق PR-008 عند حد HTTP → Application façade wiring بنتيجة 76/76. في PR-009 نُفذ PostgreSQL pool وTransactionManager وSQLExecutor وtyped repository errors، ثم Business/Customer/Conversation/ChannelConnection/ConversationReference/OutboundMessage foundation، وCommunicationMessage timeline، وChannel Capabilities، وCatalog read/write persistence الحالية. نجح integration test حقيقي على PostgreSQL 16 يثبت Catalog read/order/filter/pagination وJSON/decimal preservation وdefinitions mapping وtenant not-found، إضافة إلى create/update وresource-version 1→2 وstale rejection وschema version/definitions وtransaction commit/rollback. ما زالت Sales/AI/Audit repositories وEventStore وIdempotency/Outbox implementations وbootstrap wiring وsemantic attribute validation مؤجلة.

## معيار إغلاق HTTP API Contract وDTO-first Generation

أُغلق عقد Dashboard V1 تصميميًا بعد تحديد routes وroles وpermissions وRequest/Response envelopes وErrors وPagination وIdempotency وConcurrency وApplication Command/Query mapping، واعتماد JWT Access Authentication، مع فصل Webhooks وOperational endpoints وDeferred Scope. ثم تحوّل العقد إلى Go DTOs وHuma operation registration، وتولد منه OpenAPI 3.0.3 في `api/openapi/mujeeb24-dashboard-v1.generated.yaml`.

يمنع `scripts/check-openapi-generated.sh` اختلاف الملف المولد عن Go source، ويُشغل مع `go test ./...` في CI أو محليًا. أُضيفت Typed Commands/Queries، ووُصلت كل عمليات Huma الـ76 بـruntime dispatcher مشترك، ولكل عملية façade typed صريحة تبني Command/Query أو system contract. أربع Core callbacks لديها dependencies تنفيذية موصولة؛ بقية operations تعبر façade typed وقد تعيد `not_implemented` من Application boundary عند غياب implementation dependency. أضيف Auth middleware boundary واختباراته، لكن JWT issuer/key storage وMembership وFunctional Application logic غير منفذة. نُقلت DTOs الفعلية إلى `http/dto`، وأصبح `http/contract` مسؤولًا عن operation registration وmetadata مع aliases توافقية فقط، وحُذف placeholder القديم من `handlers`.

## الخطوة التالية داخل PR-009

```text
PR-008 CLOSED: HTTP → Application façade 76/76
→ PR-009: PostgreSQL pool + TransactionManager ✅
→ Repository foundation + CommunicationMessage timeline ✅
→ Channel Capabilities ✅
→ Catalog read/write persistence ✅
→ Sales/AI/Audit repositories ⏳
→ Event Ledger: atomic inbound dedupe ⏳
→ Outbox persistence/worker boundary ⏳
→ bootstrap dependency wiring ⏳
```

لا نبدأ SocialAPI أو Chatwoot أو AI runtime قبل اكتمال Reliability Foundation وSimulator. ولا نعتبر Provider integration ناجحًا قبل اختبار حقيقي آمن لاحقًا.
