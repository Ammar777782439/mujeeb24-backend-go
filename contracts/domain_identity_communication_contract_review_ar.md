# مراجعة المدير — Domain Contract 04: Identity وCommunication

## الحكم التنفيذي

التحليل المرفق **قوي وصحيح في اتجاهه**، وهو أهم جزء أمني وتجاري بعد Channel Contract؛ لأن الخطأ هنا قد يكشف محادثة عميل لشخص آخر أو يدمج مبيعات حسابين مختلفين.

أعتمد المبدأ التالي:

```text
Customer داخل Mujeeb
    ↓ 1:N
ExternalIdentity لكل قناة/حساب خارجي
    ↓
Conversation داخل Mujeeb
    ↓ 1:N
ConversationReference لكل Workspace/Provider resource
```

ولا نعتمد أبدًا على الاسم وحده لدمج الهويات.

## ما نعتمده كما هو

| القرار | الحكم |
|---|---|
| Customer لا يساوي Chatwoot Contact | معتمد |
| Customer لا ينتمي إلى قناة واحدة | معتمد |
| ExternalIdentity مرتبطة بـBusiness | معتمد وضروري للعزل |
| External user ID ليس global | معتمد |
| عدم الدمج بالاسم فقط | معتمد كقاعدة أمنية |
| IdentityMatch قرار موثق وليس Boolean | معتمد |
| إنشاء Customer جديد عند الغموض | معتمد |
| Customer Merge عملية مستقلة مع Audit | معتمد |
| Conversation Mujeeb ليست Chatwoot Conversation | معتمد |
| ConversationReference جسر خارجي | معتمد |
| عدم بناء Team/Assignment داخل Domain | معتمد |
| الفصل بين state وownership وai_mode | معتمد |
| إعادة بناء Reference عند crash | معتمد |
| عدم إعادة إرسال SocialAPI عند فشل Chatwoot mirror | معتمد |

## التصحيحات الإلزامية

### 1. Customer لا يستخدم phone/email كهوية مباشرة

الحقول `phone` و`email` يمكن أن تكون Contact Points، لكنها ليست هوية مضمونة دائمًا:

```text
Customer
├── id
├── business_id
├── profile
├── contact_points
├── locale_preference
├── status
└── timestamps
```

كل Contact Point يحمل:

```text
kind: phone | email | other
value_normalized
verification_status: unknown | verified | rejected
source
consent_reference?
```

لا نضع unique عالميًا على phone/email؛ قد تتشارك عائلة رقمًا، وقد يستخدم العميل أكثر من رقم، وقد تكون البيانات قديمة. تُستخدم Verified Phone/Email كدليل مطابقة قوي ضمن Business، لا كقاعدة سحرية وحيدة.

### 2. ExternalIdentity يجب أن تشير إلى ChannelConnection

التحليل يستخدم `provider + channel + external_account_id + external_user_id`، وهذا جيد، لكن المفتاح التشغيلي الأدق هو `channel_connection_id`؛ لأن الاتصال هو الذي يحدد Business والحساب الخارجي فعليًا.

```text
ExternalIdentity
├── id
├── business_id
├── customer_id?
├── channel_connection_id
├── provider_ref
├── channel
├── external_account_ref
├── external_user_id
├── profile_snapshot_reference
├── status
├── first_seen_at
└── last_seen_at
```

المفتاح الفريد:

```text
business_id + channel_connection_id + external_user_id
```

ويمكن الاحتفاظ بـ`external_account_ref` للعرض والتدقيق، لكن لا نعتمد عليه وحده بدل Connection.

### 3. ExternalIdentity يمكن أن تكون غير مربوطة مؤقتًا

عند أول رسالة قد ننشئ ExternalIdentity قبل Customer النهائي:

```text
ExternalIdentity.customer_id = null
link_status = unresolved
```

ثم يقرر Identity Resolution إنشاء Customer أو ربطها بعميل موجود. هذا أفضل من إنشاء Customer ثم التراجع عند كل حالة غامضة.

### 4. IdentityMatch هو قرار مع Evidence

لا يكفي:

```text
left_identity
right_identity
method
confidence
decision
```

العقد الأدق:

```text
IdentityMatch
├── id
├── business_id
├── left_identity_id
├── right_identity_id
├── method
├── evidence_references
├── confidence_band
├── decision
├── decided_by
├── decided_at
└── status
```

`method`:

```text
explicit_link
verified_phone
verified_email
provider_link
manual_confirmation
candidate_similarity
```

`candidate_similarity` لا يسمح بالدمج تلقائيًا. والقرار:

```text
match | no_match | needs_review | revoked
```

يجب أن نحتفظ بسبب القرار ومصدر الدليل حتى يستطيع الموظف التراجع عن Match خاطئ.

### 5. Customer Merge لا يحذف العميل فعليًا

عند الدمج:

```text
Customer B → merged_into Customer A
```

نحتفظ بـalias/redirect وAudit وننقل الهويات والمراجع وفق transaction آمنة. لا نحذف B من التاريخ، ولا نسمح بأن تؤدي إعادة معالجة حدث قديم إلى إنشاء B من جديد.

