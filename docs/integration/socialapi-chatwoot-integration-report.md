# تقرير اختبار التكامل الحقيقي: Mujeeb 24 وSocialAPI.ai وChatwoot Self-Hosted

**الحالة:** تقرير truth-mode قبل التنظيف والدفع النهائي
**التاريخ:** 25 أغسطس 2026
**المستودع:** `mujeeb24-backend-go`
**النطاق:** بيئة محلية معزولة وبيانات اختبار اصطناعية فقط

## A. الخلاصة التنفيذية

أُجري اختبار حقيقي، وليس Provider Simulator، بين أجزاء Mujeeb 24 ونسخة Chatwoot Self-Hosted تعمل محليًا مع PostgreSQL وRedis منفصلين. ثبتت صحة تشغيل Chatwoot، ومصادقته، وApplication API الفعلي، كما ثبتت سلسلة Go adapter الحقيقية من إنشاء Contact إلى Conversation ثم Message. وثبتت كذلك طبقة Mujeeb الخاصة بالـwebhook من خلال توقيع HMAC على البايتات الخام بدقة.

لكن الاختبار **ليس E2E مكتملًا**. لا يوجد حاليًا orchestrator إنتاجي ينشئ mapping دائمًا بين كيانات Mujeeb وChatwoot، ولا يوجد `OutboundDeliveryResolver` أو worker production ينفذ Outbox إلى provider. كما أن SocialAPI لم يُختبر محليًا في هذه الدفعة لغياب credential محلي صالح، ولا يوجد public HTTPS endpoint لاستقبال callback خارجي. لذلك صُنفت هذه الأجزاء **BLOCKED** أو **NOT TESTED** بدل تحويلها إلى PASS افتراضي.

## B. حدود السلامة والنطاق

استُخدمت بيانات اختبار اصطناعية فقط. لم يُرسل أي outbound إلى Facebook أو عميل حقيقي، ولم تُستخدم بيانات production، ولم تُرفع credentials إلى GitHub أو تُضمّن في التقرير أو source files. إعداد Compose والقالب الآمن فقط هما المرشحان للدفع؛ أما ملف البيئة المحلي وأي token مؤقت وأدوات receiver المحلية فخارج Git ويجب حذفها في cleanup.

> **قاعدة التقرير:** لا تعني استجابة HTTP الناجحة وحدها أن mapping أو delivery أو business processing اكتمل. كل PASS أدناه مرتبط بدليل محدد، وكل ما لم يُثبت بقي BLOCKED أو NOT TESTED.

## C. البيئة التي اختُبرت فعليًا

يعمل Chatwoot Self-Hosted محليًا على `127.0.0.1:3300`، مع PostgreSQL خاص به على منفذ محلي منفصل وRedis منفصل. ويعمل PostgreSQL الخاص بـMujeeb في حاوية أخرى وعلى منفذ مختلف. استُخدم `network_mode: host` في Compose بسبب قيد kernel المحلي في Docker bridge؛ ولم يُعتبر ذلك بديلًا عن عزل قواعد البيانات.

| المكوّن | الحالة المثبتة | الدليل |
|---|---|---|
| Chatwoot Rails/API | PASS | `GET /api` أعاد HTTP 200 |
| Chatwoot PostgreSQL | PASS | حاوية منفصلة وحالة healthy |
| Chatwoot Redis | PASS | حاوية منفصلة وحالة healthy |
| Mujeeb PostgreSQL | PASS | حاوية منفصلة ومنفذ مختلف عن Chatwoot |
| Mujeeb API live | PASS | `/api/v1/health/live` أعاد HTTP 200 |
| Mujeeb API ready | PASS | `/api/v1/health/ready` أعاد HTTP 200 |
| Worker process lifecycle | PASS جزئيًا | العملية موجودة، لكن التنفيذ الحالي ينتظر cancellation ولا ينفذ polling أو delivery |

## D. تثبيت Chatwoot Self-Hosted

تم تشغيل Chatwoot من Compose آمن داخل `integration/chatwoot/docker-compose.integration.yml`، مع `.env.example` لا يحتوي أسرارًا حقيقية. تم تشغيل migrations/prepare اللازمة للنسخة المحلية، ثم التحقق من readiness عبر HTTP وليس عبر حالة الحاوية فقط. قاعدة Chatwoot منفصلة عن قاعدة Mujeeb، ولم تُستخدم قاعدة Chatwoot لتخزين EventStore أو Outbox الخاص بـMujeeb.

