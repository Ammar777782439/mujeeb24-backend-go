# Mujeeb 24 Backend — دليل التشغيل والإستجابة للحوادث

## الغرض وحدود الحقيقة

هذا الدليل يصف تشغيل **Backend مجيب 24** بعد PR-028 ودفعة Production Hardening. مجيب هو مالك الحقيقة التجارية: العملاء، المحادثات، الرسائل المعيارية، قرارات الذكاء، الرسائل الصادرة، وسجل الأحداث. لا يصبح Chatwoot أو SocialAPI مصدر الحقيقة لمجرد أن أحدهما متصل.

هذا الدليل لا يثبت اتصالًا حيًا مع SocialAPI أو Chatwoot أو Facebook أو Instagram أو WhatsApp. كل ما يتعلق بهذه الخدمات هنا هو **إعداد وتشغيل مشروط**؛ لا يُعلن نجاحه إلا بعد اختبار خارجي منفصل ومصرّح به.

| مكوّن | دوره | حالته في الكود |
|---|---|---|
| PostgreSQL | الحقيقة الدائمة وسجل الأحداث والـOutbox وmirror jobs | مطلوب لصحة الخدمة |
| API | يستقبل Dashboard/webhooks ويحوّلها إلى سجلات مجيب | جاهز محليًا وعلى PostgreSQL |
| Worker | يطالب من Outbox وChatwoot mirror jobs وينفذها خارج المعاملة | جاهز مع feature gates |
| SocialAPI | نقل القناة والتحقق من webhook وإرسال الرسائل | contract-tested فقط |
| Chatwoot self-hosted | مساحة عمل داخلية ومزامنة اختيارية | contract-tested فقط |
| LLM | proposal منظم خلف policy gate | معطل افتراضيًا |

## ترتيب التشغيل

ابدأ PostgreSQL أولًا، ثم شغّل migrations forward-only، ثم API، ثم Worker كعملية مستقلة. لا تشغّل Worker من داخل API process ولا تشغّل العملية من sandbox مؤقت؛ الـworker يحتاج عملية مستمرة يمكن مراقبتها وإعادة تشغيلها بطريقة مسيطَر عليها.

> API يستقبل ويبهوكات ويكتب التزامًا durable. Worker فقط هو الذي ينفذ network delivery لاحقًا. لا يوجد اتصال SocialAPI أو Chatwoot داخل PostgreSQL transaction.

| الخطوة | التحقق المطلوب | النتيجة الصحيحة |
|---|---|---|
| قاعدة البيانات | migrations كاملة مرة أولى ثم صفر مرة ثانية | schema ثابت ولا توجد migrations متبقية |
| API | `GET /api/v1/health/live` | `process=ok` |
| API | `GET /api/v1/health/ready` | `postgresql=ok`؛ الميزات الاختيارية تظهر `configured` أو `disabled` |
| Worker | log بداية العملية | يظهر poll interval وbatch size وحالة provider/mirror بدون أسرار |
| Worker | إيقاف SIGTERM | يوقف polling جديدًا ويمنح الدورة الجارية مهلة bounded قبل إغلاق DB |

## إعدادات البيئة

لا تحفظ قيم الأسرار في Git أو في ملفات report. تحفظ في secret manager الخاص ببيئة التشغيل فقط. الإعدادات الافتراضية تبقي كل اتصال خارجي حساس **معطلًا**.

| المجموعة | المتغيرات | القاعدة |
|---|---|---|
| أساسية | `DATABASE_URL`, `HTTP_ADDR`, `APP_ENV`, `SHUTDOWN_TIMEOUT` | `DATABASE_URL` إلزامي؛ لا تبدأ الخدمة بدونه |
| Worker | `WORKER_POLL_INTERVAL`, `WORKER_BATCH_SIZE`, `WORKER_OWNER` | defaults: `2s`, `20`, `mujeeb-worker`؛ owner ثابت لكل instance لتتبع leases |
| SocialAPI | `SOCIALAPI_BASE_URL`, `SOCIALAPI_API_KEY`, `SOCIALAPI_WEBHOOK_SECRET`, `SOCIALAPI_HTTP_TIMEOUT` | لا يوضع المفتاح في client أو Git |
| Chatwoot | `CHATWOOT_BASE_URL`, `CHATWOOT_API_TOKEN`, `CHATWOOT_WEBHOOK_SECRET`, `CHATWOOT_PLATFORM_API_TOKEN` | Platform token لمسار self-hosted provisioning فقط |
| LLM | `LLM_ENABLED`, `LLM_BASE_URL`, `LLM_API_KEY`, `LLM_MODEL` | إذا فُعلت يجب أن تكون المجموعة كاملة |

### Feature gates

