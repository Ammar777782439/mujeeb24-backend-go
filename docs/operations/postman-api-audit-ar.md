# تدقيق جاهزية HTTP API لـPostman

## الحكم التنفيذي

يوجد **76 operation** في OpenAPI المتولد، وجميعها مسجلة في Huma وتملك dispatch typed. أصبح أول مانع حقيقي قد أُغلق: **Authentication + JWT Ed25519 + PostgreSQL Business Scope** موصولة في الـruntime عند `AUTH_ENABLED=true`. لا يعني ذلك أن جميع عمليات الأعمال الـ76 أصبحت ناجحة؛ فبعضها لا يزال يرجع `501 not_implemented` لأن application dependency المقابلة غير موصولة في bootstrap.

> لا تستخدم هذه الدفعة principal ثابتًا أو header تطوير أو token مضمّنًا. كل نجاح تجاري يعتمد على principal محفوظ في PostgreSQL وعضوية business نشطة.

| السطح | حالة Postman | الدليل/الشرط |
|---|---|---|
| `/api/v1/health/live` و`/api/v1/health/ready` | **قابل للاختبار محليًا** | public؛ readiness تفحص PostgreSQL وحالة feature configuration فقط. |
| `/api/v1/auth/login` | **قابل للاختبار محليًا** | principal موجود، bcrypt password صحيح، `AUTH_ENABLED=true`، مفاتيح Ed25519 من environment. |
| `/api/v1/auth/refresh` و`/auth/logout` | **قابل للاختبار محليًا** | refresh token opaque في `mujeeb_refresh` HttpOnly cookie، single-use rotation وrevoke حقيقيان في PostgreSQL. |
| `/api/v1/me` و`/me/businesses` | **قابل للاختبار محليًا** | Bearer JWT صالح وmembership نشطة. |
| Catalog / Sales / AI / Audit الموصولة أدناه | **قابلة لاختبار نجاح حقيقي** | JWT + business membership + سجلات مناسبة إن كانت العملية تحتاج record قائمًا. |
| Business / Conversations / Customers / Channel connection management غير المذكورة أدناه | **ليست جاهزة لنجاح Postman بعد** | operation/DTO موجودان، لكن dependency التطبيقية في runtime غير موصولة؛ النتيجة الصادقة `501`. |
| Webhooks | **اختبار contract محلي فقط** | يلزم verifier/secret مهيأ؛ لا SocialAPI أو Chatwoot live في هذه الدفعة. |
| Metrics | **غير موصول** | `GetMetrics` لا يزال `501`. |

## طبقة المصادقة ونطاق الأعمال المنفذة

```text
Postman login
  ↓
principal + bcrypt password hash (PostgreSQL)
  ↓
Ed25519 access JWT
  ↓
Huma bearer middleware (protected operations only)
  ↓
principal context
  ↓
active business_membership (PostgreSQL)
  ↓
ScopeProvider
  ↓
application handler / repository
```

المسارات العامة مقصودة وصريحة: login، refresh، liveness، readiness، metrics، وwebhooks. أما logout و`/me` وكل business operation فهي محمية بـBearer JWT. لا تمر membership من header؛ تُحل من PostgreSQL في كل طلب business scoped.

## العمليات ذات surface موصول في bootstrap

| المجموعة | العمليات القابلة للتشغيل محليًا |
|---|---|
| Auth/identity | `authenticatePrincipal`, `rotateRefreshSession`, `revokeRefreshSession`, `getCurrentPrincipal`, `listAccessibleBusinesses` |
| Operational | `getLiveness`, `getReadiness` |
| Messages/Capabilities | `listConversationMessages`, `getConnectionCapabilities` |
| Catalog | `createCatalog`, `updateCatalog`, `createAttributeSchemaVersion`, `createCatalogItem`, `updateCatalogItem`, `createOffer`, `updateOffer`, `createVariant`, `updateVariant`, `listCatalogs`, `getCatalog`, `listCatalogItems`, `getCatalogItem`, `listOffers`, `listVariants`, `listAttributeSchemas`, `getAttributeSchema` |
| Sales | `createLead`, `updateLead`, `qualifyLead`, `markLeadLost`, `createTransactionDraft`, `updateTransactionDraft`, `confirmTransaction`, `cancelTransaction`, `submitTransactionReview`, `approveTransactionReview`, `rejectTransactionReview`, `listLeads`, `getLead`, `listLeadAttributions`, `listLeadScores`, `listTransactions`, `listCustomerTransactions`, `getTransaction`, `getTransactionReview` |
| AI/Audit | `requestHumanReview`, `listAIDecisions`, `getAIDecision`, `listAuditEvents`, `getAuditEvent` |
| Conditional adapters | `beginChannelConnection` فقط عند تفعيل provisioning بإعدادات صحيحة؛ webhook operations فقط عند ضبط receiver/verifier. |

تم إصلاح فجوة dispatch كانت تجعل `listCatalogs` يرجع 501 رغم أن dependency محقونة؛ أصبح الآن يصل إلى service/repository مع تطبيق business scope الحقيقي.

## ما ما زال يرجع 501 بصدق

السطوح غير الموصولة بعد هي: `getBusiness`، profile/policy، dashboard overview، conversation/customer management، outbound message creation، channel connection read/reconnect/disconnect، وmetrics. وجودها في OpenAPI ليس دليلاً على نجاحها في runtime، ولذلك ستوضع في Collection داخل folder منفصل باسم **Known 501 — غير مكتملة**، لا ضمن smoke PASS.

## إعداد محلي آمن للاختبار

1. شغّل migrations أولًا، ثم أنشئ business صالحًا.
2. ولّد زوج Ed25519 محليًا وضعه فقط في environment: `JWT_ED25519_PRIVATE_KEY` و`JWT_ED25519_PUBLIC_KEY` بترميز Base64؛ لا تضع أي مفتاح داخل `.env.example` أو Git.
3. عيّن `AUTH_ENABLED=true` و`JWT_ISSUER` و`JWT_ACCESS_TTL` و`REFRESH_SESSION_TTL`.
4. شغّل `cmd/bootstrap-principal` بقيم environment وقت التشغيل فقط: `BOOTSTRAP_EMAIL` و`BOOTSTRAP_PASSWORD` و`BOOTSTRAP_DISPLAY_NAME` و`BOOTSTRAP_BUSINESS_ID` و`BOOTSTRAP_ROLE` و`BOOTSTRAP_PERMISSIONS_JSON`.
5. استخدم login من Collection. يحتفظ Postman بـ`Set-Cookie` تلقائيًا في Cookie Jar؛ لا تحفظ refresh token في Environment أو Collection.

## Truthmode للاختبار الحالي

اختبارات PostgreSQL 16 أثبتت migration 000041، email case-insensitive، membership isolation/revocation، opaque cursor، refresh single-use/revoke، transaction rollback، ومسار HTTP login → JWT → `/me` → scoped Catalog → cross-tenant 403 → refresh rotation → logout. لم يُشغّل أي SocialAPI أو Chatwoot أو Facebook/Instagram/WhatsApp أو LLM live في هذه الدفعة.