وثائق Chatwoot الرسمية تؤكد أن Application APIs متاحة في self-hosted وتتطلب user `access_token` للمصادقة [1]. كما تؤكد وثائق النشر الرسمية مسار Docker/Compose وتهيئة قاعدة البيانات [3].

## E. اتصال SocialAPI.ai

الاختبار السابق المنفصل في GitHub Actions، رقم run `32792979839`، أثبت read-only account check حقيقيًا: HTTP 200، `account_count=1`، والمنصة `facebook`. هذا دليل اتصال وحساب فقط، وليس دليل connect أو send أو webhook registration.

لم يُنفذ في هذه الدفعة أي SocialAPI local outbound أو connect أو register webhook؛ فالـcredential المحلي غير متاح ضمن البيئة الحالية، ولا يجوز إعادة استخدام أسرار ظهرت سابقًا أو إنشاء GitHub Secret جديد. كذلك لا يوجد public HTTPS endpoint لاستقبال callback من SocialAPI. النتيجة إذن: **SocialAPI read-only account = PASS**، وكل العمليات الخارجية الأخرى = **BLOCKED**.

## F. Chatwoot Application API الحقيقي

استُخدمت نسخة Chatwoot Self-Hosted الفعلية مع Application API، وليس mock server. ثبتت المصادقة وقائمة inboxes، ثم ثبتت عمليات Contact وConversation وMessage عبر HTTP 200 وقراءة timeline. بعد ذلك كُشف اختلاف schema حقيقي في Chatwoot:

| العملية | النتيجة | الدليل |
|---|---|---|
| Application authentication | PASS | طلب authenticated إلى قائمة inboxes أعاد HTTP 200 |
| Account/inbox readiness | PASS | Account محلي وAPI Inbox محلي قابلان للاستخدام |
| Create Contact عبر HTTP | PASS | HTTP 200؛ الاستجابة الفعلية تستخدم `payload.contact.id` |
| Create Conversation عبر HTTP | PASS | HTTP 200 |
| Create Message عبر HTTP | PASS | HTTP 200؛ timeline قرأ الرسالة |
| Go adapter Contact→Conversation→Message | PASS | smoke حقيقي أعاد contact/conversation/message IDs ونجح بعد decoder fixes |

تم تعديل Go adapter لدعم `payload` كـarray، و`payload.contact.id` كـobject، وroot `id` عند وجوده. كما عولج `message_type` عندما يعيده Chatwoot كرقم، مع تطبيع القيم المعروفة إلى labels داخل adapter بدل تسريب enum provider إلى Application port. أضيفت اختبارات contract للأشكال القديمة والفعلية.

## G. Webhook وHMAC

توضح وثائق Chatwoot الرسمية أن callback الموقّع يتضمن `X-Chatwoot-Timestamp` و`X-Chatwoot-Signature`، وأن التوقيع هو HMAC-SHA256 على `{timestamp}.{raw_request_body}` مع prefix `sha256=` [2]. لذلك أُنشئ harness دائم لا يستخدم command substitution لحفظ body، بل يوقع exact bytes ثم يرسل الملف نفسه عبر `--data-binary`.

| الاختبار | الحالة | الدليل |
|---|---|---|
| valid signature إلى Mujeeb route | PASS | HTTP 202 |
| invalid signature إلى Mujeeb route | PASS | HTTP 401 |
| opaque non-empty route key | PASS | regression test يثبت قبول callback key غير مساوي لاسم provider |
| application unauthenticated error mapping | PASS | regression test يثبت HTTP 401 بدل HTTP 500 |
| Chatwoot-origin callback فعلي إلى Mujeeb | BLOCKED | لا يوجد public HTTPS endpoint، والاختبار المنفذ يدوي signed delivery إلى route المحلي |
| Chatwoot callback persistence | NOT TESTED/BLOCKED | Chatwoot service الحالي verification-only و`Ignored=true` عمدًا لمنع echo loops |

