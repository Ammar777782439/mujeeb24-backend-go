# PR-018 — أول Auto Reply Vertical Slice

**الحالة:** تنفيذ محلي آمن حتى Outbox، دون Worker أو Provider network

**النطاق:** `AI Runtime → Structured AIDecision → ANSWER Action → OutboundMessage → Outbox`

## القرار

هذا المسار لا يعيد فتح Chatwoot inbound. يبدأ بعد أن تكون الرسالة الواردة قد أصبحت رسالة داخل Mujeeb، ويحتاج إلى `business_id` و`conversation_id` ومرجع الرسالة الواردة و`channel` و`provider_ref`. لا يقرأ Dashboard من Chatwoot، ولا ينفذ AI أو Provider داخل HTTP handler أو داخل عملية الشبكة.

```text
Inbound message already materialized in Mujeeb
        ↓
AutoReplyService
        ↓
AIRuntime.Decide
        ↓
Structured AIDecisionProposal
        ↓
AIDecisionRepository.CreateProposed
        ↓ restricted_auto + answer + allowed
OutboundMessageRepository.CreatePending
        ↓
OutboxStore.Enqueue(command_type=channel.send_message)
```

## AI Runtime الحالي

التنفيذ الأول هو `SafeAutoReplyRuntime`. هو Runtime حتمي محلي، وليس LLM حيًا. ينتج رد acknowledgment غير factual حتى نثبت الحلقة دون إرسال بيانات أو استدعاء مزود خارجي. يعيد Runtime عقدًا منظمًا يحتوي على intent وentities وevidence وrequested action وconfidence band وpolicy decision وschema/model references.

هذا التنفيذ لا يدعي أنه AI production أو بديل نهائي لـLLM. عند إضافة LLM لاحقًا، يجب أن يعيد نفس `AIDecisionProposal` ويمر عبر نفس validation وpolicy gates، ولا يملك SQL أو HTTP أو Provider side effects.

## نطاق Action

الـslice الحالي يسمح فقط بـ`answer` و`ask_clarification` و`no_action` كاقتراحات. لا يتم إنشاء OutboundMessage إلا إذا تحققت جميع الشروط التالية:

1. الوضع هو `restricted_auto`.
2. الإجراء هو `answer`.
3. `policy_decision=allowed`.
4. `requires_human=false`.
5. يوجد نص رد غير فارغ.
6. يوجد ConversationReference حالي للـprovider مع `connection_id` و`resource_id`.

أي قرار يحتاج موافقة أو handoff يُحفظ كـAIDecision مقترح فقط ولا يتحول إلى Outbox.

## الملكية والتخزين

Mujeeb يملك AIDecision وOutboundMessage وOutboxEntry. Chatwoot يبقى Workspace داخليًا، وSocialAPI يبقى transport provider. النص الناتج لا يُخزن في جدول `ai_decisions`؛ بل يُحفظ في `OutboundMessage.content_reference` كمرجع V1 من الشكل:

```text
content://inline-text/v1/<base64url>
```

هذا حل text-only محدود للـslice الحالي. يجب استبداله أو توسيعه بـContent Store مخصص قبل الوسائط أو النصوص الكبيرة، ولا يجوز اعتباره provider idempotency.

## الذرية

ينفذ `AutoReplyService` إنشاء AIDecision ثم OutboundMessage ثم Outbox داخل `TransactionManager.Within`. إذا فشل إنشاء Outbox، يجب أن تتراجع المعاملة عن القرار والرسالة outbound معًا. لا توجد network calls داخل هذه المعاملة.

يستخدم Outbox command type الموجود في `OutboxProcessor`:

```text
channel.send_message
```

ويستخدم dedupe key:

```text
auto-reply:<source_message_reference>
```

لا يعني هذا أن provider يدعم idempotency؛ المفتاح داخلي في Mujeeb ويُمرر لاحقًا إلى delivery resolver وفق السياسة المعتمدة.

## الاختبارات

الاختبارات المحلية تثبت:

| الاختبار | الحالة |
|---|---|
| Safe runtime ينتج structured answer | PASS — unit |
| answer + allowed ينشئ AIDecision ثم OutboundMessage ثم Outbox | PASS — unit fakes |
| non-answer أو requires approval لا ينشئ Outbox | PASS — unit fakes |
| PostgreSQL transaction وschema وrollback عند فشل enqueue | مُضاف — يحتاج `POSTGRES_TEST_DSN` لتشغيله |

في البيئة الحالية لم يكن `POSTGRES_TEST_DSN` موجودًا، لذلك لم أدّعِ نجاح اختبار PostgreSQL الجديد في هذه الجولة. سيتم تشغيله عند توفر قاعدة الاختبار المعزولة، دون أي token أو network provider.

## ما لم يُنفذ في PR-018

لم يُنفذ LLM provider حي، ولم يُوصل Chatwoot webhook مباشرة بالـAutoReply لأن callback Chatwoot وحده لا يكفي دائمًا لاختيار provider conversation reference الآمن. لم يُنفذ Worker polling أو claim أو resolver أو Provider.SendMessage، ولم تُجرَ SocialAPI live calls. لذلك تنتهي هذه الدفعة عند Outbox فقط.

## الملفات

- `internal/application/commands/auto_reply.go`
- `internal/application/ports/ai_runtime.go`
- `internal/application/ports/ai_audit_repositories.go`
- `internal/application/services/auto_reply.go`
- `internal/application/services/safe_auto_reply_runtime.go`
- `internal/application/services/auto_reply_test.go`
- `internal/adapters/secondary/persistence/postgres/ai_audit_repositories.go`
- `internal/adapters/secondary/persistence/postgres/ai_audit_repository_integration_test.go`
- `docs/pr018-auto-reply-vertical-slice-ar.md`
