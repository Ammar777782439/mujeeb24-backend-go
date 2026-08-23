# مراجعة المدير — Application Ports Contract

## الحكم التنفيذي

التحليل المرفق **صحيح في الاتجاه العام ومتوافق مع Baseline وDomain Contracts**. وهو يضع السؤال الصحيح:

> ما الذي تحتاجه Application من العالم الخارجي؟

لكن لا نعتمد الواجهات كما كُتبت حرفيًا؛ لأنها ما زالت أمثلة مفاهيمية وتحتوي أربع فجوات تشغيلية خطيرة: **Inbound normalization، OAuth lifecycle، Atomic Outbox، وWorkspace References**.

القرار: نعتمد قائمة Ports، ثم نصحح عقودها قبل كتابة Go interfaces النهائية.

## ما نعتمده

| Port/Boundary | الحكم |
|---|---|
| `ChannelProvider` مستقل عن SocialAPI وMeta | معتمد |
| Webhook HTTP يدخل من Primary Adapter | معتمد |
| Provider DTOs لا تدخل Ports | معتمد |
| `CommunicationWorkspace` بدل `ChatwootPort` | معتمد |
| Repositories حسب Aggregate/Use Case | معتمد |
| `EventStore` ليس Event Bus | معتمد |
| `Outbox` مختلف عن Event Ledger | معتمد |
| `AIService` يرجع Structured AIDecision | معتمد |
| `Clock` Port للاختبارات | معتمد |
| `SecretStore` لا يكشف السر للـDomain | معتمد |
| `TransactionManager` مختلف عن CommercialTransaction | معتمد |
| عدم إضافة Cache/Notification/Availability/EventBus الآن | معتمد مبدئيًا |

## التصحيحات الإلزامية

### 1. ChannelProvider.Connect واسع أكثر من اللازم

الاتصال الخارجي عبر OAuth أو provisioning قد يكون غير متزامنًا. لذلك لا نثبت:

```text
Connect() → ChannelConnection
```

كأنه عملية واحدة دائمًا. الأفضل مفاهيميًا:

```text
BeginConnection(request) → ConnectionAuthorization
CompleteConnection(callback/credentials) → ConnectionProvisioned
Disconnect(request) → DisconnectResult
```

Application تملك `ChannelConnection` وتغير حالتها. Adapter يعيد نتيجة محايدة مثل:

```text
ConnectionProvisioned
├── provider_ref
├── channel
├── external_account_ref
├── capabilities
├── secret_reference
└── health
```

لا ينشئ Adapter Aggregate Mujeeb من تلقاء نفسه.

### 2. نحتاج Port للـInbound Provider Decoding

التحليل محق أن Webhook ليس `ChannelProvider.ReceiveWebhook`. لكن إذا تركناها خارج Ports تمامًا فسيتسرب SocialAPI DTO إلى HTTP Handler أو Application.

نضيف عقدًا محايدًا صغيرًا:

```text
InboundEventDecoder
├── VerifyAndDecode(ctx, request) → NormalizedInboundEvent
```

Primary HTTP Adapter يملك HTTP request/response، وProvider-specific Decoder يملك signature verification وSocialAPI JSON normalization. Application يستقبل `NormalizedInboundEvent` فقط.

هذا ليس Webhook business logic؛ هو حد تحويل Provider Transport إلى Application Contract.

### 3. SendMessage لا يستخدم Internal Connection ID وحده

Application يمكنها تحميل ChannelConnection، لكن Port request يجب أن يحمل descriptor محددًا:

```text
SendMessageRequest
├── connection_reference
├── channel
├── recipient_reference
├── conversation_reference?
├── content
├── idempotency_key
└── correlation_id
```

الـAdapter لا يذهب إلى Repository ليبحث عن Connection؛ ولا يعرف Mujeeb database.

`idempotency_key` ينتمي إلى Outbound intent ويجب أن يكون deterministic لكل إرسال منطقي.

### 4. SendMessageResult يحتاج semantics واضحة

```text
SendMessageResult
├── outcome: accepted | sent | unknown | failed
├── provider_message_reference?
├── accepted_at?
├── failure_class?
├── retryable?
└── provider_status_reference?
```

لا نعيد HTTP response أو Provider DTO. `unknown` لا يعني `failed`، ولا يسمح بإعادة إرسال أعمى.

### 5. CommunicationWorkspace لا يعيد Mujeeb Conversation بالخطأ

`EnsureConversation` يجب أن يعيد Workspace Reference، وليس Domain Conversation.

```text
WorkspaceContactReference
WorkspaceConversationReference
WorkspaceMessageReference
```

كل Reference يحمل system/provider/resource type/resource id. Application هي التي تربط هذه المراجع بـMujeeb `ConversationReference`.

العقود يجب أن تكون idempotent:

```text
EnsureContact
EnsureConversation
CreateMessage
Assign
AddLabel
UpdateConversation
```

لا تكتفي عمليات Assign/Label/Update بـ`error` فقط؛ الأفضل أن تعيد نتيجة محايدة أو reference/status حتى نتمكن من reconciliation والتدقيق.

### 6. نحتاج Inbound Mirror وOutbound Mirror بوضوح

`CreateMessage` يجب أن يميز:

```text
direction: inbound | outbound
origin: customer | ai | human | automation | system
private: boolean
source_reference
```

Chatwoot Adapter لا يقرر هل الرسالة ستُرسل إلى SocialAPI. Application/Orchestration يقرر، وChatwoot هنا Workspace mirror/operation.

