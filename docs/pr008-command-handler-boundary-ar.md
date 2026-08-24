# PR-008 — Typed Application Commands/Queries وHTTP Handler Boundary

## الحالة

تم تنفيذ **Typed Application boundary وHTTP mapping skeletons**. هذه المرحلة لا تنفذ PostgreSQL أو SocialAPI أو Chatwoot أو AI runtime. الـHandlers الحالية تستدعي Application interfaces عندما تُحقن dependencies، وتعيد `not_implemented` typed error عندما لا تكون Application wiring موجودة.

## قاعدة الاعتماد

```text
HTTP DTO
  → HTTP Handler mapping
  → Application Command / Query typed
  → Application Handler interface
  → Port
```

لا يستورد `internal/application` HTTP أو Huma أو PostgreSQL أو Provider SDKs. ولا يستورد `internal/adapters/primary/http/handlers` PostgreSQL أو SocialAPI أو Chatwoot.

## CommandMeta

كل Command يستخدم `commands.CommandMeta` ويحتوي على `ActorContext` و`RequestID` و`CorrelationID` و`IdempotencyKey` و`ExpectedVersion` الاختياري. `ActorContext.BusinessID` هو النطاق الذي تم التحقق منه، وليس قيمة موثوقة من Request Body.

`If-Match` يبقى **opaque resource version**. طبقة HTTP تزيل علامات الاقتباس فقط، ولا تحوله إلى رقم أو تفسر بنيته. Application أو Repository يقرر معنى المقارنة.

`Idempotency-Key` يتحول إلى Meta في Commands ذات side effect. Application هي التي تقرر إلزاميته ونطاقه؛ Handler لا ينفذ idempotency بنفسه ولا يقرر Provider retry.

## QueryMeta

القراءات تستخدم `queries.QueryMeta` مع Actor Scope وRequest/Correlation IDs، وحقول pagination/filter typed مثل `Limit` و`Cursor` و`Status` وIDs الداخلية.

## قواعد Handler

الـHandler مسؤول عن تحويل HTTP DTO إلى Application type، طلب Business Scope من `ScopeProvider`، ومواءمة أخطاء Application إلى HTTP error. لا يحتوي SQL ولا يرسل Provider requests ولا ينشئ Domain aggregate مباشرة.

كل Scope resolution يتحقق من أن BusinessID العائد من Auth/Membership يساوي BusinessID الموجود في Route. mismatch يرفض كـForbidden.

## Commands المغلقة

تشمل Business وPolicy، Channel Connections، Conversation actions وOutbound Message، Customer lifecycle وMerge، Catalog/Schema/Item/Offer/Variant، Leads، Transactions وReviews، Auth transport، وRequest Human Review.

## Queries المغلقة

تشمل Principal وBusinesses وBusiness/Policy، Connections/Capabilities، Dashboard Overview، Conversations/Messages، Customers، Catalog، Leads/Scores/Attributions، Transactions/Reviews، AI Decisions، وAudit Events.

## القيم الرقمية والمال

حقول HTTP القديمة التي تعرض `amount` أو `quantity` قد تستخدم JSON number لأسباب توافقية، لكن Application لا يستقبل أموالًا بصيغة `float64`. المال في Commands يستخدم minor units typed مثل `AmountMinor int64`، والكمية في `TransactionLine` تستخدم `commands.Decimal` كسلسلة عشرية base-10 validated. سياسة scale والتحويل النهائي إلى Domain Value Object ستُغلق مع تنفيذ Application/Domain use case، ولا يتغير نوع migration لمجرد هذا القرار.

## Error Mapping

Application لا يعيد HTTP status. يستخدم typed application errors مثل `validation_error` و`not_found` و`forbidden` و`conflict` و`stale_resource` و`external_dependency_unavailable`. HTTP Adapter يترجمها إلى status codes وErrorEnvelope.

## ما لم يُنفذ بعد

لم تُربط كل عمليات Huma الـ76 بتطبيقات Use Case فعلية؛ التسجيل الحالي وثيقة HTTP وroute skeleton، وCore mapping المطبق هو Conversations وCustomers مع أمثلة Scope/Meta. لا ندعي أن Business logic أو Repositories أو Auth storage جاهزة.

الخطوة التالية هي إكمال route wiring بنفس النمط عند الحاجة، ثم PostgreSQL Adapter وTransactionManager. لا نضع SQL داخل Handler ولا نعيد فتح OpenAPI أو Schema.
