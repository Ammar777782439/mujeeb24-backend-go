# خارطة التنفيذ — Mujeeb 24 Backend Go

## المرحلة الحالية: Domain Contract

الحالة: **مغلقة تصميميًا، غير منفذة ككود**.

العقود الموجودة في `contracts/` تحدد shared وbusiness وchannel وidentity وcommunication وcatalog وsales وai وaudit. لا نكتب Repositories أو HTTP DTOs قبل مراجعتها.

## المرحلة 1: Application Ports

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

## المرحلة 2: SQL Schema

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

معيار النجاح: migrations تعمل من قاعدة فارغة، وrollback/forward policy واضحة، وconstraints تمنع cross-tenant references والتكرار الأساسي.

## المرحلة 3: Reliability Foundation

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

## ترتيب العمل في أول Pull Requests

```text
PR-001: Domain shared Value Objects + invariant tests
PR-002: Domain business + channel contracts as Go types
PR-003: Identity + communication contracts as Go types
PR-004: Application ports
PR-005: SQL migrations and transaction boundaries
PR-006: Event ledger/idempotency/outbox
PR-007: Provider simulator
PR-008: Chatwoot/SocialAPI adapters
PR-009: First vertical slice
PR-010: AI Context/Intent/Decision
```

كل Pull Request يجب أن يذكر العقد الذي يطبقه والاختبارات التي تثبت invariants الخاصة به.
