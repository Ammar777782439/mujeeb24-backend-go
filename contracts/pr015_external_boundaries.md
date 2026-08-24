# PR-015 External Boundaries

## النطاق

هذا العقد يثبت حدود التكامل الخارجي في Mujeeb 24 دون كشف DTOs أو أسرار SocialAPI.ai وChatwoot إلى Dashboard أو Domain. يملك Mujeeb الحقيقة التجارية والـtenant state، بينما يعمل SocialAPI كـprovider transport، ويعمل Chatwoot كـcommunication workspace داخلي.

## SocialAPI inbound

يُتحقق من توقيع webhook على raw bytes قبل normalization. يدعم verifier توقيع v2 باستخدام `timestamp + "." + raw_body` مع replay window، ويدعم fallback القديم الموثق في adapter. استثناء `webhook.test` يُقبل كـacknowledgement ولا يدخل EventStore.

`ProviderEventID` هو هوية الحدث القادمة من payload عندما تكون موجودة. `X-SocialAPI-Delivery` يُحفظ منفصلًا في `DeliveryID` لأنه ثابت عبر retries، ولا يُخلط تلقائيًا مع message/interaction ID. إذا غابت هوية الحدث من payload، يُستخدم delivery ID كـ`provider_specific_fallback`، ويُحفظ ذلك صراحة في `DedupeStrategy`.

بعد التحقق، يجب أن يمر raw payload إلى `RawPayloadStore` حقيقي يعيد reference opaque وSHA-256 للبايتات نفسها. لا يجوز إنشاء قيمة مثل `socialapi://...` والادعاء أنها تخزين. لذلك لا يُوصل SocialAPI ingestion إلى production Bootstrap حتى يوجد adapter durable لهذا المنفذ.

يُحل provider account إلى `channel_connections` عبر `GetByProviderReferences`. المطابقة الوحيدة تُسند business/connection. عدم وجود مطابقة يكتب الحدث بحالة `unresolved` مع بقاء business/connection فارغين، ولا يُخترع tenant من route أو payload. تعدد المطابقات خطأ conflict ولا يُقبل الحدث في pipeline العادية.

## Chatwoot inbound

يُتحقق من HMAC على `timestamp + "." + raw_body` مع replay window، وتُختبر numeric IDs في normalization. Chatwoot ليس provider transport لمجيب ولا مصدر customer truth؛ لذلك تُقبل callback الصحيحة بعد التحقق وتُهمل حاليًا لمنع echo/mirror loops. لا تُسجل في EventStore ولا تُحوّل إلى inbound customer message حتى يعتمد use case mapping مستقل.

## Outbound

لا يرسل worker داخل transaction. المسار المقصود هو: claim مع lease token، resolve من Mujeeb state، استدعاء provider خارج transaction، ثم `MarkCompleted` أو `MarkRetryableFailure/MoveToDeadLetter` مع owner/token.

خطأ النقل لا يثبت أن provider لم يقبل الرسالة؛ لذلك يستخدم processor `provider_send_outcome_unknown` مع `NextAttempt=nil`، ويترك قرار reconciliation لاحقًا بدل retry أعمى قد يكرر رسالة العميل. كما لا يثبت `Idempotency-Key` exactly-once عند provider؛ هو metadata best effort حتى يثبت provider contract ذلك.

## Bootstrap

يُنشئ Bootstrap clients اختياريًا عند وجود secrets في البيئة، دون network call أثناء startup. Chatwoot verifier-only service يمكن وصله عند وجود webhook secret. SocialAPI inbound service لا يُوصل بعد لأن RawPayloadStore durable غير موجود. وجود client أو constructor لا يعني أن اتصالًا حيًا أو إرسالًا فعليًا تم اختباره.
