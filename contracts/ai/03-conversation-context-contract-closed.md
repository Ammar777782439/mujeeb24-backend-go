

تمام. نغلق ③ Conversation Context Contract بالكامل الآن، ولا نرجع نعيد تصميمه.

اعتمدت على عقد المشروع الموجود، وعلى توثيق Gemini الحالي. Google توصي حاليًا بـ Interactions API للمشاريع الجديدة، ويدعم previous_interaction_id لمتابعة المحادثة دون إعادة إرسال كامل التاريخ في كل مرة. 

③ Conversation Context Contract — CLOSED

القرار الأساسي

لدينا ذاكرتان، لكن مصدر حقيقة واحد فقط:

Mujeeb
└── Canonical State
    ├── messages
    ├── conversation
    ├── conversation_state
    └── business data

Gemini
└── Conversation continuity
    └── previous_interaction_id

وهذا متوافق مع العقد الموجود في المشروع: Mujeeb يحتفظ بـconversation_state والرسائل وبيانات التاجر، بينما Gemini يستخدم سياق المحادثة لمساعدته على الفهم. 


---

1. ConversationState هو الذاكرة التجارية المهمة

نثبت الحقول التي حددناها سابقًا:

{
  "focus": {},
  "previous": {},
  "comparison": {},
  "preferences": {},
  "constraints": {},
  "pending": {},
  "version": 1
}

المعنى:

focus
→ الشيء الذي يدور حوله الحوار الآن

previous
→ المرجع السابق عندما يعتمد الكلام الحالي عليه

comparison
→ ما الذي يقارنه العميل

preferences
→ التفضيلات التي ظهرت في الحوار

constraints
→ القيود التي ذكرها العميل

pending
→ المعلومة أو الخطوة التي ما زالت معلقة

version
→ نسخة حالة الـConversation State

ولا نضيف حقولًا أخرى الآن.


---

2. ما الذي يدخل Gemini في كل Turn؟

الـContext النهائي يكون:

SYSTEM RULES
+
BUSINESS CONTEXT
+
CURRENT CONVERSATION
+
CURRENT STATE
+
CURRENT USER MESSAGE
+
CATALOG EVIDENCE

SYSTEM RULES

قواعد ثابتة مثل:

لا يخترع بيانات
لا ينفذ بنفسه
يستخدم بيانات الكتالوج عند الحاجة
Human handoff له أولوية
النظام يتحقق من المقترح النهائي

هذا موجود بالفعل في عقد الـAI الحالي. 

BUSINESS CONTEXT

من PostgreSQL:

Business
Agent Role
Policies
Knowledge

ولا نضع قاعدة بيانات التاجر كاملة داخل الـprompt. نأخذ فقط الـContext المطلوب. 

CURRENT CONVERSATION

رسائل المحادثة ذات الصلة، بترتيبها.

مثال:

Customer: طيب الأساسية وش فيها؟
AI: ...
Customer: وكم مدتها؟

CURRENT STATE

focus
previous
comparison
preferences
constraints
pending
version

CURRENT USER MESSAGE

الرسالة الجديدة التي يجب على Gemini تفسيرها.

CATALOG EVIDENCE

لا توجد إلا عندما تصبح بيانات الكتالوج متاحة نتيجة مسار الـCatalog الذي أغلقناه سابقًا.

ولا نرسل Catalog كاملًا بلا حاجة.


---

3. لماذا نحتاج ConversationState مع Gemini؟

لأن الرسالة:

> «والثاني؟»



بمفردها لا تحمل معنى كافيًا.

Mujeeb يعرف مثلًا:

focus = المنتج A
comparison = المنتج A مقابل B

ومع الرسائل الأخيرة يصبح لدى Gemini:

Conversation
+
State
+
Current Message

فيستطيع فهم المرجع.

هذا هو نفس المثال الذي وثقه المشروع سابقًا. 


---

4. previous_interaction_id

هذا سيكون موجودًا على مستوى المحادثة الطبيعية مع Gemini:

Customer Turn 1
      ↓
Interaction #1
      ↓
previous_interaction_id
      ↓
Customer Turn 2
      ↓
Interaction #2

Google توضح أن previous_interaction_id يحافظ على تاريخ المدخلات والمخرجات، بينما tools وsystem_instruction وgeneration_config ليست محفوظة تلقائيًا كإعدادات مستمرة ويجب تحديدها للتفاعل الحالي عند الحاجة. 