| Flag | القيمة الافتراضية | لا يسمح بتفعيله إلا عندما |
|---|---:|---|
| `CHATWOOT_AUTOREPLY_ENABLED` | `false` | Chatwoot webhook secret وSocialAPI API key وLLM كامل متوفرون |
| `CHATWOOT_MIRROR_ENABLED` | `false` | SocialAPI webhook secret وChatwoot API token متوفران |
| `CHATWOOT_PROVISIONING_ENABLED` | `false` | SocialAPI API key وChatwoot Platform token وcallback/webhook URLs مكتملة؛ وفي production يجب أن تكون public HTTPS |
| `LLM_ENABLED` | `false` | Base URL وAPI key وmodel وحدود input/output صالحة |

## قراءة الصحة والمراقبة

`live` يجيب فقط عن بقاء process حيًا. `ready` يجيب عن قدرة قاعدة البيانات على العمل، ثم يعرض feature checks ثابتة لا تكشف أسرارًا. الحالة `configured` تعني أن الكود أُعطي prerequisites اللازمة؛ **لا تعني** أن SocialAPI أو Chatwoot تم اختباره حيًا أو أن رسالة وصلت للطرف الخارجي.

راقب logs التالية بصيغة structured أو كسجل مركزي: webhook verification failures، ledger states، outbox dead letters، mirror dead letters، delivery status غير المطابق، وإيقاف worker. يجب أن تتضمن السجلات business/resource IDs آمنة للتشخيص، ولا تتضمن raw contact أو token أو payload كاملًا إلا ضمن مخزن raw الداخلي المقيّد.

## الاستجابة للحوادث

| العرض | لا تفعل | افعل | المالك |
|---|---|---|---|
| webhook مرفوض | لا تعطّل HMAC | راجع signature/timestamp/route config ثم أصلح secret في secret manager | تشغيل القنوات |
| inbound event غير معروف | لا تنشئ Customer يدويًا | راجع provider account ↔ ChannelConnection؛ يبقى ledger unresolved بوضوح | تشغيل القنوات |
| Outbox `dead_letter` بسبب `provider_send_outcome_unknown` | **لا تعيد الإرسال** | استعلم عن provider message/status من المصدر الحي عند توفر موافقة؛ ثم قرر reconcile أو message جديد مستقل | مسؤول التشغيل |
| Chatwoot mirror `dead_letter` | لا تعيد job أعمى | افحص Account/Inbox/Conversation في Chatwoot؛ لأن create APIs لا تضمن idempotency عامة لمسار الرسالة | مسؤول workspace |
| delivery status بلا OutboundMessage | لا تنشئ رسالة صادرة من callback | يحتفظ النظام بسجل `ignored` ويُراجع provider message correlation | تشغيل القنوات |
| PostgreSQL غير جاهز | لا تتابع webhook processing يدويًا | أصلح DB أولًا؛ readiness سيبقى `not_ready` | مسؤول قاعدة البيانات |

حاليًا يوجد `OutboxStore.Requeue` داخل backend، لكنه ليس أداة تشغيل عامة للمستخدم. لا تعدّل status أو leases مباشرة عبر SQL، ولا تكتب script requeue مؤقتًا. أي إعادة تشغيل يجب أن تأتي بعد reconciliation موثقة وبأمر إداري typed ومراجَع في دفعة مستقلة.

## النسخ الاحتياطي والاستعادة

خذ نسخة PostgreSQL منتظمة تشمل schema والبيانات. عند الاستعادة إلى بيئة جديدة، شغّل migrations forward-only فقط؛ لا تعدّل أو تعيد ترقيم migrations القديمة. قبل إعادة تشغيل Worker، راجع entries ذات `processing` المنتهية leases و`dead_letter` بدل إرسالها تلقائيًا.

## قائمة موافقة التشغيل الحي

لا تُفعل أي feature flag خارجي حتى تؤكد كل البنود التالية: domain HTTPS عام، قاعدة بيانات production مع backup مجرّب، API وWorker كعمليتين مراقبتين، secrets manager، SocialAPI redirect/webhook مسجلان، Chatwoot self-hosted عند استخدام provisioning، business/channel test tenant، ومسار rollback. بعد ذلك فقط ينفذ اختبار خارجي واحد منخفض الأثر وبموافقة صريحة.

## ما لا يزال خارج هذا الدليل

Dashboard التاجر خارج النطاق عمدًا. كذلك لا يوجد هنا نشر فعلي أو credentials أو إنشاء حسابات خارجية أو إرسال رسالة حقيقية. تلك عمليات منفصلة لأنها تغيّر أنظمة خارجية وقد تنتج بيانات أو تكاليف أو تواصلًا مع عميل.
