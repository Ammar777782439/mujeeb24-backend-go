# دورة حياة Mujeeb 24 الكاملة — من Dashboard إلى كل الخدمات

## الخلاصة أولًا

عند تشغيل المشروع الآن، **لن يعمل كل شيء live تلقائيًا**. الذي يعمل ومثبت هو جزء كبير من الـbackend والـpersistence والـcontracts والاختبارات المحلية، لكن توجد مراحل خارجية تحتاج إعدادًا وتشغيلًا واعتمادًا منفصلًا، وبعض الوصلات لم تُنفذ بعد.

القاعدة المعمارية التي نعتمدها هي:

```text
Dashboard الخاص بـMujeeb
        ↓
Mujeeb API / Application
        ↓
Mujeeb PostgreSQL = source of truth
        ↓
Chatwoot = internal workspace / mirror
        ↓
SocialAPI = external transport
        ↓
Facebook / Instagram / WhatsApp
```

> **Mujeeb لا يقرأ Customer أو Conversation أو CommunicationMessage من Chatwoot عند عرض Dashboard.** هذه السجلات مملوكة لـMujeeb وتُقرأ من PostgreSQL. Chatwoot خدمة داخلية تحت الغطاء، أما SocialAPI فهو وسيلة النقل الخارجية.

## 1. دورة ربط القناة من Dashboard

عندما يضغط التاجر `Connect WhatsApp` أو `Connect Instagram` في Dashboard، لا ينبغي أن يدخل التاجر إلى Chatwoot أو ينسخ Account ID وInbox ID يدويًا.

### التدفق المستهدف

```text
Merchant Dashboard
  → POST /businesses/{business_id}/channel-connections
  → JWT + business scope
  → Mujeeb ينشئ provisioning session idempotent
  → Mujeeb يبدأ SocialAPI OAuth
  → Dashboard يوجّه التاجر إلى auth_url
  → SocialAPI يعيد OAuth callback إلى Mujeeb
  → Mujeeb يتحقق من state
  → Mujeeb ينشئ Chatwoot Account وAPI Inbox في self-hosted Chatwoot
  → Mujeeb ينشئ Chatwoot workspace binding
  → Mujeeb يحفظ ChannelConnection
  → الحالة connected
```

### ما يملكه كل نظام

| العنصر | مالكه | ما يُحفظ في Mujeeb |
|---|---|---|
| OAuth state وprovisioning session | Mujeeb | session ID، idempotency key، status، provider refs، failure code |
| SocialAPI token | SocialAPI/secret management | reference فقط، وليس raw token في Dashboard |
| Mujeeb ChannelConnection | Mujeeb | business، provider، channel، account ref، connection ref، status |
| Chatwoot Account/Inbox | Chatwoot self-hosted | IDs داخلية تُحفظ في workspace binding |
| الربط بين Chatwoot وMujeeb | Mujeeb | business، route، account، inbox، channel، active |

### الحالة الحالية لهذه المرحلة

| الجزء | الحالة |
|---|---|
| HTTP begin endpoint | منفذ |
| idempotent provisioning session | منفذ ومختبر |
| OAuth state/callback contract | منفذ ومختبر بعقد fake |
| SocialAPI direct success وselection-required contract | مختبر بعقد fake؛ لا live claim |
| Chatwoot self-hosted Account/Inbox adapter | مختبر بعقد fake؛ لا live claim |
| binding وChannelConnection persistence | مختبر على PostgreSQL 16 |
| إنشاء حسابات حقيقية | **NOT RUN** |
| Chatwoot Cloud auto-provisioning | **غير مدعوم بهذا التصميم**؛ Platform API المقصود self-hosted |
| Dashboard frontend الفعلي | **ليس مغلقًا ضمن هذه الدفعات** |

إذًا زر Connect موجود على مستوى backend contract، لكن لا نقول إن onboarding الحي مكتمل حتى نجهز self-hosted Chatwoot Platform App، وSocialAPI redirect URI، وHTTPS عام، وsecrets خارج Git، ثم نأخذ إذنًا لاختبار حي آمن.

## 2. دورة استقبال SocialAPI event

هذه هي القطعة التي أُغلقت في PR-026 على مستوى local/PostgreSQL.