الدمج محصور داخل Business واحد في V1. لا ندمج Customer من Business A مع Business B حتى لو كان الهاتف متطابقًا.

## Communication Contract

### 1. Conversation داخل Mujeeb

Conversation هو السياق التشغيلي الذي يجمع الرسائل والـAI والـSales Context:

```text
Conversation
├── id
├── business_id
├── customer_id
├── state
├── ownership
├── ai_mode_override?
├── priority
├── last_activity_at
├── created_at
└── updated_at
```

حالات V1:

```text
open
ai_handling
waiting_customer
waiting_human
human_handling
closed
```

`ai_mode_override` اختياري؛ القيمة الافتراضية تأتي من BusinessPolicy. ولا نخلط الحالة مع المالك أو Chatwoot status.

### 2. ConversationReference كـBinding مستقل

لا نضع Chatwoot وProvider IDs كلها كحقول إجبارية داخل Conversation؛ فكل نظام قد يملك resource مختلفًا، وقد تتغير المراجع بمرور الوقت.

نستخدم مفهوم Binding/Reference:

```text
ConversationReference
├── id
├── business_id
├── conversation_id
├── system: provider | chatwoot
├── provider_ref
├── resource_type
├── resource_id
├── channel_connection_id?
├── conversation_kind?
├── is_current
├── mapping_status
└── timestamps
```

إذا احتاج التنفيذ أداءً أعلى، يمكن إضافة أعمدة typed مثل Chatwoot Account/Inbox/Conversation، لكن Contract لا يجعل Chatwoot هو الهوية الأساسية.

المفتاح الفريد يكون حسب النظام والمورد:

```text
business_id + system + provider + resource_type + resource_id
```

والـApplication يضمن وجود Binding حالي مناسب للمحادثة بدل إنشاء نسخة ثانية.

### 3. Conversation Kind ليس Conversation Creation Rule

```text
conversation_kind:
  dm | comment | story_reply | mention | review | other
```

هذا يصف المصدر، ولا يعني تلقائيًا أن Comment أصبح Conversation. Application ConversationMappingService يقرر existing/create/interaction-only.

### 4. Ownership وAI Mode منفصلان

```text
ownership: none | ai | human
assignment_reference: optional
ai_mode: allowed | draft_only | disabled
```

قد تكون الحالة:

```text
state = human_handling
ownership = human
ai_mode = draft_only
```

وهذا يعني أن AI يستطيع اقتراح مسودة فقط، ولا يرسل.

### 5. CommunicationMessage مرجع خفيف

Chatwoot هو Workspace Truth للرسائل التشغيلية، لكن Mujeeb يحتاج سجل ربط وموثوقية وContext مختصر:

```text
CommunicationMessage
├── id
├── business_id
├── conversation_id
├── inbound_event_id?
├── direction
├── origin
├── transport
├── external_message_reference?
├── chatwoot_message_reference?
├── content_reference
├── occurred_at
└── created_at
```

لا نحفظ Chatwoot DTO كاملًا في Domain، ولا نجعل CommunicationMessage نسخة ثانية من Message Workspace. أما محتوى الرسالة اللازم لـAI فيحفظ عبر Message/Content Store بسياسة retention واضحة، مع بقاء Domain reference-neutral.

## سيناريوهات الفشل التي يغلقها العقد

إذا وصل الحدث مرتين، يمنع Inbound Idempotency إنشاء Customer أو Conversation ثانية.

إذا أنشأ Chatwoot Conversation ثم انهار Go قبل حفظ Reference، تعيد Application البحث بالمفاتيح الخارجية وChatwoot scope وتعيد بناء Binding.

إذا وصلت رسالة SocialAPI وحُفظ الحدث لكن فشل Chatwoot Mirror، نعيد Mirror فقط. لا نعيد إرسال inbound إلى SocialAPI.

إذا كان Cross-channel match غامضًا، نبقي هويتين أو Customerين منفصلين وننشئ `needs_review`. لا نكشف بيانات أحدهما للآخر.

## معايير إغلاق Identity وCommunication

1. Customer مستقل عن القناة وChatwoot.
2. ExternalIdentity scoped بـBusiness وConnection.
3. phone/email دليل مطابقة وليس هوية عالمية تلقائية.
4. IdentityMatch يحفظ Evidence وDecision وActor.
5. Merge لا يحذف التاريخ ولا يتجاوز Business boundary.
6. Conversation Mujeeb مستقلة عن Chatwoot Conversation.
7. ConversationReference يدعم أكثر من نظام خارجي دون Chatwoot lock-in.
8. Comment لا يتحول تلقائيًا إلى Conversation.
9. state وownership وassignment وai_mode حقول منفصلة.
10. CommunicationMessage مرجع خفيف لا نسخة Chatwoot.

## القرار

**أعتمد التحليل المرفق بعد هذه التصحيحات.** أصبح `identity` و`communication` جاهزين كعقد تصميم، ولم نكتب implementation بعد.

الخطوة التالية المنطقية هي `domain/catalog`، مع ربطه بـBusiness وCustomer وConversation/Lead لاحقًا، دون تغيير قواعد الهوية التي أُغلقت هنا.
