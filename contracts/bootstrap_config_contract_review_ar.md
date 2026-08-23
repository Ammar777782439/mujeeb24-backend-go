# مراجعة المدير — Bootstrap وConfig Contract

## الحكم التنفيذي

التحليل المرفق **صحيح ومتوافق مع Baseline وPorts وPersistence Boundary**. وهو يغلق كيفية تركيب النظام في Go دون تسرب dependencies أو تكرار Business Logic.

أعتمد الفكرة المركزية:

```text
cmd
  → bootstrap
      → application ports/adapters
          → application services
              → domain
```

لكن التنفيذ يجب أن يأتي بعد تثبيت Go Ports وPersistence Model؛ ما نغلقه الآن هو **Composition Contract** وليس كود التشغيل الكامل.

## ما نعتمده

| القرار | الحكم |
|---|---|
| Bootstrap هو Composition Root | معتمد |
| `cmd/*/main.go` نحيف | معتمد |
| لا Service Locator عالمي | معتمد |
| Constructor Injection | معتمد |
| API وWorker يستخدمان Application Services نفسها | معتمد |
| Provider SDK لا يدخل Application | معتمد |
| Config لا يحمل Business/Customer runtime data | معتمد |
| SecretStore منفصل عن Config | معتمد |
| Redis/Asynq ليسا Source of Truth | معتمد |
| Reconciler Job داخل Worker في V1 | معتمد |
| `cmd/migrate` مستقل ولا يشغل باقي الخدمات | معتمد |
| Liveness وReadiness وProvider Health منفصلة | معتمد |
| Graceful Shutdown وDrain | معتمد |
| لا DI Framework ثقيل | معتمد |

## التصحيحات الإلزامية

### 1. Bootstrap لا يملك Business-scoped connections

`BuildDependencies` ينشئ Clients وAdapters عامة مثل SocialAPI Client وChatwoot Client، لكنه لا يحمل كل `ChannelConnection` للتجار ولا يضع access tokens داخل Composition Container.

الاتصالات التجارية تُقرأ من PostgreSQL، والـSecret يُستدعى عبر SecretStore وقت Use Case/Worker أو عبر cache محكوم لاحقًا.

```text
Process Config
  → Provider Client/Adapter

Business ChannelConnection
  → Application/Repository
  → SecretReference
  → SecretStore
```

### 2. Config ثلاث طبقات لا طبقة واحدة

نميز بين:

```text
ProcessConfig
= كيف يعمل البرنامج؟

Business Configuration
= كيف يعمل التاجر وسياساته؟

Secret Material
= credentials الفعلية
```

الأولى تُحمّل عند startup. الثانية Domain/Application state. الثالثة لا تظهر في Config العادي أو logs أو Domain.

### 3. Optional Provider Config لا يكسر Core بلا سبب

إذا كانت SocialAPI أو Chatwoot أو AI Integration مفعلة في بيئة الإنتاج، يجب التحقق من إعداداتها. أما غياب integration غير مفعلة فلا يجب أن يمنع تشغيل Core أو migrations.

الـReadiness لا يختزل في وجود كل Provider؛ بل يوضح:

```text
core readiness: database + required queue
provider health: per connection/provider
```

### 4. Readiness وProvider Health منفصلان

تعطل SocialAPI لا يعني أن عملية Go ماتت. يستطيع النظام حفظ Inbound Events وOutbox ورفع حالة `degraded` وإجراء Reconciliation.

لكن إذا كانت وظيفة أساسية لا يمكن قبولها بدون Chatwoot أو Queue، يجب أن يحدد Application ذلك على مستوى Use Case، لا أن ينهار process كله تلقائيًا.

### 5. Dependencies typed وليس Container ديناميكيًا

نستخدم Structs typed في Composition Root:

```text
Dependencies
├── Config
├── Runtime
├── Repositories
├── Ports
└── Application Services
```

لا نستخدم `Get("leadService")` ولا reflection ولا global singleton. كل service يأخذ dependencies اللازمة فقط عبر constructor.

### 6. Bootstrap بعد Ports وPersistence Contract

