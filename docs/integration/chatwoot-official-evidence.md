# Chatwoot Official Evidence

## API Channel وWebhooks — 2026-08-25

توضح صفحة Chatwoot الرسمية الخاصة بـAPI Channel أن إنشاء API inbox يتطلب اسمًا وcallback URL وإضافة agents، وأن تدفق الاختبار هو Contact ثم Conversation ثم Message. كما تنص على أن إنشاء Message في API Channel يرسل callback إلى URL المحدد، وأن API Channel يولّد secret للتحقق من payload. [1]

توضح وثيقة Webhooks الرسمية أن callback الموقّع يستخدم `X-Chatwoot-Signature` و`X-Chatwoot-Timestamp` و`X-Chatwoot-Delivery`، وأن التوقيع هو HMAC-SHA256 على `{timestamp}.{raw_request_body}` باستخدام secret الخاص بالـwebhook، مع ضرورة استخدام raw bytes وعدم إعادة serializing JSON. [2]

في الاختبار المحلي، ظهر `channel_api.hmac_token` كإعداد للقناة، بينما callback من API Inbox بعد استخدام عنوان loopback فشل بسبب منع Chatwoot hostname غير العام. بعد استعمال عنوان HTTPS العام، وصل الطلب إلى Mujeeb لكنه أعاد 401، ما يثبت أن channel secret/contract يجب مواءمته مع التوقيع الفعلي ولا يجوز افتراض أن channel hmac token يساوي account webhook secret.

[1]: https://www.chatwoot.com/hc/user-guide/articles/1677839703-how-to-create-an-api-channel-inbox
[2]: https://www.chatwoot.com/hc/user-guide/articles/1677693021-how-to-use-webhooks