```text
SocialAPI webhook
  → HTTPS endpoint في Mujeeb
  → HMAC verification
  → normalize provider event
  → store exact raw payload + SHA-256
  → resolve account إلى active ChannelConnection
  → EventStore.RecordIfAbsent
  → dedupe حسب provider/account/event
  → ProviderInboundStore transaction
  → external identity
  → Customer
  → provider ConversationReference
  → Conversation
  → CommunicationMessage
  → ledger = processed
```

### ماذا يحدث داخل Mujeeb

عند وصول `dm.received` مثلًا:

1. يتحقق Mujeeb من توقيع SocialAPI على raw body.
2. يحوّل `dm.received` إلى `interaction_received`، بدل تخزين اسم provider غير المقبول في enum الداخلي.
3. يحفظ raw bytes داخليًا في `inbound_webhook_payloads`، ويضع reference opaque في Event Ledger.
4. يبحث عن ChannelConnection نشطة باستخدام `provider_account_ref` داخل tenant الصحيح.
5. يسجل الحدث في `inbound_event_ledger`. إذا تكرر نفس event، لا ينشئ event جديدًا منطقيًا.
6. يستخدم `external_identities` لربط `(connection_id, external_user_id)` بـCustomer.
7. ينشئ Conversation إذا لم يوجد provider conversation reference حالي.
8. ينشئ ConversationReference بالقيم:

```text
system         = provider
provider_ref   = socialapi
resource_type  = conversation
resource_id    = SocialAPI conversation ID
connection_id  = Mujeeb ChannelConnection ID
is_current     = true
mapping_status = active
```

9. ينشئ CommunicationMessage مملوكًا لـMujeeb باتجاه `inbound` ومصدر `customer` ونقل `provider`.
10. يضع Event Ledger في `processed` بنتيجة `socialapi_materialized`.

### ما تم إثباته

| الحالة | الدليل |
|---|---|
| normalization وHMAC | unit/contract tests |
| raw payload idempotency | PostgreSQL integration |
| Customer/identity creation | PostgreSQL integration |
| Conversation/reference creation | PostgreSQL integration |
| CommunicationMessage creation | PostgreSQL integration |
| duplicate callback | PostgreSQL integration؛ لا records إضافية |
| tenant mismatch | PostgreSQL integration؛ conflict قبل duplicate return |
| rollback | PostgreSQL integration |
| SocialAPI live webhook | **NOT RUN** |

## 3. أين يدخل Chatwoot؟

هناك مساران يجب عدم خلطهما.

### المسار الأول: Chatwoot callback inbound

هذا المسار منفذ ومثبت محليًا وعلى PostgreSQL، كما تم تشغيل callback حقيقي من Chatwoot self-hosted في الدفعة السابقة:

```text
Chatwoot message_created
  → signed callback إلى Mujeeb
  → HMAC verification
  → message_type normalization
  → ChatwootInboundStore transaction
  → Customer
  → Conversation
  → Chatwoot ConversationReference
  → CommunicationMessage
  → Event Ledger processed
```

عند `message_type=incoming` يمكن للمسار الحالي، إذا كان AutoReply مفعّلًا وprovider reference صالحًا، أن يكمل:

```text
Chatwoot inbound
  → exact provider reference lookup
  → AutoReply bridge
  → AIDecision
  → OutboundMessage
  → Outbox
```

أما `outgoing` أو `private` أو `activity` أو `template` أو النوع المفقود، فيُحفظ حسب العقد ولا يشغل AutoReply، لمنع echo loop.

### المسار الثاني: SocialAPI inbound إلى Chatwoot

هذا هو الجزء الذي لا يزال ناقصًا بعد PR-026:

```text
SocialAPI inbound
  → Mujeeb materialization
  → Chatwoot Create Contact/Conversation/Message
```

PR-026 **لا ينفذ هذه المكالمة**. السبب مقصود: Chatwoot network call لا يجوز أن تكون داخل transaction التي تنشئ Customer وConversation وCommunicationMessage.

لذلك نحتاج PR مستقلًا يحتوي على:

```text
Mujeeb committed materialization
  → durable mirror obligation
  → worker خارج DB transaction
  → Chatwoot contact/conversation/message
  → idempotent mirror reference
  → retry أو unknown outcome quarantine
  → Chatwoot callback echo suppression
```

لا يجوز أن نرسل إلى Chatwoot ثم نكتشف أن قاعدة Mujeeb فشلت، ولا أن نعيد الإرسال أعمى بعد timeout غير معروف.

## 4. دورة AI والرد التلقائي

