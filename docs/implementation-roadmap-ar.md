# خارطة التنفيذ — Mujeeb 24 Backend Go

## المرحلة الحالية: SQL Migrations وPersistence Tests

الحالة: **SQL migrations من 000001 إلى 000027 منفذة، وFoundation وFull Schema constraint tests ناجحة؛ العمل الحالي هو PostgreSQL Adapter وReliability Foundation**.

العقود الموجودة في `contracts/` تحدد shared وbusiness وchannel وidentity وcommunication وcatalog وsales وai وaudit، كما توثق Persistence Boundary وComposite Tenant FKs وEvent Ledger وOutbox وIdempotency. Migration Runner مضمّن في Go ويطبق الإصدار من قاعدة فارغة دون إعادة تنفيذ المigrations السابقة.

## المرحلة المنجزة تصميميًا: Application Ports

نكتب interfaces التي تحتاجها Use Cases فقط:

```text
ChannelProvider
CommunicationWorkspace
EventStore
IdempotencyStore
ConversationRepository
IdentityRepository
CatalogRepository
LeadRepository
TransactionRepository
Outbox
SecretStore
AIService
Clock
Transaction
```

لا يحتوي Port على SocialAPI DTO أو Chatwoot DTO. لا ينفذ Port شيئًا؛ هو عقد اختبار وحدود اعتماد.

معيار النجاح: يمكن اختبار Application باستخدام fakes دون تشغيل SocialAPI أو Chatwoot أو PostgreSQL.

## المرحلة الحالية: SQL Schema

نبني SQL migrations versioned، لا AutoMigrate:

```text
businesses
channel_connections
customers
external_identities
conversations
conversation_references
inbound_events
communication_messages
outbound_messages
outbox_jobs
catalogs
catalog_items
offers
leads
commercial_transactions
audit_events
```

كل جدول تجاري يحمل business scope حيث يلزم. كل unique constraint يترجم قاعدة Idempotency أو Mapping من Domain.

حالة التنفيذ: Foundation وFull Schema migrations تعملان من قاعدة فارغة، ونجحت اختبارات cross-tenant references والتكرار الأساسي وUnresolved Event وOutbox Lease وSnapshots. شغّلنا Runner مرتين ونتج `applied=27` ثم `applied=0`. المرحلة التالية هي تنفيذ PostgreSQL Adapter لا إعادة تصميم Schema.

## المرحلة التالية: PostgreSQL Adapter وReliability Foundation

نطبق:

```text
Event Ledger
→ Idempotency
→ Outbox
→ Asynq Worker
→ Retry/Backoff
→ Dead Letter
→ Reconciliation
```

Inbound event يحفظ أولًا. Outbound intent يحفظ قبل enqueue. UNKNOWN لا يعاد إرساله تلقائيًا.

معيار النجاح: اختبار crash بين كل خطوتين لا ينشئ Customer أو Conversation أو Message ثانية ولا يفقد event.

## المرحلة 4: Provider Simulator

ينفذ FakeSocialProvider وFakeCommunicationWorkspace سيناريوهات:

```text
success
duplicate
out-of-order
rate-limit
timeout
unknown-result
permanent-failure
delivery-status
```

معيار النجاح: Vertical Slice تعمل دون credentials حقيقية، وتنتج logs/audit/reconciliation قابلة للفحص.

## المرحلة 5: Adapters

ننفذ SocialAPI Adapter حسب عقد Provider، ثم Chatwoot Workspace Adapter. لا نضع adapter code داخل Domain.

SocialAPI مسؤول عن inbound/outbound/delivery الخارجي. Chatwoot مسؤول عن mirror وworkspace operations. Mujeeb Go يحتفظ بالحالة والعلاقات والـSales truth.

## المرحلة 6: Vertical Slice

```text
SocialAPI inbound
→ verify
→ persist event
→ idempotency
→ tenant resolution
→ identity resolution
→ conversation mapping
→ Chatwoot mirror
→ outbound intent
→ SocialAPI send
→ delivery update
```

لا نضيف AI auto-reply في أول Vertical Slice. أولًا نثبت النقل والمزامنة وعدم الفقد.

## المرحلة 7: AI Context وIntent

بعد نجاح النقل، نبني Context من Evidence موثقة وننتج Structured AIDecision. لا يملك Model side effects مباشرة.

## المرحلة 8: Catalog وLead وTransaction

بعد AI contract والتنفيذ الآمن، نضيف CatalogItem وOffer وVariant ثم Lead وCommercialTransaction بأنواع Order/Booking/Appointment/Service Request/Quote.

## ترتيب العمل في Pull Requests التالية

```text
PR-005: SQL migrations 000001–000012 + Foundation constraint tests — مكتمل
PR-006: SQL migrations 000013–000027 + full-schema tests/review — مكتمل
PR-007: PostgreSQL Adapter and TransactionManager — التالي
PR-008: Event ledger/idempotency/outbox implementation
PR-009: Provider simulator
PR-010: Chatwoot/SocialAPI adapters
PR-011: First vertical slice
PR-012: AI Context/Intent/Decision
```

تصميم Domain وPorts وPersistence مغلق، لكن كل PR يجب أن يذكر العقد الذي يطبقه والاختبارات التي تثبت invariants الخاصة به.
