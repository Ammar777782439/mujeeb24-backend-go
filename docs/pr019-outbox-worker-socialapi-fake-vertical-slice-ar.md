# PR-019 — Outbox Worker وSocialAPI Fake Outbound Vertical Slice

**الحالة:** مكتمل محليًا حتى fake provider execution، دون SocialAPI live

## النطاق

هذه الدفعة تكمل الجزء التالي بعد PR-018:

```text
Outbox pending
    ↓
Worker RunOnce / polling
    ↓
Claim with lease
    ↓
MujeebOutboundDeliveryResolver
    ↓
SocialAPI ChannelProvider
    ↓
complete أو dead-letter
```

لا تعيد هذه الدفعة فتح Chatwoot inbound أو EventStore أو Catalog أو Sales، ولا تنفذ LLM حقيقيًا.

## Worker

تم تحويل `WorkerRuntime` من lifecycle-only إلى polling loop. `RunOnce` يقرأ claimable entries لجميع التجار عبر `OutboxStore.ListClaimable`، ثم يمرر كل entry إلى `OutboxProcessor`. worker لا ينفذ أي إرسال إذا لم يكن provider مكوّنًا.

تمت إضافة `ListClaimable` إلى application port بدل استخدام `List` مع BusinessID فارغ؛ لأن worker يحتاج قراءة obligations عبر tenants مع بقاء tenant_id محفوظًا داخل كل entry.

## Resolver

`MujeebOutboundDeliveryResolver` يقرأ فقط من Mujeeb:

1. OutboundMessage بالـbusiness scope.
2. ChannelConnection ويتحقق من `active` وprovider/channel/account.
3. ConversationReference ويتحقق من `provider` و`is_current` وconnection match.
4. Inline text content reference.

ثم يعيد `ports.OutboundDelivery` فقط. لا توجد network calls داخل resolver أو database transaction.

## Unknown provider outcome

إذا أعاد provider خطأ نقل، لا نعيد الرسالة تلقائيًا. يحوّل `OutboxProcessor` entry إلى `dead_letter` مع `provider_send_outcome_unknown` حتى يفحص reconciliation حالة المزود قبل تقرير إعادة المحاولة. هذا يمنع duplicate customer messages. تم تصحيح الاختبار ليغطي هذا السلوك.

## SocialAPI fake contract

أضيف اختبار integration محلي يستخدم `httptest.Server` فقط. يثبت أن Worker يستدعي SocialAPI adapter بالمسار الصحيح وبـBearer authorization وbody الصحيح، ثم يكمل Outbox بعد response نجاح. هذا **ليس SocialAPI live**؛ لا DNS خارجي، ولا token حي، ولا Facebook message.

## ما هو مثبت

| المسار | الحالة | الدليل |
|---|---|---|
| Outbox `ListClaimable` contract | PASS — code + unit path | PostgreSQL implementation موجود |
| Worker RunOnce → claim → processor | PASS — unit fake | `worker_test.go` |
| Resolver provider/channel/reference validation | PASS — unit contract | `outbound_delivery_resolver_test.go` |
| Worker → SocialAPI adapter → fake HTTP | PASS — local fake contract | `TestWorkerRunOnceExecutesThroughSocialAPIAdapterContract` |
| success → Outbox complete | PASS — unit fake | worker/processor tests |
| transport error → dead-letter | PASS — unit fake | OutboxProcessor test |
| PostgreSQL worker execution | NOT RUN | `POSTGRES_TEST_DSN` غير موجود في البيئة الحالية |
| SocialAPI live send | NOT RUN عمدًا | لا token حي ولا target حقيقي |
| Chatwoot callback → AutoReply trigger | NOT WIRED | يحتاج provider conversation mapping آمنًا، ولا يُخمن من Chatwoot IDs |
| real Facebook reply | NOT RUN | خارج هذه الدفعة |

## الحدود المعمارية

Chatwoot يظل Workspace داخليًا. Mujeeb يملك OutboundMessage وOutbox وقرار الإرسال. SocialAPI هو transport. worker لا يقرأ Chatwoot DTOs ولا يضع secrets في Domain.

الـAutoReplyService من PR-018 يستطيع إنشاء AIDecision ثم OutboundMessage ثم Outbox عندما يستلم provider-neutral input يحتوي على provider reference صالح. أما ربطه تلقائيًا بــChatwoot callback فيحتاج تحديدًا موثقًا يربط inbox/workspace بالمحادثة الخارجية، ولا يجوز إنشاء هذا الربط اعتمادًا على أرقام Chatwoot وحدها.

## التشغيل اللاحق

يمكن تشغيل Worker الحقيقي لاحقًا فقط في بيئة مقصودة تحتوي PostgreSQL وSocialAPI configuration محليًا. قبل ذلك يجب تشغيل PostgreSQL integration test، ثم fake outbound tests، ثم اختبار حي محدود بحساب وهدف تجريبيين وبموافقة صريحة. لا تُضاف credentials إلى GitHub.
