# تدقيق جاهزية HTTP API لـPostman

## الحكم التنفيذي

يوجد **76 operation** في OpenAPI المتولد، وكلها مسجلة في Huma وتمتلك dispatch typed وhandler runtime. أُزيلت جميع حالات `501 not_implemented` الناتجة عن **Application Dependency غير موصولة**. لا تُفهم هذه النتيجة على أنها تشغيل حي لكل خدمة خارجية؛ المسارات التي لا تملك إعدادات SocialAPI أو Chatwoot حقيقية تعيد الآن خطأً واضحًا ومقصودًا بدلاً من نجاح مزيف أو 501.

> لا يوجد principal ثابت أو token مضمّن أو header تطوير. نجاح كل عملية business-scoped ما زال يعتمد على JWT Ed25519 وعضوية business نشطة في PostgreSQL.

| نوع العملية | النتيجة في Postman | الشرط أو السلوك |
|---|---|---|
| Health وMetrics | **نجاح محلي** | `/health/live` و`/health/ready` و`/metrics` تعمل دون تسجيل دخول؛ metrics تعرض pgxpool gauges المحلية فقط. |
| Auth وIdentity | **نجاح محلي** | login، refresh cookie rotation، logout، `/me`، و`/me/businesses` تعمل مع principal مُنشأ محليًا. |
| Business وDashboard | **نجاح محلي** | get/update profile، policy، overview مع PostgreSQL و`If-Match` حيث يلزم. |
| Customers وConversations | **نجاح محلي** | read/list/create/update/merge، conversation list/get/update/assign/labels، private notes، message timeline، وcustomer conversations كلها موصولة بـPostgreSQL scope. |
| Outbound Message Intent | **نجاح محلي حتى Outbox** | ينشئ `OutboundMessage` وOutbox transactionally بعد provider reference/connection active؛ **لا يرسل إلى provider من طلب HTTP**. |
| Catalog/Sales/AI/Audit | **نجاح محلي** | repositories والخدمات الموصلّة سابقًا تعمل مع business scope والسجلات اللازمة. |
| Channel Connections | **نجاح محلي للحالة والقراءة** | list/get/reconnect/disconnect تعمل في PostgreSQL، وتكتب reason/actor state event مع optimistic concurrency. |
| Begin Channel Provisioning | **422 عند التعطيل، لا 501** | يتطلب `CHATWOOT_PROVISIONING_ENABLED=true` وإعداد SocialAPI/Chatwoot صحيحين؛ لا يبدأ provider OAuth محليًا بلا تلك الإعدادات. |
| SocialAPI/Chatwoot Webhooks | **503 عند عدم إعداد receiver، لا 501** | تصبح accepted/processed فقط عند ضبط verifier وreceiver المناسبين؛ لا توجد محاكاة نجاح. |

## طبقة المصادقة والنطاق

```text
Postman login
  ↓
principal + bcrypt hash في PostgreSQL
  ↓
Ed25519 JWT access token
  ↓
Bearer middleware للمسارات المحمية
  ↓
active business_membership في PostgreSQL
  ↓
ScopeProvider
  ↓
service / repository scoped
```

المسارات العامة مقصودة: login، refresh، health، metrics، وwebhooks. أما logout و`/me` وكل business operation فهي محمية بـBearer JWT. لا يقبل النظام business scope من headers.

## الضمانات الجديدة لإزالة 501

| السطح | الضمان الذي أُضيف |
|---|---|
| Business profile/policy | migration `000042`، resource versions، update ذري، stale conflict، tests PostgreSQL وHTTP. |
| Customer/Conversation | migration `000043`، keyset opaque، tenant isolation، optimistic updates، merge، assignments. |
| Labels/private note | migration `000044`، labels normalized ومرتبة، private visibility صريحة في communication timeline، ولا Outbox للـprivate note. |
| Channel state | migration `000045`، state events تحفظ action/reason/actor، reconnect/disconnect لا تنفذ provider network call. |
| Manual outbound | current+active provider reference وconnection active إلزاميان؛ OutboundMessage وOutbox يكتبان في transaction واحدة و`Idempotency-Key` إلزامي. |
| Metrics | gauges PostgreSQL pool فقط؛ لا تدّعي metrics لخدمات خارجية غير مشغلة. |

## إعداد محلي آمن

شغّل migrations أولًا. أنشئ business ثم principal/membership عبر `cmd/bootstrap-principal` من environment وقت التشغيل، وأدخل مفاتيح Ed25519 وpassword فقط في environment المحلي. اضبط `AUTH_ENABLED=true` و`JWT_ISSUER` و`JWT_ACCESS_TTL` و`REFRESH_SESSION_TTL`. تحتفظ Postman بـrefresh token في Cookie Jar من `Set-Cookie`؛ لا يوضع refresh token أو password أو key داخل Environment أو Collection.

## Truthmode للاختبار الحالي

أثبتت اختبارات PostgreSQL 16 migrations `000001..000045`، isolation، optimistic concurrency، private visibility، channel state events، auth/scope، وmetrics/feature-gated response عبر HTTP. كما نجحت `go test ./...` و`go vet ./...` وOpenAPI drift. لم يتم تشغيل أي SocialAPI أو Chatwoot أو Facebook/Instagram/WhatsApp أو LLM live في هذه الدفعة.