إذن في Mujeeb:

conversation_id
    ↓
last_gemini_interaction_id

لكن last_gemini_interaction_id ليس مصدر الحقيقة التجاري.


---

5. لا نستخدم Gemini كمخزن للمحادثة

لو حدث:

Gemini unavailable

Mujeeb لا يفقد:

Conversation
Messages
State
Business Context

لأنها محفوظة عندنا.

وهذه نقطة مقفلة.


---

6. لا نرسل كل شيء في كل Turn

الـContext Builder يبني فقط:

Static Rules
+
Business Context
+
Relevant Conversation
+
Validated State
+
Current Message
+
Catalog Evidence

وليس:

SELECT *
FROM entire_database

ولا حتى:

كل بيانات التاجر

هذا متوافق مع القرار الموجود في المشروع: ContextBuilder هو الذي يجمع Business / Policies / Knowledge / Conversation / State / Catalog Evidence قبل Gemini. 


---

7. علاقة Context مع Catalog

التدفق النهائي أصبح:

Customer Message
      ↓
Conversation State + Recent Context
      ↓
Gemini
      ↓
هل يحتاج Catalog؟
      ↓
Catalog Evaluation / Batching
      ↓
Catalog Evidence
      ↓
Gemini
      ↓
Proposal

ولا يوجد:

Mujeeb understands customer intent

ولا:

Mujeeb matches products


---

8. العلاقة مع الـPolicies

Gemini يقترح:

{
  "action": "answer",
  "requires_approval": false
}

لكن هذا ليس القرار النهائي.

Mujeeb يرجع إلى Policy الحقيقية:

AI Proposal
   ↓
PolicyEvaluator
   ↓
Effective Decision

فإذا كانت سياسة التاجر تقول أن الرسائل تحتاج موافقة:

requires_approval = true

بغض النظر عما اقترحه Gemini. 


---

9. التخزين في Gemini

هنا نثبت اختيارنا:

نستخدم Interactions API stateful conversation باستخدام previous_interaction_id للمحادثة الطبيعية.

لكن نعرف بالضبط ماذا يعني ذلك:

Google تقول إن Interactions API يخزن التفاعلات افتراضيًا (store=true) لاستخدام حالة المحادثة، والاحتفاظ الحالي الموثق هو يوم واحد للمستوى المجاني و55 يومًا للمستوى المدفوع؛ ويمكن استخدام store=false لإيقاف التخزين، لكن عندها لا يمكن استخدام previous_interaction_id. 

إذن:

Mujeeb retention
= canonical

Gemini retention
= convenience / reasoning state

وليس العكس.


---

10. الـContract النهائي المغلق

Messenger
   ↓
Inbound Message
   ↓
Mujeeb
   ├── conversation_id
   ├── messages
   ├── ConversationState
   ├── Business Context
   └── Policies
          ↓
    Context Builder
          ↓
       Gemini
          │
          ├── needs catalog
          │       ↓
          │   Catalog Evaluation
          │       ↓
          │   Catalog Evidence
          │       ↓
          └───────────────┐
                          ↓
                       Gemini
                          ↓
                    AI Proposal
                          ↓
                  Mujeeb Validator
                          ↓
              Policy / Authorization
                          ↓
                 Send / Approval / Handoff

مغلق نهائيًا

✅ Mujeeb = canonical conversation state
✅ Gemini = conversational reasoning
✅ previous_interaction_id = continuity
✅ ConversationState = focus / previous / comparison / preferences / constraints / pending / version
✅ Business context ديناميكي
✅ Policies من Mujeeb
✅ Current message منفصل وواضح
✅ Catalog evidence يدخل عند الحاجة
✅ لا نرسل قاعدة البيانات كاملة
✅ Gemini ليس مصدر الحقيقة
✅ AI لا يقرر الـauthorization
✅ لا semantic search داخل Mujeeb

انتهى موضوع Conversation Context  

التالي

الآن ننتقل إلى ④ Gemini System Contract + AI Input/Output Contract: نثبت بالضبط ماذا نقول لـGemini، ماذا نرسل له، وماذا يجب أن يعيد لنا، ثم بعده يصبح الانتقال إلى التنفيذ واضحًا.