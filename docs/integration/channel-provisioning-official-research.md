# Channel Provisioning — Official API Research

## Chatwoot

المصدر الرسمي لإنشاء Account هو:

- https://developers.chatwoot.com/api-reference/accounts/create-an-account

العقد المنشور يحدد `POST /platform/api/v1/accounts`، ويستخدم Platform App API key في `api_access_token`. صفحة Chatwoot توضح أن Platform APIs مخصصة لإدارة accounts/users/roles، وأنها متاحة فقط في **self-hosted Chatwoot installations**:

- https://developers.chatwoot.com/contributing-guide/chatwoot-platform-apis

المصدر الرسمي لإنشاء Inbox هو:

- https://developers.chatwoot.com/api-reference/inboxes/create-an-inbox

العقد يحدد `POST /api/v1/accounts/{account_id}/inboxes`. لإنشاء API inbox، يكون `channel.type=api`، ويمكن تمرير `webhook_url` و`hmac_mandatory`. لذلك provisioning التلقائي لـChatwoot Account/Inbox لا يُدّعى كحل عام للـCloud؛ adapter الحالي مخصص لمسار self-hosted Platform API، بينما Cloud يحتاج account/inbox جاهزًا أو مسارًا آخر تدعمه الخطة.

## SocialAPI

المصدر الرسمي لتدفق ربط الحسابات:

- https://docs.social-api.ai/guides/oauth

`POST /v1/accounts/connect` يبدأ managed platform OAuth، ويعيد `auth_url` و`state` في HTTP 202. بعد موافقة المستخدم يعيد SocialAPI redirect إلى `redirect_uri`:

- `status=success` مع `account_id`، أو
- `status=selection_required` مع `connection_id` في Facebook/بعض الحالات، ثم يُستكمل عبر `POST /v1/accounts/pending/{connection_id}/select`.

في Facebook يُرسل الاختيار عبر `page_ids`. SocialAPI يحتفظ بتوكنات المنصة، ولا يجب على Mujeeb رؤيتها أو تخزينها.

## حدود الإثبات

تم استخدام هذه المصادر لتصميم contracts وadapters فقط. لم يتم إنشاء Chatwoot Account/Inbox حقيقي، ولم يتم تشغيل SocialAPI OAuth أو provisioning خارجي في هذه الجولة. الاختبارات المقصودة محلية/fake إلى أن يقرر المستخدم تشغيل مسار حي بحسابات تجريبية وإعدادات self-hosted/Cloud مؤكدة.
