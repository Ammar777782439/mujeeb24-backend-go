# ملاحظة اختبار Chatwoot-origin callback

- أنشئ Chatwoot Self-Hosted محليًا Account 1 وAPI Inbox 1 باسم اختباري.
- Application API Contact/Conversation/Message أعاد HTTP 200 في اختبار سابق.
- عند استخدام loopback في Webhook URL، سجّل Chatwoot أن hostname 127.0.0.1 لا يملك public IP وأن الرسالة أصبحت failed.
- تم تغيير Webhook URL إلى HTTPS public proxy مؤقت لمنفذ Mujeeb 8080.
- channel_api metadata أظهر hmac_mandatory=false ووجود hmac token بطول فقط؛ لا تُحفظ قيمة token هنا.
- Mujeeb API يعمل على 127.0.0.1:8080، والـdatabase منفصلة على منفذ 55433.
- لم تُثبت بعد نتيجة callback بعد تغيير URL؛ يلزم إنشاء Message جديدة ومراقبة inbound_event_ledger وcustomers وconversations وcommunication_messages.

بعد تشغيل Mujeeb على `:8080`، أعاد proxy العام health=200. لكن callback إلى Mujeeb أعاد 401. وُجّه API Inbox مؤقتًا إلى receiver تشخيصي عام على منفذ منفصل، وقبل Chatwoot التحديث بنجاح. receiver مصمم ليرد 202 ويسجل metadata فقط: أسماء headers، أطوالها، طول body، وSHA-256 للـbody، دون raw payload أو قيم headers.

بعد توجيه API Inbox إلى receiver التشخيصي العام، أنشأ Chatwoot Message اختبارية جديدة عبر Application API بنجاح HTTP 200. رقم الرسالة المحلي الناتج هو 7. receiver لم يحفظ raw body أو قيم headers، وسأستخدم metadata المسجلة لتحديد صيغة التوقيع.

تشخيص المصدر أثبت أن Chatwoot `WebhookJob` يمرر `inbox.channel.secret` إلى HMAC، وليس `channel.hmac_token`. تم إعادة تشغيل Mujeeb باستخدام `channel_api.secret` داخل الذاكرة، ثم أُعيد callback URL إلى Mujeeb عبر HTTPS العام وحُفظ التحديث بنجاح. الاختبار التالي يجب أن ينشئ Message جديدة من Chatwoot ثم يفحص Event Ledger والكيانات الأربع.

دليل حاسم: بعد توجيه API Inbox إلى receiver العام، أنشأ Chatwoot Message اختبارية وأرسل callback فعليًا. receiver رأى `X-Chatwoot-Signature` و`X-Chatwoot-Timestamp` و`X-Chatwoot-Delivery`، وحسب HMAC على `timestamp.raw_body` باستخدام `channel_api.secret` داخل الذاكرة؛ النتيجة `signature_valid=true`. هذا يثبت أن secret الصحيح هو `channel_api.secret` وأن Chatwoot-origin callback والتوقيع يصلان عبر proxy.

ظل callback إلى Mujeeb يعيد 401 رغم تطابق secret داخل عملية Mujeeb، ما يشير إلى فجوة في مسار HTTP DTO/Huma أو اختلاف bytes/headers بين proxy وreceiver، وليس في Chatwoot signing نفسه. يجب تشخيص forwarded request داخل Mujeeb قبل ادعاء materialization PASS.

تمت إضافة دعم `created_at` الرقمي والنصي (RFC3339 وصيغ UTC) في Chatwoot normalizer، ونجحت اختبارات adapter/services/handlers. Probe Huma المؤقت أثبت أن raw body وX-Chatwoot headers تصل إلى dispatcher. أُعيد callback URL إلى Mujeeb بعد التصحيح، والخطوة التالية هي إرسال Message جديدة من Chatwoot وقياس materialization في قاعدة Mujeeb.

النتيجة النهائية للاختبار الحقيقي بعد إنشاء binding محلي لـaccount=1/inbox=1 وbusiness الاختباري: Chatwoot Application API أنشأ Contact/Conversation/Message بنجاح (Message ID محلي 23). Chatwoot أرسل callback فعليًا إلى Mujeeb عبر HTTPS، وMujeeb materialized النتيجة في PostgreSQL: `inbound_event_ledger=1`, `customers=1`, `conversation_references=1`, `communication_messages=1`, `processing_state=processed`, وmessage reference count=1. هذه أول PASS فعلية لمسار Chatwoot-origin → Mujeeb-owned records. سجلات Chatwoot الأقدم التي تعرض 503 تخص محاولات سابقة قبل إصلاح decoder/binding؛ لا تُستخدم كحكم على الطلب النهائي.
