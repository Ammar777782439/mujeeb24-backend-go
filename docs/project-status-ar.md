# حالة مشروع Mujeeb 24 Backend Go

> هذه الوثيقة هي نقطة الرجوع الأولى عند فتح المستودع. آخر تحديث: 2026-08-24.

## الخلاصة التنفيذية

`mujeeb24-backend-go` هو Backend مستقل ونظيف لمجيب 24. أُنشئ بعيدًا عن Prototype القديم المرتبط بـPostiz/Facebook. المشروع أنهى تصميم **Domain Contracts وApplication Ports وPersistence Boundary وFull Migration Schema**، ونفّذ SQL migrations من 000001 إلى 000027 مع اختبارات PostgreSQL أساسية وFull Schema ناجحة. كما نفّذ DTO-first HTTP Contract: Go DTOs وoperation registration هي مصدر الحقيقة، وOpenAPI v1 يُولد تلقائيًا منها. لم يبدأ بعد تنفيذ Repositories التجارية أو Provider integrations أو AI runtime.

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
| Auth transport | JWT معتمد تصميميًا | EdDSA access token قصير العمر؛ Auth provider/storage وMembership contract منفصلان |
| PostgreSQL Persistence Contract | مغلق كتصميم | Composite Tenant FKs وLeases وIdempotency موثقة |
| PostgreSQL schema/migrations | مكتملة 000001–000027 | Migration Runner مضمّن ويطبقها من قاعدة فارغة؛ لا AutoMigrate |
| PostgreSQL constraint tests | Foundation وFull Schema ناجحة | تشمل Tenant Isolation وIdempotency وLeases وSnapshots |
| Event Ledger/Idempotency/Outbox | تصميم + SQL tables | Go implementation لم يبدأ؛ يأتي بعد HTTP boundary |
| Provider Simulator | لم يبدأ | مطلوب قبل أي credentials حقيقية |
| SocialAPI.ai integration | غير مثبت | لا توجد credentials إنتاجية أو test credentials |
| Chatwoot Adapter | غير منفذ | Feasibility مثبتة، Adapter لم يُكتب |
| Facebook/Instagram/WhatsApp الحقيقي | غير مثبت | لا ندّعي نجاحًا قبل اختبار فعلي آمن |

## ما تم رفضه

لا نستخدم المستودع القديم `yemen-social-reply-engine` كقاعدة تطوير. لا نعيد Facebook adapter القديم. لا نجعل Chatwoot أو SocialAPI مصدر Sales Truth. لا نضع Provider DTOs داخل Domain. لا نستخدم `float64` للأموال ولا `AutoMigrate` للإنتاج.

## معيار إغلاق الـMigrations

أُغلق تنفيذ الـFull Schema بعد تطبيق migrations من 000001 إلى 000027 على PostgreSQL فارغة، ونجاح اختبارات Tenant Isolation وProvider Event Dedupe وScoped Outbound Idempotency وUnresolved Event وOutbox Lease وSnapshots، ونجاح تشغيل Runner مرتين (`applied=27` ثم `applied=0`). هذا لا يعني أن Event Ledger أو Outbox Go implementation أصبحا منفذين.

## معيار إغلاق HTTP API Contract وDTO-first Generation

أُغلق عقد Dashboard V1 تصميميًا بعد تحديد routes وroles وpermissions وRequest/Response envelopes وErrors وPagination وIdempotency وConcurrency وApplication Command/Query mapping، واعتماد JWT Access Authentication، مع فصل Webhooks وOperational endpoints وDeferred Scope. ثم تحوّل العقد إلى Go DTOs وHuma operation registration، وتولد منه OpenAPI 3.0.3 في `api/openapi/mujeeb24-dashboard-v1.generated.yaml`.

يمنع `scripts/check-openapi-generated.sh` اختلاف الملف المولد عن Go source، ويُشغل مع `go test ./...` في CI أو محليًا. هذا لا يعني أن Handlers أو Application Commands أو Auth storage أصبحت منفذة؛ التسجيل الحالي HTTP skeletons للتوثيق والتحقق فقط.

## الخطوة التالية الوحيدة

```text
Write shared/endpoint DTO tests
→ Write HTTP route/handler skeletons over generated contract
→ Finalize Application Commands/Queries
→ Implement PostgreSQL Adapter/Reliability
```

لا نبدأ SocialAPI أو Chatwoot أو AI runtime قبل اكتمال Reliability Foundation وSimulator. ولا نعتبر Provider integration ناجحًا قبل اختبار حقيقي آمن لاحقًا.
