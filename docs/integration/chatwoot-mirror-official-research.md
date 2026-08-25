# Chatwoot mirror — official API notes

تمت مراجعة التوثيق الرسمي في 25 أغسطس 2026 لاستخدامه في تصميم `SocialAPI inbound → Chatwoot mirror`.

| العملية | المصدر الرسمي | الدليل المؤثر على التصميم |
|---|---|---|
| إنشاء Contact | https://developers.chatwoot.com/api-reference/contacts/create-contact | endpoint account API يستخدم `api_access_token` ويوثق `identifier` كمعرّف خارجي فريد للـContact. يستخدم Mujeeb identifier ثابتًا مشتقًا من business/external user عند إنشاء mirror. |
| إنشاء Conversation | https://developers.chatwoot.com/api-reference/conversations-api/create-a-conversation | التوثيق يصف إنشاء conversation، لكنه لا يوثق idempotency key عامة لمسار create account API الذي يستخدمه adapter الحالي. |
| إنشاء Message | https://developers.chatwoot.com/api-reference/messages-api/create-a-message | واجهة public API توثق `echo_id` كمعرف مؤقت يعاد عبر websockets؛ adapter الحساب الحالي لا يحمل idempotency key موثقة لإنشاء الرسالة. |

## القرار

لا ينفذ Mujeeb Chatwoot HTTP داخل PostgreSQL transaction. بعد materialization، ينشئ `chatwoot_mirror_jobs` durable obligation. إذا فشلت network call أو أصبحت نتيجتها غير معروفة، ينتقل job إلى `dead_letter` بدلاً من retry أعمى قد يكرر Contact أو Conversation أو Message.

هذه المراجع لا تثبت تشغيلًا حيًا. لم تُنشأ أي موارد Chatwoot خارجية في هذه الدفعة.
