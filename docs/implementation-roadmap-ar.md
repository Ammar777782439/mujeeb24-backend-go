# خارطة التنفيذ — Mujeeb 24 Backend Go

## المرحلة الحالية: PR-009 — PostgreSQL Adapter وTransactionManager

الحالة: **PR-008 مغلق عند HTTP → Application façade 76/76؛ PR-009 يملك الآن PostgreSQL pool وTransactionManager وRepository foundation الأساسية وCommunicationMessage وChannel Capabilities وCatalog read persistence، لكنه ما زال مفتوحًا قبل Catalog writes وSales/AI/Audit repositories وEventStore/Outbox وbootstrap wiring.**

SQL migrations من 000001 إلى 000028، مع 000028 كـforward migration لـCommunicationMessage. Full Schema constraint tests وmigration runner تحققت فعليًا على PostgreSQL 16 (`applied=28` ثم `applied=0`). عقد HTTP Dashboard V1 موجود في `contracts/http_api_dashboard_v1_contract_ar.md`. Go Request/Response DTOs داخل `internal/adapters/primary/http/dto` مع operation registration داخل `internal/adapters/primary/http/contract` هي مصدر الحقيقة المشترك، وHuma يولد OpenAPI 3.0.3 إلى `api/openapi/mujeeb24-dashboard-v1.generated.yaml`. يمنع `scripts/check-openapi-generated.sh` drift ويعمل في CI.

الـHuma registration يولد العقد من DTOs الموجودة في `http/dto` عبر operation registration في `http/contract`. يحتوي `handlers` على dispatcher موحد وfaçade typed لكل الـ76 operation؛ كل façade تبني Command/Query أو system contract وتستدعي dependency typed، وقد تعيد `not_implemented` من Application boundary عند غياب التنفيذ الداخلي. في PR-009 أصبحت repositories الأساسية و`MessageRepository.Record/ListByConversation` منفذة ومثبتة؛ لا يوجد Provider call في هذه الدفعة.

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

## المرحلة المنجزة: SQL Schema

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

حالة التنفيذ: Foundation وFull Schema migrations من 000001 إلى 000027 بقيت دون تعديل، وأضيفت 000028 لـCommunicationMessage كـforward migration. نجحت اختبارات cross-tenant references وmessage constraints وmessage timeline integration وmigration runner؛ التوقع المثبت هو `applied=28` ثم `applied=0`.

## المرحلة المنفذة: HTTP Application façades، والمرحلة الحالية PostgreSQL foundation

نطبق:

```text
Generated DTO Contract
→ runtime dispatcher لكل routes
→ typed Application façade لكل Command/Query
→ Auth/Tenant/Error/Metadata tests
→ PostgreSQL pool + TransactionManager ✅
→ Repository foundation + CommunicationMessage timeline ✅
→ Channel Capabilities ✅
→ Catalog read persistence ✅
→ Catalog write contract/repositories ⏳
→ Sales/AI/Audit repositories ⏳
→ Event Ledger + atomic inbound dedupe ⏳
→ Outbox ⏳
→ Asynq Worker
```

Inbound event يحفظ أولًا. Outbound intent يحفظ قبل enqueue. UNKNOWN لا يعاد إرساله تلقائيًا.

معيار النجاح: كل Handler يمرر DTO إلى Command/Query typed ولا يحتوي SQL أو Provider call، واختبارات HTTP تثبت Auth/Tenant/Error/Idempotency/Concurrency boundaries.

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
PR-009 message correction: 000028 communication_messages + Record/ListByConversation + integration tests — مرفوع ضمن commit سابق
PR-007: HTTP API Contract → Go DTOs + generated OpenAPI v1 + drift check — مكتمل
PR-008: Typed Commands/Queries + dispatcher وfaçades typed لكل routes — مكتمل
PR-009: PostgreSQL Adapter وTransactionManager وRepository foundation — قيد التنفيذ؛ CommunicationMessage وChannel Capabilities وCatalog read persistence منفذة ومثبتة، والآن يجب حسم Catalog write contract ثم تنفيذها قبل Sales/AI/Audit وReliability/bootstrap
PR-010: Event ledger/idempotency/outbox implementation
PR-011: Provider simulator
PR-012: Chatwoot/SocialAPI adapters
PR-013: First vertical slice
PR-014: AI Context/Intent/Decision
```

تصميم Domain وPorts وPersistence مغلق، لكن كل PR يجب أن يذكر العقد الذي يطبقه والاختبارات التي تثبت invariants الخاصة به.
