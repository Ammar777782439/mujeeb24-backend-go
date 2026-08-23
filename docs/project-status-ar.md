# حالة مشروع Mujeeb 24 Backend Go

> هذه الوثيقة هي نقطة الرجوع الأولى عند فتح المستودع. آخر تحديث: 2026-08-24.

## الخلاصة التنفيذية

`mujeeb24-backend-go` هو Backend مستقل ونظيف لمجيب 24. أُنشئ بعيدًا عن Prototype القديم المرتبط بـPostiz/Facebook. المشروع حاليًا في مرحلة **Domain Contract وFoundation**؛ لم يبدأ تنفيذ Repositories أو SQL migrations أو Provider integrations أو AI runtime.

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
| application/ports | لم يُغلق | الخطوة التالية بعد Domain Review |
| PostgreSQL schema/migrations | لم يبدأ | لن نستخدم AutoMigrate |
| Event Ledger/Idempotency/Outbox | لم يبدأ | Reliability Foundation إلزامي قبل AI |
| Provider Simulator | لم يبدأ | مطلوب قبل أي credentials حقيقية |
| SocialAPI.ai integration | غير مثبت | لا توجد credentials إنتاجية أو test credentials |
| Chatwoot Adapter | غير منفذ | Feasibility مثبتة، Adapter لم يُكتب |
| Facebook/Instagram/WhatsApp الحقيقي | غير مثبت | لا ندّعي نجاحًا قبل اختبار فعلي آمن |

## ما تم رفضه

لا نستخدم المستودع القديم `yemen-social-reply-engine` كقاعدة تطوير. لا نعيد Facebook adapter القديم. لا نجعل Chatwoot أو SocialAPI مصدر Sales Truth. لا نضع Provider DTOs داخل Domain. لا نستخدم `float64` للأموال ولا `AutoMigrate` للإنتاج.

## معايير الانتقال من المرحلة الحالية

لا ننتقل إلى Ports حتى تكون أسماء Aggregates وValue Objects وInvariants وحالات الانتقال واضحة. ولا ننتقل إلى SQL حتى تكون Ports واضحة. ولا ننتقل إلى Provider حقيقي حتى ينجح Provider Simulator في duplicate وretry وout-of-order وfailure وdelivery.

## الخطوة التالية الوحيدة

ابدأ بقراءة:

```text
contracts/domain_contract_index_ar.md
contracts/domain_shared_business_contract_ar.md
contracts/domain_channel_contract_review_ar.md
contracts/domain_identity_communication_contract_review_ar.md
```

ثم أغلق `application/ports`، دون كتابة API أو AI قبل ذلك.
