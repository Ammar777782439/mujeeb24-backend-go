# تقرير PR-015 — External Communication Boundaries

## الحالة التنفيذية

تم تنفيذ ورفع حدود التكامل الآمن لـSocialAPI.ai وChatwoot في مستودع `mujeeb24-backend-go`. هذا الإصدار **ليس External E2E مكتملًا**؛ فهو يثبت عقود adapters وapplication boundaries والاختبارات المحاكية وBootstrap الاختياري، لكنه لا يدّعي اتصالًا حيًا أو إنشاء حساب أو تسجيل webhook أو إرسال رسالة.

> **النتيجة الأساسية:** أصبح الكود جاهزًا للانتقال إلى gap-fix خاص بـRawPayloadStore وlive configuration الآمن، وليس إلى إرسال خارجي غير متحقق.

## A. العقد الذي أثبته الكود

يحافظ Domain/Application على نماذج provider-neutral. يعرّف `ChannelProvider` عمليات التحقق من webhook، normalization، الإرسال، والاستعلام عن delivery status. ويحمل `InboundEvent` الآن `Channel` و`DeliveryID` و`DedupeStrategy` بحيث لا تختلط هوية webhook delivery بهوية message أو interaction.

يحوّل HTTP façade الرؤوس الخاصة بـSocialAPI وChatwoot إلى `IngestWebhookCommand`، بينما تبقى verification وnormalization وtenant resolution خارج HTTP handler. ويظل Dashboard غير مطلع على provider DTOs أو raw payloads أو secrets.

## B. لماذا يلزم هذا الحد

SocialAPI هو transport خارجي، بينما Chatwoot communication workspace داخلي. لذلك لا يملك Chatwoot customer truth ولا business truth في Mujeeb. وجود حدود مستقلة يمنع نسخ schema Chatwoot إلى Domain، ويمنع وضع network calls داخل repositories أو transactions.

استندت semantics الخاصة بالتوقيع وdelivery وsend إلى الوثائق الرسمية التي تمت مراجعتها وحُفظت نتائجها في `docs/pr015-external-research-notes-ar.md`؛ منها SocialAPI webhook/auth/inbox documentation [1] [2]، وChatwoot API/webhook documentation [3] [4].

## C. schema وraw payload policy

لم تُعدّل أي migration تاريخية، ولم تُضف migration جديدة في هذا الجزء. EventStore الحالي ما زال يفرض `RawPayloadReference` و`PayloadHash`. لذلك أُضيف `ports.RawPayloadStore` كحد صريح يعيد reference opaque وSHA-256 للـraw bytes الفعلية.

**لا يوجد بعد adapter durable لـRawPayloadStore في production composition root.** ولهذا لا يكتب SocialAPI ingestion reference صورية من نوع `socialapi://...`، ولا يُوصل SocialAPI inbound service في API runtime قبل توفير تخزين حقيقي. هذا مقصود لمنع الادعاء الكاذب بأن payload الخام محفوظ.

## D. رقم migration الفعلي

رقم migration لم يتغير. تحقق schema runner فعليًا من:

| الفحص | النتيجة |
|---|---:|
| migrations في قاعدة فارغة | `applied-33` |
| إعادة التشغيل | `applied-0` |
| foundation constraints | PASS |
| full schema constraints | PASS |

## E. Application ports والخدمات

أُضيف `RawPayloadStore` لتخزين bytes والتحقق من hash، وأُضيف `OutboundDeliveryResolver` لتحويل outbox obligation إلى provider-neutral send command بعد تحميل Mujeeb state.

أُضيفت `SocialAPIWebhookService` التي تنفذ: command validation، signature verification، webhook.test acknowledgement، normalization، raw payload storage، provider-account lookup، unresolved handling، ثم `EventStore.RecordIfAbsent` atomic dedupe.

إذا لم توجد channel connection مطابقة، تُسجّل الحالة `unresolved` دون اختراع `BusinessID` أو استخدام `route_key` كـtenant. وإذا تعددت المطابقات، تُعاد حالة conflict. أُضيفت `ChatwootWebhookService` بسياسة صريحة: التوقيع الصحيح يُقبل ويُهمل حاليًا لمنع echo/mirror loops، ولا يُسجّل في EventStore.

## F. PostgreSQL repository

أُضيف إلى `ChannelConnectionRepository` إجراء `GetByProviderReferences` المقيّد بـ`provider_ref` وprovider account/connection reference. ينفذ الاستعلام قراءة لاختيار مطابقة واحدة فقط، ويعيد typed `not_found` عند عدم وجودها وtyped `conflict` عند التعدد. أُضيف assertion إلى PostgreSQL integration fixture الحالي، لكن تشغيل اختبارات PostgreSQL integration نفسها كان **skipped** لأن `POSTGRES_TEST_DSN` لم يكن مضبوطًا في البيئة الحالية.

## G. List/HTTP/mapping وOutbound boundary

تم إصلاح raw webhook DTO ليستخدم نمط Huma `Body json.RawMessage` مع `RawBody []byte`. وهكذا يصف OpenAPI body كـJSON ويحتفظ التطبيق بالبايتات الأصلية التي يحتاجها HMAC، بدل إعادة JSON عبر marshal.