الـ202 هنا يثبت boundary verification/acknowledgement فقط. لا يثبت أنه تم إنشاء Customer أو Conversation أو Message في Mujeeb.

## H. EventStore وinbound dedupe

تم تشغيل اختبارات PostgreSQL integration الحقيقية باستخدام build tag `integration` على PostgreSQL Mujeeb المعزولة. شملت الاختبارات تسجيل أول event، duplicate detection، provider-connection scoping، والتزامن، مع تنفيذ migrations قبل الاختبارات. نتيجة الاختبار المحدد كانت PASS لـ`TestInboundEventStoreAgainstPostgres`.

كما ثبت runner الرسمي للمخطط أن التشغيل الأول طبق 33 migration، والتشغيل الثاني طبق 0 migration. هذا يثبت idempotency الخاصة بالـmigration runner في قاعدة الاختبار، ولا يعني أن SocialAPI inbound production wiring اكتمل.

## I. Outbox وlease reliability

اختبار `TestOutboxStoreAgainstPostgres` نجح على PostgreSQL الحقيقي ضمن نفس تشغيل integration. هذا يثبت storage وclaim/lease mutation boundary وعمليات completion/retry/dead-letter بحسب contract الموجود في الاختبار. لا يثبت إرسالًا خارجيًا ولا exactly-once؛ فالـnetwork outcome يظل قابلًا لأن يكون unknown، ولا يجوز إعادة الإرسال الأعمى.

| جانب Outbox | الحالة | التفسير |
|---|---|---|
| PostgreSQL persistence | PASS | integration test حقيقي نجح |
| Claim/lease fencing | PASS | مغطى ضمن اختبار Outbox الحقيقي |
| Production polling loop | BLOCKED | WorkerRuntime الحالي lifecycle-only |
| Provider delivery | BLOCKED | لا يوجد production `OutboundDeliveryResolver` |
| Exactly-once over network | NOT CLAIMED | غير ممكن إثباته بهذا التصميم |

## J. Mujeeb↔Chatwoot mapping

هذا الجزء **ليس PASS**. توجد ports وadapter لإنشاء عناصر workspace، لكن لا توجد application orchestration service مكتملة تربط Mujeeb-owned Customer وConversation وCommunicationMessage بمراجع Chatwoot، ولا توجد معاملة application تحفظ mapping في `conversation_references` أو message references بعد نجاح workspace call. لذلك لا يجوز اعتبار customer resolution أو duplicate prevention أو mirror synchronization منجزًا.

القرار المعماري ثابت: Mujeeb يملك business/customer/conversation/message truth، وChatwoot workspace داخلي/مرآة، وSocialAPI provider transport. إبقاء Chatwoot callback في verification-only حاليًا يمنع loop ويمنع ادعاء mapping غير موجود.

## K. Outbound

| مسار outbound | الحالة | الدليل/السبب |
|---|---|---|
| Mujeeb business transaction إلى Outbox | PARTIAL/PASS على مستوى storage | Outbox integration test نجح |
| Worker claim ثم resolve delivery | BLOCKED | لا يوجد polling/execution production |
| Chatwoot mirror send من Mujeeb | BLOCKED | mapping/orchestrator غير موجود |
| SocialAPI send | BLOCKED | credential محلي غير متاح ولا يوجد test-safe target مثبت |
| Facebook outbound | BLOCKED عمدًا | لم يُرسل أي شيء إلى Facebook أو عميل حقيقي |

## L. End-to-End scenario

السيناريو الكامل المطلوب هو: inbound provider event، EventStore dedupe، tenant/identity resolution، Mujeeb communication record، Chatwoot mirror، AI/business action، Outbox claim، provider delivery، ثم delivery status. لم يكتمل هذا السيناريو فعليًا.

المثبت فعليًا هو مساران منفصلان: أولًا Chatwoot API chain حقيقية عبر Go adapter، وثانيًا Mujeeb webhook boundary مع HMAC valid/invalid، وثالثًا EventStore/Outbox storage tests على PostgreSQL. لذلك تصنيف E2E هو **BLOCKED** وليس PASS.

## M. مصفوفة الحقيقة النهائية