المسار الحالي المثبت للـAI يبدأ من Chatwoot inbound بعد materialization وprovider reference resolution:

```text
CommunicationMessage
  → ContextBuilder
      → Business
      → Customer context محدود
      → Conversation
      → recent message history
      → Catalog / Offer / Variant
      → Knowledge evidence
      → Business Policy evidence
  → AIRuntime
      → Safe runtime في الاختبارات أو OpenAI-compatible LLM runtime
  → structured proposal
  → application validation
  → Policy Engine
  → AIDecision
  → إذا كان ANSWER مسموحًا
  → OutboundMessage
  → OutboxEntry
```

النموذج لا يقرأ SQL ولا يستدعي SocialAPI أو Chatwoot ولا ينشئ Outbox. هو يعيد proposal مهيكلًا، وMujeeb هو الذي يقرر هل الرد مسموح.

### الحالة الحالية

| الجزء | الحالة |
|---|---|
| Safe auto-reply runtime | PASS محليًا |
| OpenAI-compatible runtime | PASS بعقد وsmoke صناعي اصطناعي؛ ليس بيانات عميل |
| ContextBuilder من PostgreSQL | PASS |
| Catalog grounding | PASS |
| Knowledge/Policy grounding | PASS ضمن النطاق الحالي |
| policy gate الأساسي | PASS |
| Chatwoot inbound إلى AutoReply إلى Outbox | PASS محليًا وعلى PostgreSQL |
| SocialAPI inbound إلى AI مباشرة | **غير موصول كمسار مستقل بعد** |
| LLM production data | **NOT RUN** |
| تفعيل LLM افتراضيًا | **مغلق**؛ `LLM_ENABLED=false` |
| تفعيل Chatwoot AutoReply افتراضيًا | **مغلق**؛ `CHATWOOT_AUTOREPLY_ENABLED=false` |

## 5. دورة OutboundMessage وOutbox والـWorker

عندما يقرر Mujeeb إرسال رد:

```text
AIDecision = ANSWER
  → OutboundMessage pending
  → OutboxEntry pending
  → worker polling
  → claim lease
  → exact provider reference resolver
  → ChannelConnection validation
  → SocialAPI SendMessage خارج DB transaction
  → provider result
  → Outbox completed أو failure/unknown quarantine
```

الـresolver لا يخمن SocialAPI conversation ID من Chatwoot ID. يجب أن يجد ConversationReference حالية، active، ومطابقة للـbusiness والـconnection والـprovider/account.

إذا أعاد network call نتيجة غير معروفة، لا يُعاد الإرسال تلقائيًا؛ تُحفظ حالة `unknown` حتى reconciliation أو قرار آمن.

### الحالة الحالية

| الجزء | الحالة |
|---|---|
| OutboundMessage persistence | PASS |
| Outbox persistence | PASS على PostgreSQL |
| claim/lease fencing | PASS |
| resolver exact binding | PASS محليًا |
| worker polling/runtime boundary | منفذ ومختبر ضمن worker tests |
| fake provider delivery | PASS |
| SocialAPI live send | **NOT RUN** |
| Facebook/Instagram/WhatsApp real reply | **NOT RUN** |
| exactly-once over network | **لا ندعيه** |
| Chatwoot mirror outbound | **غير منفذ** |

## 6. Delivery status والرسائل اللاحقة

SocialAPI status events مثل `dm.status.delivered` أصبحت تُطبع إلى `delivery_status_changed`، لكنها ليست رسالة inbound جديدة. ما يزال يلزم مسار مستقل يربط status event بـOutboundMessage ويحدّث delivery state بأمان:

```text
SocialAPI delivery status webhook
  → verify + normalize
  → resolve outbound/provider message
  → dedupe
  → update OutboundMessage status
  → audit/reconciliation
```

هذه ليست نفس عملية إنشاء CommunicationMessage، ولا ينبغي أن تنشئ Customer أو Conversation جديدة.

## 7. ماذا يرى التاجر في Dashboard؟

Dashboard لا يتعامل مباشرة مع SocialAPI أو Chatwoot. القراءة الصحيحة هي:

```text
Dashboard
  → Mujeeb authenticated API
  → business scope
  → Mujeeb PostgreSQL repositories
  → DTO projection
```