أُضيف HTTP test فعلي عبر Huma يثبت وصول `X-SocialAPI-Signature-V2` و`X-SocialAPI-Timestamp` و`X-SocialAPI-Delivery` و`X-SocialAPI-Event` وrequest ID والـraw body إلى command.

أُضيف `OutboxProcessor`: claim مع owner/token/lease، resolve، provider call خارج transaction، ثم completion أو failure مع fencing. خطأ النقل يُسجّل كـ`provider_send_outcome_unknown` مع `NextAttempt=nil` لأن فشل الشبكة لا يثبت أن provider لم يقبل الرسالة؛ لا يوجد retry أعمى ولا ادعاء exactly-once.

## H. الاختبارات التي شُغّلت فعليًا

| الاختبار أو الفحص | النتيجة |
|---|---:|
| `go test ./...` | PASS |
| `go vet ./...` | PASS |
| SocialAPI adapter contract tests | PASS |
| Chatwoot CRUD + HMAC/replay/normalization tests | PASS |
| application webhook ingestion tests | PASS |
| Outbox processor tests | PASS |
| HTTP Huma header-forwarding test | PASS |
| `go generate ./internal/adapters/primary/http/contract` | PASS |
| `scripts/check-openapi-generated.sh` | PASS |
| `scripts/test-postgres-schema.sh` | PASS — 33/0 |
| PostgreSQL integration packages with `-p 1` | SKIPPED — `POSTGRES_TEST_DSN` غير موجود |
| `git diff --check` | PASS |

اختبارات adapter استخدمت `httptest` وعناوين محلية/placeholder، ولم تستخدم مفتاح SocialAPI الحقيقي أو Chatwoot token حقيقي.

## I. الملفات التي تغيرت

| المجال | الملفات الرئيسية |
|---|---|
| SocialAPI | `internal/adapters/secondary/providers/socialapi/client.go` و`client_test.go` |
| Chatwoot | `internal/adapters/secondary/workspaces/chatwoot/client.go` و`client_test.go` و`webhook_test.go` |
| Application ports | `channel_provider.go`, `core_repositories.go`, `communication_workspace.go`, `raw_payload.go`, `outbound_delivery.go` |
| Application services | `webhook_ingestion.go` و`outbox_processor.go` مع الاختبارات |
| HTTP | `dto/inputs.go`, `handlers/system_facades.go`, واختبار forwarding |
| PostgreSQL | provider-reference lookup واختبار fixture |
| Bootstrap/config | `api.go`, `worker.go`, `integrations.go`, `config.go`, `.env.example` |
| Contracts/docs | `contracts/pr015_external_boundaries.md` وملفات البحث والتقرير |
| OpenAPI | `api/openapi/mujeeb24-dashboard-v1.generated.yaml` مولد تلقائيًا |

## J. Commit SHA وGit state

الـfeature commit هو `eae2e5c`، ثم دُمج تحديث remote غير متزامن في merge commit النهائي:

`d9c83e393a7e9573d04b26d8ea142a927f620c43`

تم الدفع إلى `origin/main` بنجاح. بعد `git fetch origin main` كانت النتيجة:

| الحالة | النتيجة |
|---|---:|
| `HEAD == origin/main` | true |
| working tree clean | true |
| final verification | PASS |

أثناء الدمج وُجد تحديث remote لـ`.env.example` يحتوي قيمة credential مكشوفة؛ لم أحتفظ بها، وأُبقي الملف النهائي placeholders فارغة. يجب إلغاء/تدوير ذلك المفتاح وعدم إعادة استخدامه.

## K. ما تبقى من PR-015

يبقى أولًا تنفيذ adapter durable لـ`RawPayloadStore` مع سياسة retention/redaction ومفتاح تخزين opaque، ثم حقن `SocialAPIWebhookService` في Bootstrap بعد اكتمال ذلك. يبقى أيضًا بناء resolver production لـOutboundDelivery يقرأ outbound message، conversation reference، channel connection، ومحتوى الرسالة من Mujeeb state داخل حدود transaction-safe، ثم توصيل worker loop الحقيقي دون network داخل transaction.

للاختبار الحي نحتاج إدخالًا سريًا خارج المحادثة: SocialAPI API key محدود بالـbrand التجريبي، SocialAPI webhook secret، وChatwoot base URL/token/account/inbox/webhook secret عند الحاجة. أول فعل حي يجب أن يكون read-only `GET /v1/accounts`. لم يُنفذ هذا الفعل في هذه الدفعة، ولم يُنفذ connect أو webhook registration أو send.

## References

[1]: https://docs.social-api.ai/guides/authentication "SocialAPI Authentication"
[2]: https://docs.social-api.ai/guides/webhooks "SocialAPI Webhooks"
[3]: https://developers.chatwoot.com/api-reference/introduction "Chatwoot API Introduction"
[4]: https://www.chatwoot.com/hc/user-guide/articles/1677693021-how-to-use-webhooks "Chatwoot Webhooks"
