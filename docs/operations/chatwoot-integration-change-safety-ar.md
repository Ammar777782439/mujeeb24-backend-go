# حواجز تغيير تكامل Chatwoot وSocialAPI

> **القاعدة التشغيلية:** لا يُعلن نجاح المسار إلا بعد إثبات `event → CommunicationMessage → mirror job completed → Chatwoot message`، وفشل الاختبارات أو فحص الأسرار يمنع الدمج.

## مصادر العقد الخارجي

عند تعديل Chatwoot، يُراجع التوثيق الرسمي قبل تغيير أي adapter. إنشاء Contact يجعل `identifier` معرّفًا خارجيًا فريدًا، ويعمل البحث عبر `GET /api/v1/accounts/{account_id}/contacts/search?q=...` ويعيد قائمة Contacts. لذلك يعالج العميل فقط `422` الذي يثبت أنه تكرار للـidentifier ثم يبحث عن contact مطابق داخل الـinbox المطلوب؛ لا يجوز تحويل كل أخطاء `422` إلى نجاح.

أما provisioning، فيُنشأ Account بواسطة Platform API، ثم يُضاف مستخدم Application API إلى الـAccount عبر Platform API، وبعدها يُنشأ API Inbox باستخدام Application API. يجب أن يكون user ID إعدادًا صريحًا، لا رقمًا ثابتًا في المصدر، ويجب ألا يُتجاهل فشل إضافة المستخدم.

## قواعد لا تُكسر

| المجال | القاعدة | الحاجز المطلوب |
|---|---|---|
| Route key | `cw_<connection_uuid_without_hyphens>` هو identity callback للقناة. | اختبار PostgreSQL يربط provisioning binding ثم يشغّل mirror resolve على نفس connection ID. |
| Webhook | لا يُقبل callback إلا مع HMAC صحيح وبـroute key وaccount/inbox مطابقين للـbinding النشط. | اختبارات webhook وtenant isolation. |
| Contact idempotency | فقط duplicate `identifier` المحدد يعيد contact موجودًا من الـinbox نفسه. | اختبار duplicate ناجح واختبار 422 غير متعلق يفشل ولا يبحث. |
| External results | لا retry أعمى عند نتيجة مجهولة من Chatwoot أو SocialAPI. | dead-letter مع failure code قابل للتدقيق. |
| Provisioning credentials | Platform token وApplication token وApplication user ID ثلاثة مدخلات مستقلة. | فشل configuration عند غياب أي منها عند تمكين provisioning. |
| أسرار التطوير | يمنع وضع المفاتيح أو أدوات توليدها في جذر المشروع أو في ملفات tracked. | فحص قبل الدمج ومنع `generate-keys.go` و`testjwt.go`. |

## بوابة التغيير قبل الدمج

تُشغّل بوابة التحقق محليًا وفي CI قبل أي دمج عبر `scripts/verify-repository.sh`:

1. `gofmt` دون فرق.
2. `go test ./...` و`go vet ./...`.
3. اختبارات PostgreSQL الموسومة `integration` ضد PostgreSQL 16 نظيفة.
4. فحص `git diff --check` وتسرب الأسرار وملفات أدوات التطوير المحظورة.
5. عند تغيير adapter خارجي: اختبار HTTP contract باستخدام `httptest` لكل رد نجاح، duplicate، ورفض ذي صلة.

للتحقق الكامل محليًا يُشغّل `RUN_INTEGRATION=1 POSTGRES_TEST_DSN=... bash scripts/verify-repository.sh` ضد PostgreSQL 16 فارغة ومؤقتة. لا تستخدم هذه البوابة مفاتيح SocialAPI أو Chatwoot ولا تتصل بالخدمات الخارجية.

## نطاق الإثبات

يُسجّل كل تغيير في وثيقة التشغيل أو PR مع أربع حالات منفصلة: **اختبار وحدة، اختبار PostgreSQL، دليل تشغيل محلي، دليل خارجي**. لا يثبت ظهور Inbox وحده materialization، ولا يثبت وصول event وحده نجاح mirror.

## المراجع

[1] Chatwoot، [Create Contact](https://developers.chatwoot.com/api-reference/contacts/create-contact).

[2] Chatwoot، [Search Contacts](https://developers.chatwoot.com/api-reference/contacts/search-contacts).

[3] Chatwoot، [Create an Account User عبر Platform API](https://developers.chatwoot.com/api-reference/account-users/create-an-account-user).