في timeline، يقرأ التاجر CommunicationMessage من Mujeeb، مع references وstatus projection. بالنسبة للرسالة outbound، status المعروض يأتي من OutboundMessage lifecycle. بالنسبة للرسالة inbound، `received` تعني أنها وصلت وسُجلت، ولا تعني أن ردًا أُرسل.

المطلوب لاحقًا في Dashboard هو إظهار حالات عملية مثل:

| الحالة | معناها |
|---|---|
| `connected` | ChannelConnection وbinding جاهزان حسب runtime state |
| `pending_authorization` | التاجر لم يكمل OAuth |
| `selection_required` | SocialAPI يحتاج اختيار صفحة/حساب |
| `provisioning` | Mujeeb يهيئ Chatwoot self-hosted resources |
| `failed` | فشل محفوظ بسبب typed failure code |
| `received` | رسالة inbound موجودة في timeline |
| `pending` | نية outbound محفوظة وتنتظر worker |
| `accepted/sent/delivered/read` | مراحل delivery التي أثبتها provider |
| `unknown` | نتيجة network غير محسومة؛ لا تعني نجاحًا ولا فشلًا |

## 8. ما الناقص بالترتيب الصحيح؟

### الأول: Chatwoot mirror من SocialAPI inbound

هذا هو أول نقص مباشر بعد PR-026. يحتاج عقدًا مستقلًا للـmirror، durable obligation، worker خارج transaction، idempotency، وunknown outcome handling.

### الثاني: Delivery status materialization

يجب تحويل SocialAPI status callbacks إلى تحديث آمن لـOutboundMessage بدل أن تُعامل lifecycle events كرسائل.

### الثالث: SocialAPI live operations

يحتاج ذلك إلى API key صالح خارج Git، public HTTPS، webhook registration، redirect URI، test account/channel، وموافقة صريحة على اختبار خارجي. لم يُنفذ في هذه الجولة.

### الرابع: Chatwoot provisioning الحي

PR-025 يثبت adapter والعقد محليًا، لكن التشغيل الحي يحتاج self-hosted Platform API وauth path صحيحًا لإنشاء Account وInbox. Chatwoot Cloud ليس بديلًا تلقائيًا لهذا المسار.

### الخامس: تشغيل المنتج التجاري

يبقى توصيل Dashboard frontend الفعلي، إدارة المستخدمين/الأدوار، secrets manager، monitoring، retention للـraw payloads، worker دائم، retries/reconciliation، health checks، وعمليات backup/restore والإطلاق.

## 9. الإجابة المباشرة: ماذا سيعمل إذا ثبّت المشروع وشغّلته الآن؟

سيعمل الآتي بحسب الإعدادات:

```text
PostgreSQL migrations
Mujeeb repositories
HTTP contracts
Health endpoints
Local/fake provider contracts
Chatwoot inbound materialization إذا رُكبت بيئة Chatwoot وsecrets المطلوبة
Chatwoot inbound AutoReply إذا فُعّل flag ووُصل LLM
SocialAPI inbound materialization إذا فُعّل webhook ووفرت credentials/HTTPS
Outbox worker إذا شُغل process ووصل provider
```

لكن لن يحدث تلقائيًا:

```text
لن ينشئ حسابات SocialAPI أو Chatwoot حقيقية دون config وOAuth حي
لن يرسل Facebook/Instagram/WhatsApp رسالة حقيقية دون live provider setup
لن يعمل Chatwoot Cloud auto-provisioning بهذا Platform API
لن يمر SocialAPI inbound تلقائيًا إلى Chatwoot mirror بعد PR-026
لن تتحدث delivery statuses إلى OutboundMessage دون status-runtime PR مستقل
لن يكون Dashboard frontend التجاري مكتملًا لمجرد تشغيل backend
لن يكون LLM أو AutoReply مفعّلًا افتراضيًا
```

## الحكم النهائي

الحالة الحالية هي **Backend foundation قوي ومثبت محليًا وعلى PostgreSQL، مع Chatwoot inbound path وSocialAPI inbound Mujeeb materialization منفذين ضمن حدود واضحة**. لكنها ليست بعد منصة live كاملة من زر Dashboard إلى Facebook/Instagram/WhatsApp والرد الحقيقي.

المرحلة المنطقية التالية ليست E2E عشوائيًا، بل **SocialAPI inbound → Chatwoot mirror runtime** ثم **delivery status materialization**، وبعد ذلك فقط يصبح من المنطقي إجراء اختبار live خارجي محدود وآمن.
