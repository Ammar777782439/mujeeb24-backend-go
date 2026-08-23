# قرار المدير: مقارنة هيكل Backend Go المقترح

## الحكم الحاسم

الهيكل المرفق من الذكاء الاصطناعي **أفضل من الهيكل الأول الذي أنشأناه من ناحية التنظيم التجاري والتشغيلي**، وسنعتمد فكرته الأساسية. لكنه لا يُنسخ حرفيًا؛ لأن نسخ كل المجلدات والملفات المقترحة الآن سيصنع مشروعًا ضخمًا أغلبه فارغ.

سنعتمد **Hybrid Architecture** تجمع وضوح الهيكل المقترح مع انضباط الهيكل الأول:

```text
internal/
├── domain/          # Business rules only
├── application/     # Use cases by business capability
├── ports/           # Contracts for external dependencies
├── adapters/        # SocialAPI, Chatwoot, AI adapters
├── transport/       # HTTP and webhook ingress
├── infrastructure/  # Postgres, Redis, queue, secrets, telemetry
├── jobs/             # Runtime workers and scheduled jobs
└── platform/        # Technical cross-cutting helpers
```

## لماذا الهيكل المرفق أقوى؟

الهيكل المقترح يفصل بوضوح بين `transport` و`adapters` و`infrastructure`، ويضع `jobs` خارج HTTP، ويضيف `reconciler` مستقلًا، وينظم `application` حسب قدرات العمل مثل inbound وoutbound وcatalog وleads وtransactions. هذه نقاط مهمة في منتج إنتاجي متعدد القنوات وليست CRUD عاديًا.

كما أنه يصحح نقطة مهمة: **Chatwoot ليس Sales Domain**. لذلك لا ننشئ داخل Domain كيانات مثل Chatwoot Message أو Inbox أو Team، بل نحتفظ بمراجع تشغيلية فقط.

## ما الذي نعدله في الهيكل المقترح؟

| النقطة | القرار |
|---|---|
| اسم المشروع القديم `yemen-social-reply-engine` | لا نعيد استخدامه؛ المستودع الجديد هو `mujeeb24-backend-go` |
| `internal/ports` | نعتمده بدل وضع Ports داخل `application/ports` لتكون الحدود ظاهرة |
| `internal/transport` | نعتمده منفصلًا عن adapters؛ فيه HTTP وWebhook ingress فقط |
| `internal/infrastructure` | نعتمده للتقنيات: PostgreSQL، Redis، Queue، Secrets، Auth، Observability |
| `internal/adapters` | نعتمده لمترجمات SocialAPI وChatwoot وAI فقط |
| `internal/jobs` | نعتمده للـworkers؛ أما منطق القرار فيبقى Application Service |
| `cmd/reconciler` | نعتمده كبرنامج مستقل، ويمكن تشغيله مبدئيًا ضمن worker عند الحاجة |
| `domain/communication/message` | لا نعتمده كـCommunication Truth؛ Chatwoot يملك ذلك |
| `domain/customer` و`domain/identity` | نعتمدهما معًا؛ Customer هو كيان Mujeeb وIdentity هي هوية القناة الخارجية |
| `application/commands/queries/services` | نستبدله بتنظيم حسب capability: inbound/outbound/catalog/leads/transactions/ai |
| `configs` و`deployments` | نضيفهما لإعدادات التشغيل وDocker/deployment، دون أسرار |

## الهيكل النهائي المعتمد

```text
mujeeb24-backend-go/
├── cmd/
│   ├── api/main.go
│   ├── worker/main.go
│   ├── reconciler/main.go
│   └── migrate/main.go
│
├── internal/
│   ├── domain/
│   │   ├── business/
│   │   ├── tenant/
│   │   ├── identity/
│   │   ├── customer/
│   │   ├── conversation/
│   │   ├── channel/
│   │   ├── catalog/
│   │   ├── lead/
│   │   ├── transaction/
│   │   ├── ai/
│   │   ├── automation/
│   │   ├── notification/
│   │   ├── subscription/
│   │   └── shared/
│   │
│   ├── application/
│   │   ├── business/
│   │   ├── channels/
│   │   ├── inbound/
│   │   ├── outbound/
│   │   ├── conversations/
│   │   ├── ai/
│   │   ├── catalog/
│   │   ├── leads/
│   │   ├── transactions/
│   │   ├── automation/
│   │   ├── analytics/
│   │   ├── notifications/
│   │   └── subscriptions/
│   │
│   ├── ports/
│   │   ├── social/
│   │   ├── communication/
│   │   ├── ai/
│   │   ├── persistence/
│   │   ├── events/
│   │   ├── idempotency/
│   │   ├── delivery/
│   │   ├── secrets/
│   │   ├── cache/
│   │   ├── clock/
│   │   ├── ids/
│   │   └── observability/
│   │
│   ├── adapters/
│   │   ├── socialapi/
│   │   ├── chatwoot/
│   │   └── ai/
│   │
│   ├── transport/
│   │   ├── http/
│   │   └── webhooks/
│   │       ├── socialapi/
│   │       └── chatwoot/
│   │
│   ├── infrastructure/
│   │   ├── postgres/
│   │   ├── redis/
│   │   ├── queue/
│   │   ├── storage/
│   │   ├── secrets/
│   │   ├── auth/
│   │   ├── observability/
│   │   └── config/
│   │
│   ├── jobs/
│   │   ├── inbound/
│   │   ├── outbound/
│   │   ├── chatwoot/
│   │   ├── ai/
│   │   ├── automation/
│   │   └── maintenance/
│   │
│   └── platform/
│       ├── httpclient/
│       ├── retry/
│       ├── backoff/
│       ├── errors/
│       ├── validation/
│       ├── security/
│       ├── telemetry/
│       └── lifecycle/
│
├── migrations/
├── api/openapi/
├── configs/
├── deployments/
├── scripts/provider-simulator/
├── tests/{contract,integration,fixtures}
├── docs/
├── go.mod
├── go.sum
└── Makefile
```

## ما لا نفعله

لا ننشئ Agent منفصلًا لكل قطاع. لا نضع منطقًا في HTTP handlers. لا نجعل `infrastructure` يعرف Sales decisions. لا نعرض SocialAPI أو Chatwoot APIs للواجهة الأمامية. ولا ننشئ 100 ملف فارغ فقط ليبدو المشروع كبيرًا.

## ترتيب التنفيذ

```text
1. domain + ports
2. SQL migrations الأساسية
3. Event Ledger + Idempotency + Outbox
4. SocialAPI adapter + Provider Simulator
5. Chatwoot adapter
6. transport/webhooks
7. Vertical Slice inbound → mirror → outbound → status
8. AI Context + Intent + Decision
9. Catalog / Lead / Transaction
10. Automation / Analytics / Subscription
```

## القرار النهائي

**نعم، نعتمد هيكل الذكاء الاصطناعي كمرجع أقوى، مع التعديلات المذكورة.** المستودع الجديد الحالي كان Foundation صحيحًا لكنه يحتاج إعادة ترتيب المجلدات قبل بدء التنفيذ الحقيقي. لن نعود إلى المستودع القديم، ولن نخلط إرث Postiz مع Mujeeb 24.