يمكن كتابة وثيقة Bootstrap الآن، لكن التنفيذ الفعلي يجب أن يتبع:

```text
Domain Contracts
→ Application Ports
→ Persistence/Application Boundary
→ Persistence Model Contract
→ Bootstrap implementation
```

لأن Bootstrap يحتاج constructors ثابتة، ولا نريد إعادة wiring عند اكتشاف أن Event Ledger وOutbox يجب أن يشتركا في transaction.

### 7. API وWorker لا يملكان Business Logic منفصلًا

HTTP Handler وAsynq Handler هما entrypoints فقط. كلاهما يستدعي Application Command/Service نفسه. Worker يقرأ Outbox ويعيد فحص الحالة قبل أي side effect.

```text
HTTP / Worker
      ↓
Application Use Case
      ↓
Domain + Ports
```

لا يوجد `HTTP Lead Service` مختلف عن `Worker Lead Service`.

### 8. HTTP/Webhook Handler لا ينفذ AI أو Provider call

الـHandler يتحقق من الحجم/التوقيع ويحول الطلب إلى Application Command. Webhook Ingestion يحفظ Event Ledger ويعيد ACK بعد الحفظ الدائم. AI وSocialAPI وChatwoot تعمل في Application/Workers، لا داخل request handler طويل.

### 9. Shutdown Order

نعتمد:

```text
Stop accepting HTTP
→ Stop scheduling new jobs
→ Drain in-flight handlers
→ Close Asynq client/server
→ Close provider HTTP clients
→ Close Redis
→ Close PostgreSQL
→ Exit
```

كل خطوة يجب أن تملك timeout وإشارة فشل واضحة، ولا نقتل DB قبل إنهاء العاملين الذين يستخدمونها.

### 10. cmd/migrate أقل Runtime ممكن

`cmd/migrate` يحمّل ProcessConfig اللازمة لـPostgreSQL فقط، ويتحقق من الاتصال ويطبق versioned migrations. لا ينشئ Redis أو Asynq أو SocialAPI أو Chatwoot أو AI أو Application Services.

### 11. Bootstrap لا يخفي lifecycle

`api.go` يبني Router وHandlers وHealth endpoints، و`worker.go` يبني Queue Server وHandlers. كل runtime يملك start/stop واضحًا، ويمكن اختبار wiring دون تشغيل integrations حقيقية باستخدام fakes.

## Runtime Composition النهائي

```text
cmd/api
  → bootstrap.BuildAPI
      → HTTP Router
          → Auth/Tenant/Request-ID Middleware
              → Application Commands/Queries

cmd/worker
  → bootstrap.BuildWorker
      → Asynq Handlers
          → Application Workers/Services

cmd/migrate
  → ProcessConfig
      → PostgreSQL
          → Versioned migrations
```

والـAdapters تُركب في Composition Root:

```text
SocialAPI Config → SocialAPI Client → ChannelProvider
Chatwoot Config  → Chatwoot Client  → CommunicationWorkspace
AI Config        → AI Client        → AIService
Postgres         → Repositories/EventStore/Outbox
```

## معايير الإغلاق

1. لا Provider SDK داخل Domain أو Application.
2. لا Business data داخل Process Config.
3. لا secret material داخل logs أو DTOs أو Domain.
4. لا Global dependencies أو Service Locator.
5. API وWorker يستعملان Use Cases نفسها.
6. `cmd/migrate` مستقل.
7. Core readiness منفصل عن Provider health.
8. Shutdown يوقف العمل قبل إغلاق الموارد.
9. Bootstrap لا يملك tenant connections؛ Application تملك runtime resolution.
10. يمكن بناء Runtime باستخدام fakes للاختبار.

## القرار

**أعتمد تحليل الذكاء الاصطناعي بعد هذه التصحيحات.** أصبح Bootstrap وConfig Contract مغلقًا على مستوى التصميم، ولم نكتب implementation بعد.

الخطوة التالية المنطقية هي `Persistence Model Contract`: تحويل كل Aggregate وLedger وOutbox إلى جدول وعلاقات وTenant Constraints وIndexes قبل كتابة SQL migrations الفعلية.
