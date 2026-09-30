# PR-022 — OpenAI-Compatible LLM Runtime

## الحكم التنفيذي

تم تنفيذ **LLM Runtime حقيقي قابل للتبديل** خلف `ports.AIRuntime`، مع إبقاء `SafeAutoReplyRuntime` للاختبارات المحلية فقط. الـruntime الجديد يستخدم OpenAI-compatible Chat Completions عبر HTTP، ويطلب structured JSON schema، ثم يحوّل الاستجابة إلى `ports.AIDecisionProposal`.

لم يُضف أي SDK خاص بـOpenAI أو Anthropic أو Gemini، ولم تُحفظ أي API key في المستودع. يمكن تشغيل adapter مع أي endpoint متوافق مع العقد نفسه عبر إعدادات runtime.

## العقد والمسؤوليات

```text
LLM adapter
  ↓ structured proposal فقط
AutoReplyService
  ↓ validation + policy decision gate
AIDecision
  ↓ transaction
OutboundMessage + Outbox
  ↓ لاحقًا
Worker + Provider adapter
```

الـLLM لا يملك قاعدة البيانات أو Chatwoot أو SocialAPI أو Outbox، ولا ينفذ tools أو side effects. كما لا يتم تخزين chain-of-thought أو prompt secrets أو raw provider response.

`AutoReplyService` بقي مالكًا للبوابة التطبيقية. فهو يرفض action غير المسموح، وpolicy decision غير المعتمد، وschema version غير الصالح، ولا ينشئ OutboundMessage إلا عندما تكون النتيجة `answer` و`allowed` ولا تتطلب human review.

## Structured proposal

الـadapter يطلب الحقول التالية:

| الحقل | الغرض |
|---|---|
| `intent_base` | نية العميل كتوصيف، وليست أمر تنفيذ |
| `domain_context` | سياق القرار |
| `entities` | مفاتيح entities معروفة من عقد Mujeeb، بقيم نصية فارغة عند الغياب |
| `evidence_references` | مراجع الأدلة التي قدمها السياق |
| `requested_action` | `answer` أو `ask_clarification` أو `no_action` |
| `response_text` | النص المقترح فقط |
| `confidence_value` و`confidence_band` | الثقة، وليست authorization |
| `requires_human` | طلب مراجعة بشرية |
| `missing_information` و`reason_codes` | أسباب القرار والنواقص |
| `policy_decision` | `allowed` أو `requires_approval` أو `denied` |
| `policy_version` و`knowledge_version` | مراجع النسخ |
| `schema_version` | إصدار عقد proposal، حاليًا `1` |

المزود يفرض object مغلقًا عبر `additionalProperties=false`. وبسبب اختلاف دعم بعض مزودي OpenAI-compatible للـenum داخل strict schema، بقيت قيم action وpolicy تحت validation التطبيقية الصارمة بدل الاعتماد على المزود وحده.

## Configuration

الإعدادات الجديدة هي:

| المتغير | الافتراضي | الحكم |
|---|---:|---|
| `LLM_ENABLED` | `false` | لا يعمل LLM دون تفعيل صريح |
| `LLM_BASE_URL` | فارغ | endpoint متوافق مع `/chat/completions` |
| `LLM_API_KEY` | فارغ | runtime secret فقط، لا يذهب إلى Domain أو logs |
| `LLM_MODEL` | فارغ | يجب تحديد model عند التفعيل |
| `LLM_HTTP_TIMEOUT` | `30s` | timeout للطلب |
| `LLM_MAX_OUTPUT_TOKENS` | `700` | حد output |
| `LLM_MAX_INPUT_CHARACTERS` | `12000` | حد input |
| `LLM_OUTPUT_TOKENS_FIELD` | `max_completion_tokens` | يمكن تغييره إلى `max_tokens` للمزود المناسب |

إذا كان `LLM_ENABLED=true` دون `LLM_BASE_URL` أو `LLM_API_KEY` أو `LLM_MODEL`، يفشل config/bootstrap صراحةً. وإذا كان `CHATWOOT_AUTOREPLY_ENABLED=true` دون LLM runtime صالح، يرفض Bootstrap التشغيل بدل استخدام fallback صامت.

## الملفات التي تغيرت

```text
internal/adapters/secondary/ai/openaicompatible/client.go
internal/adapters/secondary/ai/openaicompatible/client_test.go
internal/application/services/auto_reply_llm_runtime_test.go
internal/bootstrap/api.go
internal/bootstrap/integrations.go
internal/bootstrap/integrations_test.go
internal/platform/config/config.go
internal/platform/config/config_test.go
.env.example
docs/pr022-llm-runtime-decision-ar.md
```

## الاختبارات الفعلية

نجحت اختبارات adapter المحلية باستخدام `httptest`، وشملت التحقق من Authorization، strict JSON schema، `additionalProperties=false`، اختيار حقل token الصحيح، mapping إلى proposal، ورفض JSON/schema غير الصالح.

ونجح اختبار الوصلة:

```text
OpenAI-compatible response
  → AIRuntime
  → AutoReplyService
  → AIDecision
  → OutboundMessage
  → Outbox
```

كما تم تنفيذ **live smoke test حقيقي** على proxy LLM المدمج باستخدام رسالة اصطناعية فقط، وبعد قراءة الكتالوج الحي اختير `gpt-5-mini` لأنه كان موجودًا فعليًا وقت الاختبار. عاد النموذج structured proposal صالحًا، وكانت النتيجة:

```text
action=ask_clarification
confidence_band=high
policy_decision=allowed
model_reference=openai-compatible/gpt-5-mini
```

ظهرت أثناء smoke محاولة أولى مشكلة توافق في strict schema، ثم عُزل السبب باختبارات probes قصيرة وأُصلح دون إضعاف validation التطبيقية. اختبارات Go الكاملة و`go vet` نجحت بعد الإصلاح.

## حدود الإغلاق

هذا PR يغلق **LLM adapter وحقن runtime وعقد structured proposal** محليًا. لا يدعي إغلاق Context Builder أو Catalog grounding أو production policy engine المتقدم؛ فالـadapter الحالي يستقبل `AIDecisionInput` الموجود، ولا يقرأ قاعدة البيانات بنفسه.

كما لم يتم في هذا PR تشغيل Chatwoot live أو SocialAPI live أو Facebook أو worker خارجي. يبقى الـoutbound متوقفًا عند نفس reliability boundary المثبتة سابقًا، ولا يوجد provider send في transaction أو auto-resend لنتيجة network مجهولة.

## قرار التشغيل

للتطوير المحلي فقط، يضبط المطور `LLM_ENABLED=true` ويحقن القيم من خارج GitHub. لا تُستخدم مفاتيح ظهرت في المحادثات أو في ملفات تاريخية. الـ`.env.example` يحتوي placeholders فارغة، وdefault التشغيل يظل `LLM_ENABLED=false`.
