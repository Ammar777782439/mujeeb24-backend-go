# حالة مشروع Mujeeb 24 Backend Go

> هذه الوثيقة هي نقطة الرجوع الأولى عند فتح المستودع. آخر تحديث: 2026-08-24.

## الخلاصة التنفيذية

`mujeeb24-backend-go` هو Backend مستقل ونظيف لمجيب 24. أُنشئ بعيدًا عن Prototype القديم المرتبط بـPostiz/Facebook. المشروع أنهى تصميم **Domain Contracts وApplication Ports وPersistence Boundary وFull Migration Schema**. لم يبدأ بعد تنفيذ SQL migrations أو Repositories أو Provider integrations أو AI runtime.

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
| PostgreSQL Persistence Contract | مغلق كتصميم | Composite Tenant FKs وLeases وIdempotency موثقة |
| PostgreSQL schema/migrations | SQL لم يبدأ | سيُكتب من 000001 حتى 000027؛ لا AutoMigrate |
| PostgreSQL constraint tests | لم يبدأ | مطلوب قبل Reliability implementation |
| Event Ledger/Idempotency/Outbox | لم يبدأ | Reliability Foundation بعد نجاح migrations/tests |
| Provider Simulator | لم يبدأ | مطلوب قبل أي credentials حقيقية |
| SocialAPI.ai integration | غير مثبت | لا توجد credentials إنتاجية أو test credentials |
| Chatwoot Adapter | غير منفذ | Feasibility مثبتة، Adapter لم يُكتب |
| Facebook/Instagram/WhatsApp الحقيقي | غير مثبت | لا ندّعي نجاحًا قبل اختبار فعلي آمن |

## ما تم رفضه

لا نستخدم المستودع القديم `yemen-social-reply-engine` كقاعدة تطوير. لا نعيد Facebook adapter القديم. لا نجعل Chatwoot أو SocialAPI مصدر Sales Truth. لا نضع Provider DTOs داخل Domain. لا نستخدم `float64` للأموال ولا `AutoMigrate` للإنتاج.

## معيار إغلاق تصميم الـMigrations

أُغلق تصميم الـFull Schema بعد تثبيت Foundation وCatalog وSales وAI وAudit، مع Composite Tenant FKs، External References، Event Ledger، Outbox، Idempotency، Snapshots، وسياسة `RESTRICT/ARCHIVED` بدل الحذف المتسلسل من Business. هذا لا يعني أن SQL أو الاختبارات التنفيذية قد نجحت.

## الخطوة التالية الوحيدة

```text
Finalize typed IDs + composite FK strategy
→ Write SQL migrations 000001–000012
→ Run PostgreSQL constraint tests
→ Write SQL migrations 000013–000027
→ Run full-schema migration tests/review
→ Implement PostgreSQL Adapter
```

لا نبدأ SocialAPI أو Chatwoot أو AI runtime قبل اكتمال migrations والاختبارات. ولا نعتبر Event Ledger أو Outbox منفذين لمجرد أن تصميم الجداول مغلق.