### 7. EventStore يحتاج RecordIfAbsent وLease

`Exists` ثم `Record` عمليتان منفصلتان قد تسببان race condition. العقد الأفضل:

```text
RecordIfAbsent(ctx, event) → {created: bool, record: EventRecord}
Claim(ctx, event_id, lease) → ClaimResult
MarkProcessed(ctx, event_id, result)
MarkRetryableFailure(ctx, event_id, failure)
MoveToDeadLetter(ctx, event_id, reason)
```

`MarkProcessing` بلا lease/ownership قد يسمح لعاملين بمعالجة الحدث نفسه. Event Ledger هو المصدر الدائم، وAsynq مجرد منفذ.

لا نضيف `IdempotencyStore` منفصلًا في V1 إذا كان EventStore يملك العملية الذرية `RecordIfAbsent` للأحداث الواردة. أما Outbound فله idempotency key مستقل.

### 8. Outbox يجب أن يكون Atomic مع Business Transaction

لا نريد:

```text
Save Aggregate
commit
Outbox.Enqueue منفصل
```

لأن العملية قد تنجح في DB وتفشل قبل enqueue.

العقد المطلوب:

```text
WithinTransaction(ctx, func(txCtx) error {
    save aggregate
    append event/outbox record
    return nil
})
```

الـOutbox record يملك:

```text
id
command_type
aggregate_reference
dedupe_key
payload_reference
status
attempt_count
lease_reference?
next_attempt_at?
```

`Claim` يجب أن يمنح lease أو ownership token، و`Complete/Fail` يتحققان من lease حتى لا يكتب Worker قديم فوق Worker جديد.

### 9. Outbox وEventStore ليسا Queue API فقط

`Outbox.Enqueue/Claim/MarkCompleted` مفهومة، لكن لا نضع Redis keys أو Asynq task payload في Port. كما لا نضع retry algorithm داخل Domain.

Application تحدد obligation؛ Adapter يكتبها في PostgreSQL ويشغلها عبر Asynq. Dead Letter سجل دائم وليس مجرد Redis queue.

### 10. Repository contracts لا تكون CRUD عامة

نقبل Repository لكل Aggregate، لكن لا نعتمد واجهات `Get/Save/Update` بلا Use Case. أمثلة:

```text
ExternalIdentityRepository.FindByConnectionAndExternalUser
ConversationReferenceRepository.FindCurrentByExternalResource
LeadRepository.FindOpenByCustomerAndOffering
```

`AuditRepository` ينبغي أن يكون `AuditWriter.Append` append-only، لا Update/Delete.

`TransactionRepository` يجب ألا يختلط مع `TransactionManager`؛ الأول CommercialTransaction، والثاني Unit of Work.

### 11. AIService عقده صحيح، لكن Request يجب أن يكون Context جاهزًا

لا نرسل Conversation كاملة أو Business database إلى AI Adapter. `AIDecisionRequest` يحتوي Context مفلترًا وEvidence وPolicy وexpiry. AI Adapter يعيد Structured AIDecision، ولا ينفذ Action.

### 12. SecretStore يحتاج سياسة وصول

```text
Get(ctx, SecretReference) → SecretMaterial
Put(ctx, SecretMaterial) → SecretReference
Delete(ctx, SecretReference)
```

لكن `SecretMaterial` لا يظهر في logs أو Domain أو Audit. Application تستعمله فقط في boundary الذي يحتاج provider call. لا نضع `Secret` في DTO أو repository model عادي.

### 13. TransactionManager يجب ألا يكون Closure عامة بلا قيود

الفكرة صحيحة، لكن يجب أن تضمن أن Repository operations تستخدم نفس transaction context. نحتاج لاحقًا أحد النموذجين:

```text
UnitOfWork.Within(ctx, func(uow UnitOfWorkHandle) error)
```

أو context-bound transaction adapter مع قواعد واضحة.

لا نكتب SQL قبل تثبيت أن Event Ledger وOutbox وAggregate writes تشترك في transaction عندما يتطلبها Use Case.

## قائمة Ports المعتمدة مبدئيًا

```text
application/ports/
├── channel_provider.go
├── inbound_event_decoder.go
├── communication_workspace.go
├── repositories.go
├── event_store.go
├── outbox.go
├── ai_service.go
├── clock.go
├── secret_store.go
└── transaction.go
```

لا نضيف الآن:

```text
availability_provider.go
cache.go
notification.go
event_bus.go
```

حتى يظهر Use Case حقيقي واختبار يبررها.

## Dependency Rule

```text
Primary HTTP/Webhook Adapter
        ↓
Application Service
        ↓
Application Port
        ↓
Secondary Adapter
```

وفي inbound:

```text
Provider Webhook
  → Primary HTTP transport
  → InboundEventDecoder
  → NormalizedInboundEvent
  → Inbound Application Service
```

وفي outbound:

```text
Application Action
  → Outbox/Worker
  → ChannelProvider.SendMessage
  → SocialAPI Adapter
```

وفي mirror:

```text
Application Orchestrator
  → CommunicationWorkspace
  → Chatwoot Adapter
```

## القرار

**أعتمد تحليل الذكاء الاصطناعي بعد هذه التصحيحات.** لا نكتب Go interfaces النهائية حتى نغلق Request/Result types وtransaction semantics في هذه المراجعة.

الخطوة التالية ليست SQL مباشرة؛ بل كتابة `Persistence/Application Boundary Contract` يغلق atomicity بين Aggregate وEvent Ledger وOutbox، ثم نكتب Go Ports كترجمة حرفية للعقود.
