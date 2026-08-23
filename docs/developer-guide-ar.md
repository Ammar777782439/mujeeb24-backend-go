# دليل المطور — Mujeeb 24 Backend Go

## كيف تبدأ

ابدأ بقراءة `README.md` ثم `docs/project-status-ar.md` ثم `docs/structure_decision_ar.md`. بعد ذلك اقرأ عقود `contracts/` قبل إنشاء أي ملف Domain أو Application.

الأوامر الحالية:

```bash
go test ./...
go list ./...
```

المشروع Foundation؛ نجاح الاختبار الحالي يعني أن الحزم الموجودة قابلة للبناء، وليس أن التكاملات الخارجية أو الـSales workflows مكتملة.

## قاعدة اتجاه الاعتماد

```text
adapters/primary/http
          ↓
application/{ports,commands,queries,services,workers}
          ↓
domain
          ↑
adapters/secondary/{providers,workspaces,persistence,queue,ai}
```

`domain` لا يستورد Provider SDK أو Fiber أو PostgreSQL أو Redis أو Chatwoot DTO. `application` تنسق Use Cases وتعرّف Ports. `adapters` تنفذ Ports. `bootstrap` يركب الاعتماديات.

## أين يذهب كل شيء؟

| نوع الكود | مكانه |
|---|---|
| Entity وValue Object وInvariant | `internal/domain/<context>` |
| Command يغير الحالة | `internal/application/commands` |
| Query للقراءة | `internal/application/queries` |
| Orchestration واضح بين Ports وDomain | `internal/application/services` |
| Async entrypoint يستدعي Service | `internal/application/workers` |
| Interface تحتاجها Application | `internal/application/ports` |
| HTTP Handler وDTO وMiddleware | `internal/adapters/primary/http` |
| SocialAPI Provider | `internal/adapters/secondary/providers/socialapi` |
| Chatwoot Workspace Adapter | `internal/adapters/secondary/workspaces/chatwoot` |
| PostgreSQL repositories وmodels | `internal/adapters/secondary/persistence/postgres` |
| Redis/Asynq | `internal/adapters/secondary/queue/asynq` |
| Config/Database/HTTP lifecycle | `internal/platform` |
| تركيب dependencies | `internal/bootstrap` |
| SQL migrations | `migrations` |
| Contracts واختبارات provider | `contracts` و`tests` |

## قواعد Domain

كل كيان تجاري ينتمي إلى `business_id`. لا تستخدم Provider ID كهوية Mujeeb. استخدم Typed IDs ومراجع خارجية منفصلة.

لا تضع كل المحادثات والعملاء داخل Business Aggregate. كل Aggregate مستقل ويرتبط بIDs.

لا تجعل AI ينفذ side effects. AI ينتج Structured Decision، ثم Application يتحقق من Evidence وPolicy ويستدعي Command.

لا تجعل Chatwoot Domain داخل Mujeeb. نحتفظ بـConversation References وMessage References اللازمة للمزامنة والـSales Context فقط.

لا تجعل `Product` جذر الكتالوج. استخدم `CatalogItem` و`Offer` و`Variant` و`Attributes`، ثم `CommercialTransaction` بنوع Order أو Booking أو Appointment أو Service Request أو Quote.

## قواعد الموثوقية

كل Inbound event يُتحقق من توقيعه ويُحفظ قبل المعالجة. المعالجة At-Least-Once وIdempotent؛ لا نعتمد على Exactly-Once من Provider.

كل Outbound intent يملك Provider Idempotency Key. إذا كانت نتيجة Provider غامضة، الحالة `UNKNOWN` وتحتاج Reconciliation؛ لا نعيد الإرسال بشكل أعمى.

فشل Chatwoot Mirror لا يعيد إرسال رسالة العميل إلى SocialAPI. وفشل تسجيل الحالة بعد إرسال Provider لا يساوي فشلًا مؤكدًا.

## قواعد الأمان

لا تسجل tokens أو webhook secrets أو raw payloads الحساسة. استخدم SecretReference وSecret Port. لا تضع credentials حقيقية في `.env.example` أو Git.

كل عملية ذات side effect تملك `correlation_id`، وكل تعديل حساس يكتب Audit Event append-only.

## ترتيب التنفيذ الإلزامي

```text
Domain Contract
→ application/ports
→ SQL migrations
→ Event Ledger + Idempotency + Outbox
→ Provider Simulator
→ Chatwoot Adapter
→ Vertical Slice
→ AI Context + Intent + Decision
→ Catalog/Lead/Transaction implementation
```

أي اقتراح يتجاوز هذا الترتيب يحتاج سببًا واضحًا واختبارًا، وليس مجرد رغبة في البدء السريع.
