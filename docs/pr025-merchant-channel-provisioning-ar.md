# PR-025 — Merchant Channel Provisioning

## الحكم

أصبح لدينا مسار Application وHTTP يبدأ من زر `Connect` ويُنشئ جلسة provisioning idempotent، ثم ينتظر OAuth callback، وبعد نجاحه ينفذ Chatwoot Account/Inbox provisioning لمسار **self-hosted Chatwoot Platform API**، ويحفظ Chatwoot workspace binding وChannelConnection في Mujeeb.

هذا لا يعني أن provisioning الخارجي شُغّل حيًا. الاختبارات كلها محلية أو fake، ولم يُستخدم SocialAPI أو Chatwoot حقيقيان في هذه الدفعة.

## الحدود الخارجية المثبتة

Chatwoot Platform APIs لإنشاء accounts متاحة فقط في self-hosted installations وفق الوثائق الرسمية:

- https://developers.chatwoot.com/contributing-guide/chatwoot-platform-apis
- https://developers.chatwoot.com/api-reference/accounts/create-an-account
- https://developers.chatwoot.com/api-reference/inboxes/create-an-inbox

SocialAPI managed OAuth يعيد `auth_url` و`state`، ثم يعيد إلى `redirect_uri` إما `status=success&account_id=...` أو `status=selection_required&connection_id=...`. Facebook قد يحتاج `page_ids` عبر pending selection:

- https://docs.social-api.ai/guides/oauth

لم يتم افتراض قدرة Cloud Chatwoot على إنشاء Account تلقائيًا. عند غياب Platform API token أو عند استخدام Cloud، provisioning يُرفض آمنًا بدل إنشاء mapping وهمي.

## State machine

```text
pending_authorization
        ↓ OAuth callback
provisioning
        ↓
connected
```

وعند الفشل:

```text
pending_authorization/provisioning
        ↓
failed
```

إذا أنشئ Account خارجيًا ثم فشل إنشاء Inbox أو binding، تُحفظ IDs الجزئية وfailure code داخل الجلسة. لا توجد محاولة حذف تعويضية عمياء؛ التنظيف الخارجي يحتاج سياسة مستقلة ومؤكدة.

## Application contracts

أضيفت ports typed التالية:

- `ChannelProvisioningStore`
- `SocialChannelProvisioner`
- `WorkspaceProvisioner`
- `ChatwootWorkspaceBindingWriter`
- `ChannelConnectionWriter`

الـApplication لا يرى Chatwoot أو SocialAPI DTOs. يستقبل `SocialAuthorizationCallback` و`WorkspaceAccount/WorkspaceInbox` فقط.

## Persistence

Migration الجديدة هي:

```text
000037_channel_provisioning_sessions.up.sql
```

وتحفظ:

- business وidempotency key
- provider/channel/display name
- status وOAuth state
- authorization URL
- provider account/connection references
- Chatwoot account/inbox IDs الداخلية للتشغيل
- ChannelConnection ID
- failure code

أُضيفت uniqueness على `(business_id, idempotency_key)` وactive channel، وعزل business وFK إلى `businesses` و`channel_connections`. لم تُعدّل migrations القديمة.

## Runtime wiring

`POST /businesses/{business_id}/channel-connections` أصبح يبدأ provisioning ويعيد `ChannelProvisioning` لا `ChannelConnection`:

```json
{
  "data": {
    "provisioning_id": "...",
    "business_id": "...",
    "provider": "socialapi",
    "channel": "facebook",
    "status": "pending_authorization",
    "authorization_url": "https://..."
  }
}
```

وعند تفعيل provisioning صراحةً، يضاف callback:

```text
GET /oauth/socialapi/callback
```

ويقبل `state`, `status`, `platform`, `account_id`, `connection_id`, `page_id/page_ids`.

يظل `CHANNEL_PROVISIONING_ENABLED=false` افتراضيًا.

## Adapters

### SocialAPI

`ProvisioningAdapter` يستخدم `BeginConnection` ويحوّل OAuth success أو Facebook selection إلى typed authorization. لا يرى Mujeeb أي platform token؛ SocialAPI يحتفظ به وفق عقده.

### Chatwoot

`PlatformClient` ينفذ:

```text
POST /platform/api/v1/accounts
POST /api/v1/accounts/{account_id}/inboxes
```

ويُنشئ API Inbox مع HTTPS webhook وHMAC mandatory. هذا adapter مشروط بـ`CHATWOOT_PLATFORM_API_TOKEN` لمسار self-hosted فقط.

## الاختبارات الفعلية

| الاختبار | النتيجة |
|---|---|
| `go test ./...` | PASS |
| `go vet ./...` | PASS |
| OpenAPI generation/drift | PASS |
| schema runner: applied 37 ثم 0 | PASS |
| provisioning idempotency | PASS |
| OAuth success callback | PASS |
| Facebook selection contract | PASS |
| partial failure وعدم activation | PASS |
| Chatwoot Account/Inbox HTTP contract fake | PASS |
| SocialAPI OAuth HTTP contract fake | PASS |
| provisioning store وbinding على PostgreSQL 16 حقيقية | PASS |
| tenant isolation | PASS |
| `git diff --check` وsecret scan | PASS |

## ما لم يُثبت

لم يتم إجراء provisioning حي في SocialAPI، ولم يتم إنشاء Chatwoot Account أو Inbox حقيقي، ولم يتم تسجيل credential أو إرسالها إلى GitHub. Cloud Chatwoot لا يدعم هذا المسار عبر Platform API وفق المصدر الرسمي، لذلك يحتاج Cloud إلى حساب/Inbox جاهز أو حل provisioning مختلف معتمد.

كما أن زر Dashboard الفعلي لا يزال يعتمد على Auth/Membership production scope الموجود في HTTP boundary؛ الكود الآن يملك endpoint provisioning، لكن التشغيل التجاري يحتاج إعداد JWT/roles وdomain/HTTPS وworker وsecret manager.

## المراجع

1. [Chatwoot Platform APIs](https://developers.chatwoot.com/contributing-guide/chatwoot-platform-apis)
2. [Chatwoot Create an Account](https://developers.chatwoot.com/api-reference/accounts/create-an-account)
3. [Chatwoot Create an Inbox](https://developers.chatwoot.com/api-reference/inboxes/create-an-inbox)
4. [SocialAPI OAuth Flows](https://docs.social-api.ai/guides/oauth)
