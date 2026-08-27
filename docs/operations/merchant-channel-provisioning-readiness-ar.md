# جاهزية Merchant Channel Provisioning

## النتيجة الحالية

يوجد في Mujeeb مسار برمجي مكتمل على مستوى **العقد والتخزين والتركيب المشروط** لربط قناة تاجر عبر SocialAPI ثم Chatwoot. لا يصح اعتباره ربطًا حيًا بعد، لأن تشغيله يتطلب OAuth حقيقيًا من التاجر، ومفاتيح تشغيل محلية، وعنوان HTTPS ثابت أو مؤقت لمسارات callback والـwebhook.

## دورة الربط المعتمدة

```text
POST /businesses/{business_id}/channel-connections
        ↓
إنشاء provisioning session ثابتة بـ idempotency key
        ↓
POST SocialAPI /v1/accounts/connect
        ↓
ينتقل التاجر إلى موافقة المنصة
        ↓
GET /oauth/socialapi/callback في Mujeeb
        ↓
إنشاء ChannelConnection بحالة pending
        ↓
إنشاء Chatwoot Account وAPI Inbox
        ↓
حفظ route binding داخل Mujeeb
        ↓
تفعيل ChannelConnection وتسجيل connected
```

لا توجد عملية شبكة داخل PostgreSQL transaction. تسجل الجلسة والاتصال والحالة المتاحة في Mujeeb، وتتحول أي نتيجة جزئية إلى `failed` مع `failure_code` بدل نجاح غير صحيح.

## ما أثبته الكود

| المجال | الحالة | الدليل البرمجي |
|---|---|---|
| جلسة الربط والحالات | منفذ | `ChannelProvisioningService` و`ChannelProvisioningStore` |
| idempotency لكل business | منفذ | مفتاح `(business_id, idempotency_key)` في store واختبار PostgreSQL |
| بداية SocialAPI OAuth | منفذ | `SocialChannelProvisioner.BeginAuthorization` عبر `/v1/accounts/connect` |
| callback بعد موافقة التاجر | منفذ | `/oauth/socialapi/callback` في `internal/bootstrap/api.go` |
| Facebook/Page selection | منفذ بالعقد | `connection_id` و`page_ids` و`ResolveAuthorization` |
| ChannelConnection | منفذ | إنشاء pending ثم activation بعد نجاح جميع الخطوات |
| Chatwoot Account/API Inbox | منفذ بالعقد | `PlatformClient.EnsureAccount` و`EnsureInbox` |
| route binding متعدد المستأجرين | منفذ | `chatwoot_workspace_bindings` مع conflict guard |

## تصحيح callback متعدد التجار

كان تمرير `CHANNEL_PROVISIONING_WEBHOOK_URL` كعنوان كامل ثابت إلى كل Chatwoot API Inbox غير كافٍ؛ لأن مستقبِل Chatwoot في Mujeeb يتحقق من ثلاث قيم معًا: `route_key` وChatwoot `account_id` و`inbox_id`. تم تصحيح هذا قبل تفعيل provisioning الحي.

ينشئ Mujeeb الآن `route_key` من مقطع واحد خاص بكل `ChannelConnection` بالشكل `cw_{connection_uuid_without_hyphens}`. يأخذ `CHANNEL_PROVISIONING_WEBHOOK_URL` كعنوان أساس HTTPS فقط، ثم ينشئ رابط API Inbox النهائي تلقائيًا:

```text
https://YOUR-HOST/api/v1/webhooks/chatwoot/cw_{connection_uuid_without_hyphens}
```

لا يشترك تاجران في route key، ولا يقبل المستقبِل callback إذا لم تطابق قيم `route_key` وAccount وInbox binding مسجلًا ونشطًا في business نفسه. يمرر proxy Compose الموحّد فقط مسارات SocialAPI inbound وSocialAPI OAuth callback وChatwoot callbacks، ويبقي بقية API خارج الإنترنت العام.

## إعدادات التشغيل اللازمة

لا تدخل أي قيمة هنا إلى Git. تُحفظ في `deploy/local/.env` أو مدير أسرار إنتاجي فقط.

| المتغير | الغرض | مطلوب قبل البدء الحي |
|---|---|---|
| `CHATWOOT_PROVISIONING_ENABLED=true` | يفعّل endpoint بداية الربط | نعم |
| `SOCIALAPI_API_KEY` | يستدعي SocialAPI لإنشاء OAuth URL | نعم |
| `CHATWOOT_PLATFORM_API_TOKEN` | ينشئ Chatwoot Account وAPI Inbox | نعم |
| `CHANNEL_PROVISIONING_REDIRECT_URI` | عنوان HTTPS لمسار `/oauth/socialapi/callback` | نعم |
| `CHANNEL_PROVISIONING_WEBHOOK_URL` | عنوان HTTPS لمسار Chatwoot callback | نعم |
| `SOCIALAPI_WEBHOOK_SECRET` | يتحقق من inbound SocialAPI events | نعم قبل استقبال الرسائل |
| `CHATWOOT_API_TOKEN` | يحتاجه mirror worker لإرسال Mujeeb records إلى Chatwoot | لاحقًا عند تفعيل mirror |

## قيود ينبغي إثباتها قبل التشغيل الحي

يتطلب Chatwoot Platform API `access_token` صادرًا من Platform App في Super Admin Console، ولا تملك Platform API افتراضيًا الوصول إلى حسابات أو مستخدمين أنشأت من واجهة Chatwoot أو من token مختلف. لذلك يجب أن ينشئ provisioning موارد Chatwoot بنفس Platform App، كما يفعل المسار الحالي.[2]

يوصي SocialAPI باستخدام `state` كقيمة correlation يملكها Mujeeb، ويتطلب `redirect_uri` مطابقًا لقائمة العناوين المسموح بها إذا تم تسجيل واحدة. Instagram يعيد `account_id` مباشرة عند النجاح؛ أما Facebook فيعيد اتصالًا معلقًا ويتطلب اختيار Page قبل إنشاء الحساب.[1]

> لا نبدأ OAuth أو ننشئ Account أو Inbox في خدمات خارجية لمجرد أن الكود موجود. هذه عمليات تغيّر موارد خارجية وتحتاج موافقة صريحة للتنفيذ عند جاهزية إعدادات التشغيل.

## مراجع

[1]: https://docs.social-api.ai/guides/oauth "SocialAPI OAuth Flows"
[2]: https://developers.chatwoot.com/api-reference/introduction "Chatwoot API introduction"
[3]: https://www.chatwoot.com/hc/user-guide/articles/1677839703-how-to-create-an-api-channel-inbox "Chatwoot API Channel Inbox"
