# ملاحظة تشغيلية: اختبار SocialAPI Webhook عبر HTTPS مؤقت

**التاريخ:** 2026-08-27

تم إنشاء Webhook في لوحة SocialAPI بعنوان HTTPS مؤقت يشير إلى مسار الاستقبال في Mujeeb، مع الاشتراك في حدث `dm.received` فقط. رفضت اللوحة المحاولة الأولى لأن endpoint لم يكن يعمل، ثم نجح الإنشاء بعد تشغيل API Mujeeb فعليًا على PostgreSQL محلية مهيأة بالمخطط الحالي.

أظهرت لوحة SocialAPI لاحقًا تسليمًا من نوع `dm.received` بالحالة **delivered** وباستجابة HTTP `202` من Mujeeb في محاولة واحدة. لا تحفظ هذه الملاحظة نص الرسالة أو معرفات المستخدم/المحادثة/التسليم أو endpoint secret أو رؤوس التوقيع.

أكدت PostgreSQL المؤقتة أن event ledger احتوى سجلًا واحدًا بقيمة `signature_verified = true` وسجل payload خام واحد. كانت `processing_state = unresolved`، وهو السلوك المتوقع لأن البيئة المؤقتة لا تحتوي `ChannelConnection` أو binding تجاريًا يطابق حساب المزود الوارد. لذلك يثبت الاختبار **الوصول الخارجي والتحقق والتسجيل الآمن**، ولا يثبت materialization إلى Customer/Conversation/CommunicationMessage أو Chatwoot.

أجري كذلك اختبار رفض سلبي محلي على نفس endpoint بترويسة توقيع مزيفة لحدث `dm.received`. كانت النتيجة HTTP `401` ولم يتغير عدد سجلات event ledger، ما يثبت أن طلبًا غير موقّع بصورة صحيحة لا يُقبل أو يُخزَّن.

يقبل Mujeeb حدث التسجيل الرسمي `webhook.test` بدون توقيع وبدون materialization أو persistence، وهو الاستثناء المطلوب لكي يصدر SocialAPI endpoint secret بعد نجاح التحقق. تبقى الأحداث الأخرى خاضعة للتحقق HMAC داخل runtime عند ضبط `SOCIALAPI_WEBHOOK_SECRET` في بيئة العملية فقط.

## الحدود الحالية

هذا اختبار استقبال خارجي محدود. لا يثبت تشغيل Chatwoot أو إرسالًا إلى SocialAPI أو ردًا تلقائيًا أو LLM. كما أن عنوان HTTPS الحالي مؤقت ويتوقف عند انتهاء البيئة؛ يلزم نقل endpoint إلى مضيف دائم قبل الاعتماد التشغيلي.

## ما المقصود بأن إنشاء الرابط وتحققه عبر HTTPS اكتمل

لم يُستخدم Ngrok أو Cloudflare Tunnel أو نطاق مملوك. استُخدم proxy HTTPS مؤقت للبيئة يقدّم عنوانًا عامًا مشفّرًا ثم يمرر الطلبات إلى عملية API Mujeeb المحلية على المنفذ `8090`. شُغلت العملية فوق PostgreSQL 16 مؤقتة طُبقت عليها migrations المشروع من 1 إلى 45. العنوان العام ليس نشرًا إنتاجيًا ولا يستمر بعد انتهاء البيئة.

اختبرت لوحة SocialAPI هذا العنوان عند التسجيل. فشلت المحاولة الأولى باستجابة `502` لغياب خدمة مستمعة، ثم قُبلت بعد تشغيل Mujeeb. بعد ضبط endpoint secret في **ذاكرة عملية API فقط**، ظهرت في لوحة SocialAPI محاولة تسليم خارجية موقّعة لحدث `dm.received` واستقبلها Mujeeb بـ`202`. هذا يعني أن سلسلة الوصول التالية ثبتت:

```text
SocialAPI
  -> HTTPS public proxy المؤقت
  -> Mujeeb API :8090
  -> HMAC signature verification
  -> raw payload store + inbound event ledger
```

