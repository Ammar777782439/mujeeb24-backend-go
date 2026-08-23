# Baseline النهائي لـ Mujeeb 24 Backend Go

## قرار المدير

الرد الأخير هو **الأدق تنظيميًا** لمجيب 24. سنعتمد بنيته الأساسية، وليس Hybrid Architecture السابقة التي وسعت الطبقات أكثر من اللازم.

السبب أن المشروع Modular Monolith في مرحلته الحالية، ويحتاج وضوحًا لا عددًا كبيرًا من الحدود المتداخلة.

```text
adapters/primary/http
          ↓
application/{ports,commands,queries,services,workers}
          ↓
domain
          ↑
adapters/secondary/{providers,workspaces,persistence,queue,ai}
```

`bootstrap` يركب الاعتماديات، و`platform` يحتوي المكونات التقنية المشتركة. لا نحتاج الآن إلى طبقة مستقلة باسم `transport` وطبقة مستقلة باسم `infrastructure` وطبقة مستقلة باسم `jobs` إذا كانت ستكرر نفس المسؤوليات.

## الهيكل المعتمد

```text
mujeeb24-backend-go/
├── cmd/
│   ├── api/main.go
│   ├── worker/main.go
│   └── migrate/main.go
│
├── internal/
│   ├── bootstrap/
│   │   ├── dependencies.go
│   │   ├── api.go
│   │   └── worker.go
│   │
│   ├── domain/
│   │   ├── shared/
│   │   ├── business/
│   │   ├── channel/
│   │   ├── identity/
│   │   ├── communication/
│   │   ├── catalog/
│   │   ├── sales/
│   │   ├── ai/
│   │   └── audit/
│   │
│   ├── application/
│   │   ├── ports/
│   │   ├── commands/
│   │   ├── queries/
│   │   ├── services/
│   │   └── workers/
│   │
│   ├── adapters/
│   │   ├── primary/
│   │   │   └── http/
│   │   │       ├── handlers/
│   │   │       ├── dto/
│   │   │       └── middleware/
│   │   └── secondary/
│   │       ├── providers/socialapi/
│   │       ├── workspaces/chatwoot/
│   │       ├── persistence/postgres/
│   │       ├── queue/asynq/
│   │       ├── ai/
│   │       ├── storage/
│   │       ├── secrets/
│   │       └── observability/
│   │
│   └── platform/
│       ├── config/
│       ├── database/
│       ├── httpserver/
│       └── lifecycle/
│
├── migrations/
├── contracts/
├── api/openapi/
├── tests/
├── docs/
├── scripts/
├── Dockerfile
├── docker-compose.local.yaml
├── Makefile
├── README.md
├── go.mod
└── go.sum
```

## التعديلات الستة المعتمدة

أولًا، تبقى Ports تحت `application/ports`؛ لأن Use Cases هي التي تحدد العقود التي تحتاجها. لا نحتفظ بمجلد `internal/ports` بالتوازي.

ثانيًا، يبقى `adapters/primary` لكل ما يدخل إلى النظام، مثل Dashboard HTTP وWebhook HTTP. ويبقى `adapters/secondary` لكل ما يعتمد عليه النظام، مثل SocialAPI وChatwoot وPostgreSQL وAsynq وAI.

ثالثًا، `domain/communication` لا يمثل Chatwoot كاملًا. يحتوي فقط على `ConversationReference` و`SalesContext` وربما `CommunicationMessage` كمرجع داخلي محدود. لا ننشئ Team أو Label أو Inbox كـSales Entities.

رابعًا، `assignment` في Domain لا يمثل Team في Chatwoot؛ إن احتجناه فهو `AssignmentReference` أو `OwnershipState` فقط.

خامسًا، `application/services` مسموح فقط لخدمات Orchestration واضحة. لا نضع Service عملاقًا يفعل كل شيء؛ فـ`InboundIngestionService` يستقبل ويحفظ، و`InboundProcessingService` يعالج، و`IdempotencyService` يملك سياسة التكرار، و`OutboxService` يملك إنشاء وإدارة أوامر الخروج.

سادسًا، Workers ليست مكان Business Logic. Worker يستلم Job ويستدعي Application Service. نضع reconciliation داخل worker في البداية، ونفصل `cmd/reconciler` لاحقًا فقط إذا احتاج التشغيل ذلك فعليًا.

## ملكية الأنظمة

| المكوّن | الملكية |
|---|---|
| Mujeeb Domain | Business، Customer Context، Catalog، Intent، AI Decision، Lead، Transactions، Automation، Subscription |
| Chatwoot Adapter | Contacts/Conversations/Messages/Assignments كمراجع وعمليات Workspace فقط |
| SocialAPI Adapter | الاتصال بالقنوات، Webhooks، الإرسال الخارجي، حالات التسليم |
| PostgreSQL Adapter | التخزين الدائم والمعاملات وEvent Ledger وOutbox |
| Asynq Adapter | تشغيل Jobs وإعادة المحاولة المسرّعة، وليس مصدر الحقيقة الوحيد |
| Dashboard API | الواجهة الوحيدة للتاجر عبر Mujeeb API |

## ترتيب التنفيذ

```text
1. domain + application/ports
2. bootstrap + config
3. SQL migrations
4. Event Ledger + Idempotency + Outbox
5. SocialAPI Adapter + Provider Simulator
6. Chatwoot Adapter
7. Primary HTTP/Webhook handlers
8. Vertical Slice inbound → mirror → outbound → status
9. AI Context + Intent + Decision
10. Catalog / Lead / Commercial Transactions
```

## ما لا نعتمده

لا نعود إلى `yemen-social-reply-engine` القديم كقاعدة كود. لا ننقل Facebook/Meta Prototype إلى النواة. لا ننشئ `internal/ports` بجانب `application/ports`. لا ننشئ `transport` بجانب `adapters/primary/http` لنفس HTTP. لا ننشئ `infrastructure` بجانب `adapters/secondary` لنفس PostgreSQL وRedis. ولا ننشئ عشرات الملفات البرمجية الفارغة؛ المجلدات التأسيسية تستخدم `.gitkeep` فقط إلى أن يأتي تنفيذ حقيقي.

## القرار النهائي

هذا هو **Baseline المعتمد**. بعده لا نغيّر الشجرة بسبب اقتراحات عامة؛ أي تعديل يجب أن يحل مشكلة حقيقية ظهرت من كود أو اختبار أو تشغيل. الخطوة التالية هي فحص Domain ملفًا ملفًا، ثم كتابة عقود Ports وMigrations وProvider Simulator قبل إضافة AI أو واجهات تجميلية.