| المجال | الحالة | Evidence أو الفجوة الواقعية |
|---|---|---|
| SocialAPI authentication/account read-only | PASS | GitHub Actions run `32792979839`: HTTP 200، account count 1، Facebook |
| SocialAPI connect | BLOCKED | لا credential محلي صالح ضمن النطاق الحالي |
| SocialAPI send | BLOCKED | لا test-safe target مثبت؛ لا outbound مُرسل |
| SocialAPI webhook registration | BLOCKED | لا local credential/public endpoint |
| Chatwoot self-hosted health | PASS | `/api` HTTP 200 وحاويات DB/Redis healthy |
| Chatwoot auth/account/inbox | PASS | Application API authenticated request HTTP 200 |
| Chatwoot Contact | PASS | HTTP 200 وGo decoder يدعم schema الحقيقي |
| Chatwoot Conversation | PASS | HTTP 200 عبر API وGo adapter |
| Chatwoot Message/timeline | PASS | HTTP 200 وقراءة timeline، وGo decoder يدعم numeric message type |
| Mujeeb webhook valid signature | PASS | exact raw-byte HMAC ثم HTTP 202 |
| Mujeeb webhook invalid signature | PASS | HTTP 401 |
| Actual Chatwoot-origin callback | BLOCKED | لا public HTTPS delivery مثبتة |
| EventStore first/duplicate/scoping | PASS | PostgreSQL integration test نجح |
| Outbox persistence/claim/lease | PASS | PostgreSQL integration test نجح |
| Worker/provider execution | BLOCKED | Worker lifecycle-only؛ لا resolver إنتاجي |
| Mujeeb customer/conversation/message mapping | BLOCKED | application orchestrator وreference persistence غير موجودين |
| Chatwoot mirror | BLOCKED | callback intentionally ignored، ولا mirror use-case مكتمل |
| Full E2E | BLOCKED | عدة حدود لازالت غير موصولة |
| Failure case | NOT TESTED | لا توجد حالة provider failure/outcome-unknown حقيقية أُرسلت خارجيًا |

## N. الملفات، الاختبارات، والتنظيف

التغييرات البرمجية المرشحة تشمل إصلاح Chatwoot response decoding، اختبارات contract الجديدة، إصلاح route-key validation، تحويل أخطاء facade إلى أخطاء Huma صحيحة، واختبار regression لـ401. كما تشمل integration fixture الآمن و`verify-webhook.sh` الذي يوقع exact raw bytes ولا يحتوي سرًا. إعداد Compose و`.env.example` و`.gitignore` وREADME الآمن موجودة ضمن integration config.

أُجريت اختبارات `go test ./...` و`go vet ./...`، وتوليد OpenAPI، وفحص drift، وفحص whitespace. كما أُجريت اختبارات integration PostgreSQL بالـbuild tag الصحيح، ونجحت اختبارات AI/Audit وBusiness وCore repositories وEventStore وOutbox وSales وBootstrap. ونجح schema validation: `applied-33` ثم `applied-0`.

قبل الدفع النهائي يجب إيقاف Mujeeb API وWorker وreceiver المحلي، حذف token المحلي وملف البيئة المحلي والسجلات والملفات المؤقتة، إيقاف Compose مع حذف volumes الخاصة ببيئة Chatwoot، حذف حاوية/volume PostgreSQL الخاصة باختبار Mujeeb فقط، ثم التحقق من أن `.env` غير موجود وغير متتبع. لا يجوز حذف أي حاوية أو volume غير تابع لبيئة التكامل هذه.

**Commit SHA:** `ec8bf732ef75b027f7d1d663d61dc4a1a232f6a7` هو commit التغييرات المحلي الحالي قبل amend النهائي. بعد تحديث هذه الفقرة يجب أن يكون SHA النهائي مختلفًا؛ سيُثبت SHA النهائي في نتيجة التحقق بعد push. لا يحتوي هذا التقرير على credentials أو passwords أو usernames أو tokens أو raw payloads.

## المراجع

[1]: https://developers.chatwoot.com/api-reference/introduction "Chatwoot Developer Docs — Introduction to Chatwoot APIs"

[2]: https://developers.chatwoot.com/api-reference/webhooks/add-a-webhook "Chatwoot Developer Docs — Add a webhook"

[3]: https://developers.chatwoot.com/self-hosted/deployment/docker "Chatwoot Developer Docs — Docker deployment"
