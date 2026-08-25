# PR-023 — Context Builder وCatalog/Knowledge Grounding

## الحكم التنفيذي

تم تنفيذ **Context Builder محدود ومملوك لـMujeeb** قبل استدعاء `AIRuntime`. لا يقرأ الـLLM قاعدة البيانات، ولا يتصل بـChatwoot أو SocialAPI، ولا ينفذ أي side effect. الـBuilder يستخدم repository surfaces الموجودة فقط لبناء `ports.AIContext` typed ومحدود.

```text
Business + Conversation + Customer
                ↓
Recent Communication Messages
                ↓
Active Catalog → matched Items → Offers + Variants
                ↓
Evidence/Freshness/Policy state
                ↓
AIRuntime
```

## العقد الذي تم تثبيته

تمت إضافة `AIContextBuilder` إلى Application ports، وتوسيع `AIDecisionInput` ليحمل `*AIContext` اختياريًا. الحقول ليست `map[string]any`؛ كل جزء له record contract typed، بينما تبقى documents المرنة كـraw JSON bytes بعد التحقق من JSON.

| الجزء | الملكية والمحتوى |
|---|---|
| Business | reference، name، vertical، locale، default currency |
| Conversation | reference، customer reference، state، ownership، priority، AI mode override، assignment |
| Customer | reference، locale، status، profile/contact documents داخل Mujeeb context فقط |
| Catalog evidence | item reference، catalog reference، item type، name، status، attributes، retrieved_at، schema version |
| Offer evidence | offer/item/variant references، pricing mode، amount، currency، availability، status، evidence state |
| Variant evidence | variant/item references، name، status، attributes، evidence state |
| Recent messages | internal message reference، direction، origin، text، occurred_at، ordered chronologically |
| Policy evidence | application policy reference/version، state، missing reason |
| Lifecycle | `generated_at` و`expires_at`، و`schema_version=1` |

## Catalog grounding

يطلب الـBuilder الكتالوجات النشطة ضمن business scope، ثم يقرأ العناصر النشطة ويصنفها deterministic على أساس tokens من رسالة العميل. لا يتم تمرير كل Catalog Database إلى النموذج؛ الحد الافتراضي هو 10 catalog responses، و5 matched items، و5 offers، و5 variants، و8 recent messages.

بعد اختيار item، تُقرأ عروضه وvariants من خلال parent-scoped repository methods الموجودة. كل record يُراجع business ID وparent ID مرة أخرى داخل Application قبل إدخاله في context. لذلك لا يمكن أن ينتقل offer أو variant من business أو item آخر إلى prompt بسبب خطأ adapter.

إذا لم يطابق نص العميل أي item، يكون `KnowledgeState=missing` و`Freshness=partial`. وإذا وُجدت catalog evidence لكن policy repository المتقدم غير موصول بعد، يكون `KnowledgeState=partial` مع `PolicyEvidence.State=application_policy_only` وسبب النقص واضحًا.

إذا كانت `availability_status` للعرض `unknown` أو `stale`، يبقى النص كما هو ولا يتحول إلى `available`. يتم تسجيل `EvidenceState=stale` وتصبح freshness العامة `stale`. هذا يمنع النموذج من اعتبار availability غير الموثقة حقيقة تجارية.

## حدود البيانات المرسلة إلى LLM

يحوّل OpenAI-compatible adapter الـAIContext إلى prompt محدد. يمرر business/conversation references وcatalog/offer/variant evidence وhistory اللازمة، لكنه يستبعد Customer profile/contact raw من prompt. هذه الوثائق تبقى داخل Mujeeb context ولا تُرسل إلى النموذج في هذه الدفعة.

تعليمات النظام تجعل evidence مصدر الحقائق الوحيد. أي إجابة factual عن catalog أو offer أو availability يجب أن تستخدم `evidence_references`، وأي معلومة ناقصة أو stale يجب أن تؤدي إلى clarification أو no_action، لا إلى اختراع سعر أو توفر أو موعد.

## الربط مع AutoReply

قبل PR-023 كان `AutoReplyService` يمرر النص الخام إلى AIRuntime. الآن، إذا تم حقن `ContextBuilder`، يبني السياق أولًا ثم يمرر:

```text
AutoReplyCommand
      ↓
ContextBuilder.Build
      ↓
AIDecisionInput{Text + Context}
      ↓
AIRuntime
      ↓
existing validation/policy gate
      ↓
AIDecision → OutboundMessage → Outbox
```

الـContext Builder لا يعمل داخل transaction الخاصة بإنشاء AIDecision وOutboundMessage وOutbox، ولا يرسل provider calls. Bootstrap يحقنه في Chatwoot AutoReply runtime مع Business/Conversation/Customer/Catalog/Message repositories الموجودة.

## الاختبارات

الاختبارات المحلية تغطي بناء context bounded، catalog ranking، offer/variant mapping، chronology للرسائل، deduplication للرسالة الحالية، expiration، application-policy-only state، tenant/business/parent mismatch، وstale availability.

تم توسيع PostgreSQL integration test المركب السابق ليستخدم ContextBuilder الحقيقي فوق PostgreSQL 16 مؤقتة. الاختبار يثبت أن Chatwoot inbound المادي أولًا ثم يبني context من Business/Conversation/Customer/Message repositories قبل Safe runtime capture، مع بقاء AIDecision وOutboundMessage وOutbox وduplicate/echo assertions السابقة ناجحة.

كما يغطي adapter test ظهور evidence التجاري في prompt وعدم ظهور profile/contact secrets فيه.

## ما لم يُدّعَ إغلاقه

هذه الدفعة لا تدعي وجود Knowledge repository عام أو Policy Engine متقدم أو semantic/vector search. لا يوجد بعد مصدر مستقل لـbusiness policies أو availability supplier checks؛ لذلك يظل policy evidence معلنًا `application_policy_only`، وتظل availability غير المعروفة stale.

كما لم يتم تشغيل Chatwoot live أو SocialAPI live أو Facebook أو worker خارجي. هذه الدفعة تغلق **Context Builder وCatalog grounding المحليين** فوق المسار الموجود، ولا تعني أن الرد الخارجي الحقيقي صار مفعلًا.
