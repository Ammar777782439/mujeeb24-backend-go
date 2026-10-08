# حالة مشروع Mujeeb 24 Backend

> **آخر تحديث:** 2026-10-08. هذه الوثيقة تصف الفرع `feat/universal-catalog-ai-v3`؛ لا تعني دمجه في main أو فرع الإنتاج.

## الخلاصة التنفيذية

`Mujeeb 24` هو Backend Go متعدد التجار، يملك بيانات الأعمال والمحادثات والعملاء والرسائل والقرارات وسجل التدقيق داخل PostgreSQL. يعتمد على SocialAPI كناقل قنوات فقط. تم حذف runtime وCompose وadapters وroutes الخاصة بـChatwoot من هذا الفرع؛ لا تُشغّل خدمة خارجية للمحادثات في المنتج أو في Compose المحلي.

| المجال | الحالة في هذا الفرع | حد الإثبات |
| --- | --- | --- |
| Go API وHuma/OpenAPI | محدث ومختبر | عدد العمليات يُستخرج من المولد الحالي؛ لا يُثبّت رقم تاريخي في تقرير الحالة |
| JWT وtenant scope | موجود | مثبت باختبارات الوحدات الموجودة؛ راجع التشغيل الحي منفصلًا |
| PostgreSQL migrations | ترحيلات تراكمية؛ راجع مجلد migrations الحالي | لا يوجد إسقاط للـschema التاريخي في هذه الدفعة |
| Event Ledger وOutbox | موجودان | يسجلان ويعالجان ضمن مسار Mujeeb |
| SocialAPI inbound | موجود | verify/normalize ثم materialization داخل Mujeeb |
| AutoReply | مربوط مباشرة بـSocialAPI inbound | يظل متوقفًا افتراضيًا ويتطلب LLM/policy محليين |
| Outbound worker | موجود | يعالج Outbox ويرسل عبر SocialAPI عند تهيئة المزود |
| Merchant provisioning | SocialAPI-only | OAuth ثم `ChannelConnection`؛ لا ينشئ موارد خدمة محادثات خارجية |
| Compose محلي | Mujeeb/PostgreSQL/API/worker/gateway فقط | لم يُشغّل Compose داخل sandbox في هذه الجولة |
| SocialAPI حي وإرسال خارجي | غير مثبت في هذه الجولة | يحتاج credentials آمنة واختبارًا منفصلًا وموافقًا عليه |

## مسار الرسالة الحالي

```text
SocialAPI webhook موثّق
  → inbound_event_ledger
  → Customer / Conversation / CommunicationMessage
  → AutoReply اختياري: Context + Policy + AIDecision
  → OutboundMessage + Outbox
  → worker
  → SocialAPI send
```

تظل قاعدة البيانات مصدر الحقيقة. لا يُنفذ الـworker اتصالًا خارجيًا داخل transaction، ولا تُعاد الحالة الخارجية غير المعروفة تلقائيًا بلا reconciliation أو مراجعة.

## قرار schema التاريخي

الـmigrations من `000001` حتى `000045` غير قابلة للتعديل. توقفت خدمات وقراءَات/كتابات Chatwoot في runtime، لكن جداول وأعمدة تاريخية لا تزال في schema كي لا تُحذف بيانات أو تفسد ترقية بيئة قائمة. أي تنظيف لاحق يحتاج backup وخطة restore وموافقة صريحة، ثم migration forward-only جديدة.

## التحقق المتبقي دون دمج

1. تشغيل بوابة الجودة: `gofmt` و`go test ./...` و`go vet ./...` وOpenAPI drift وsecret scan.
2. محاولة PostgreSQL integration عبر `POSTGRES_TEST_DSN` إذا كانت بيئة PostgreSQL متاحة؛ لا يُدّعى نجاحها إن كانت الحاويات غير متاحة.
3. التحقق البنيوي من Compose وCaddy في صيغة Mujeeb-only.
4. مراجعة ملخص الأثر من المستخدم قبل أي `commit` أو `push` إلى الفرع. لا يحدث merge إلى `main` إلا بموافقة منفصلة.

## المستندات المرجعية

- [قرار الترحيل إلى Mujeeb-only](architecture/chatwoot-free-migration-ar.md)
- [دليل التشغيل المحلي](../deploy/local/README-ar.md)
- [دليل المطور](developer-guide-ar.md)

## AI الحالي والتنظيف

مسار B2B يستخدم MerchantCatalogAuthoringAdapter؛ مسار B2C يستخدم GeminiCustomerSalesAdapter عبر Interactions مع CatalogBatchController للتقييم الكامل بالدفعات. لا يوجد Customer Sales tool-loop قديم أو registry catalog_data في التشغيل الحالي. Gemini يقترح وMujeeb يتحقق ويطبق السياسة.

AutoReply عبر webhook يحتاج بوابة التكلفة وWorker Pool. غياب الاعتماديات يمنع بدء AI مع استمرار استقبال الرسالة. الاختبارات المحلية لا تثبت اتصال PostgreSQL أو المزود حيًا؛ لا تُعد حالات skipped نجاحًا.