ولا يعني ذلك أن سلسلة المنتج كاملة أصبحت مثبتة. توقفت المعالجة الآمنة عند `unresolved` لعدم وجود `Business` و`ChannelConnection` وbinding مطابقين للـSocialAPI account الوارد. لذلك لم تنشأ Customer أو Conversation أو CommunicationMessage، ولم يُستدعَ Chatwoot أو LLM أو Outbox worker ولم يحدث إرسال رسالة.

| طبقة الاختبار | الحالة | الدليل |
| --- | --- | --- |
| HTTPS public reachability | مثبتة | قبول التسجيل بعد فشل اتصال أولي صريح |
| SocialAPI delivery | مثبتة | تسليم `dm.received` بحالة delivered واستجابة `202` |
| توقيع HMAC | مثبت | `signature_verified = true` في event ledger |
| رفض توقيع مزيف | مثبت | `401` من دون إضافة event ledger |
| tenant / connection resolution | غير مكتمل في بيئة الاختبار | لا يوجد binding مطابق، فأصبح الحدث `unresolved` |
| Customer / Conversation / Message materialization | غير مختبر حتى الآن | يتطلب resolution صحيح أولًا |
| Chatwoot / Auto-reply / LLM / إرسال مزود | غير مشغّل عمدًا | خارج نطاق اختبار الاستقبال الحالي |

## المصدر

المتطلبات موثقة في [دليل SocialAPI Webhooks](https://docs.social-api.ai/guides/webhooks): يلزم HTTPS، ويُرسل المزود verification ping عند إنشاء endpoint، ويُعاد secret مرة واحدة فقط، وتتطلب الأحداث العادية تحقق HMAC للـraw request body.

يتطلب mirror في Chatwoot استخدام **Application API** مع user `access_token` من حساب Chatwoot، وليس Client API أو Platform API. وتوثق Chatwoot أن عمليات إنشاء Contact ثم Conversation ثم Message تقع تحت `/api/v1/accounts/{account_id}/...` وتتطلب `api_access_token`. هذه المتطلبات مستقلة عن SocialAPI ولا يجعل تسجيل Webhook أي token لـChatwoot متاحًا تلقائيًا.

## اختبار Mujeeb → Chatwoot المنفذ

بعد إنشاء `ChannelConnection` نشطة ومطابقة لحساب SocialAPI الوارد ضمن business اختبارية معزولة، أرسلنا **test delivery موقّعًا** من لوحة SocialAPI لحدث `dm.received`. أعاد Mujeeb HTTP `202`، وظهر في PostgreSQL: Customer واحد وConversation واحدة وprovider ConversationReference واحدة وCommunicationMessage واحدة وinbound ledger واحد بحالة `processed/socialapi_materialized` مع `signature_verified = true`.

شغّل الاختبار بعد ذلك Chatwoot CE محليًا على `127.0.0.1` فقط، مع PostgreSQL وRedis منفصلين عنه وبقيم مؤقتة مولدة خارج المستودع. أُنشئ binding صريح بين business الاختبار وChatwoot Account/Inbox محليين، ثم عالج Worker Mujeeb مهمة mirror واحدة. انتهت المهمة بحالة `completed` و`result_code = chatwoot_mirrored` من المحاولة الأولى، وكتب Mujeeb Chatwoot message reference. أكدت قاعدة Chatwoot وجود Contact وConversation وMessage.

لا يثبت هذا الاختبار SocialAPI outbound أو رسالة فعلية إلى Facebook/Instagram/WhatsApp أو Chatwoot provisioning في الإنتاج أو LLM أو auto-reply. هذه الميزات بقيت معطلة؛ Chatwoot والـworker هنا تشغيل اختبار مؤقت ينتهي مع البيئة وليس استضافة تشغيلية مستمرة.

[1]: https://docs.social-api.ai/guides/webhooks "SocialAPI Webhooks"
[2]: https://developers.chatwoot.com/api-reference/introduction "Chatwoot API introduction"
[3]: https://developers.chatwoot.com/api-reference/contacts/create-contact "Chatwoot Create Contact"
[4]: https://developers.chatwoot.com/api-reference/conversations/create-new-conversation "Chatwoot Create Conversation"
[5]: https://developers.chatwoot.com/api-reference/messages/create-new-message "Chatwoot Create Message"
