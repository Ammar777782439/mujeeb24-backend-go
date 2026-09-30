Mujeeb 24 — ⑩ Final Architecture Map

الحالة

CLOSED

هذه الوثيقة هي الخريطة المعمارية الجامعة للقرارات المغلقة من:

① Catalog AI Projection
② Catalog Evaluation + Batching
③ Conversation Context
④ Gemini System / Input / Output Contract
⑤ Catalog Entity Contract + Actual Catalog Data Boundary
⑥ AI Validation + Authorization
⑦ Execution Boundary + Action Contracts
⑧ Observability + Audit + AI Trace
⑨ AI Runtime Lifecycle + Failure / Retry / Timeout

ولا تضيف قرارًا معماريًا جديدًا.

---

1. Architectural Principles

Mujeeb 24 يعتمد على المبادئ التالية:

PostgreSQL = Business Truth
Mujeeb     = Application / Policy / Authorization / Execution
Gemini     = AI Reasoning
SocialAPI  = Channel Transport

والقاعدة الأساسية:

LLM ≠ Executor

Gemini لا يملك صلاحية مباشرة على:

Database
Provider
Channel
Business Authorization
Execution

---

2. System Shape

المنصة هي:

Mujeeb 24
│
├── Presentation
├── Application
├── Domain
├── Infrastructure / Adapters
└── External Providers

والـBackend يبقى Modular Monolith.

الخدمات المنطقية تكون داخل نفس النظام وليست Microservices منفصلة لمجرد الفصل الشكلي.

---

3. High-Level Architecture

                         MUJEEB 24
                            │
          ┌─────────────────┼─────────────────┐
          │                 │                 │
          ▼                 ▼                 ▼
     Presentation      Application         Domain
          │                 │                 │
          │                 └──────┬──────────┘
          │                        │
          │                        ▼
          │                 Integration Ports
          │                        │
          │          ┌─────────────┼─────────────┐
          │          │             │             │
          ▼          ▼             ▼             ▼
      Channels     Gemini       PostgreSQL     Redis/Workers
       Adapter      Adapter
          │
          ▼
      SocialAPI
          │
          ▼
 Facebook / Instagram / WhatsApp

ولا يوجد Chatwoot في هذه الخريطة.

---

4. Source of Truth

Business Data

PostgreSQL

هو المصدر الأساسي لـ:

Business
Customer
Conversation
Messages
Catalog
Policies
Leads
Orders
Audit

Gemini لا يصبح Database.

---

5. Tenant Boundary

كل AI processing يبدأ من:

Authenticated Business
        ↓
Tenant Scope
        ↓
Context / Catalog / Policy / Data

Gemini لا يحدد:

business_id
tenant_id

Mujeeb هو الذي يحدد نطاق التاجر.

---

6. Incoming Message

التدفق الأساسي:

Facebook / Instagram / WhatsApp
            ↓
        SocialAPI
            ↓
         Webhook
            ↓
         Mujeeb
            ↓
 Verify / Normalize / Idempotency
            ↓
   Resolve Identity / Conversation
            ↓
       Persist Message

من هنا يبدأ AI Runtime عندما تكون الرسالة مؤهلة للمعالجة.

---

7. Conversation Context

Mujeeb يبني السياق:

System Rules
+
Business Context
+
Conversation Context
+
Conversation State
+
Catalog Entity Contract
+
Current Message

و"ConversationState" يحتوي:

{
  "focus": {},
  "previous": {},
  "comparison": {},
  "preferences": {},
  "constraints": {},
  "pending": {},
  "version": 1
}

Mujeeb يبقى المصدر canonical للمحادثة.

Gemini يستخدم conversation continuity للمساعدة في الاستدلال فقط.

---

8. Catalog Entity Contract

قبل التعامل مع بيانات الكتالوج، Gemini يعرف تعريف Catalog نفسه:

Catalog
CatalogItem
AttributeSchema
AttributeDefinition
Variant
Offer

ويعرف:

Fields
Types
Relationships
Enums
Constraints
Meaning

هذا ليس Merchant Data.

إنه:

Catalog Entity Contract
=
لغة الكتالوج التي يفهمها Gemini

---

9. Actual Catalog Data

بيانات التاجر الفعلية تأتي من:

PostgreSQL
   ↓
Catalog Access Boundary
   ↓
Catalog AI Projection
   ↓
Gemini

ولا يتم إرسال Raw PostgreSQL rows.

---

10. Catalog AI Projection

الـProjection تحتوي على البيانات التجارية التي يحتاجها Gemini، وتشمل عند الحاجة:

AttributeSchema
AttributeDefinition
CatalogItem
Variant
Offer

مع الحقول التي حسمناها في Catalog Contract.

والـProjection ليست:

Search Index
Vector DB
Semantic Engine

---

11. Catalog Evaluation

عندما يكون نطاق الكتالوج أكبر من Context مناسب:

Actual Catalog Data
        ↓
Token Counting
        ↓
Token-aware Batching
        ↓
Batch 1
Batch 2
Batch 3
...
Batch N

لا نحدد:

100 Items per batch

ولا:

6 rounds

حجم الـBatch تحدده الـTokens والحدود التشغيلية.

---

12. Coverage Guarantee

Mujeeb Runtime/Controller يضمن:

All entitled catalog items
        ↓
Evaluated

ولا تعتبر عملية Catalog Evaluation مكتملة قبل اكتمال الـCoverage المطلوب.

Gemini مسؤول عن:

Reasoning
Comparison
Interpretation
Candidate identification

وليس عن تتبع Coverage.

---

13. Catalog Read Boundary

عند الحاجة إلى بيانات إضافية:

Gemini
   ↓
Catalog Read Boundary
   ↓
Mujeeb
   ↓
Tenant-scoped PostgreSQL read
   ↓
Catalog AI Projection
   ↓
Gemini

الـBoundary:

Read Only
Tenant Scoped
Structured
No SQL

ولا يتحول إلى Semantic Search.

---

14. AI Reasoning

بعد أن يحصل Gemini على:

Conversation Context
+
Catalog Entity Contract
+
Actual Catalog Evidence
+
Business Context

يقوم بـ:

Understand
Reason
Compare
Determine
Propose

---

15. AI Output

الناتج:

{
  "status": "resolved",
  "action": "answer",
  "response_text": "...",
  "selected": []
}

والحالات:

resolved
ambiguous
not_found
needs_more_data

والـActions:

answer
clarification
human_request
lead_draft
order_draft

هذا Proposal فقط.

---

16. AI Validation

بعد Gemini:

AI Proposal
   ↓
Structural Validation
   ↓
Reference Validation
   ↓
Tenant / Ownership Validation
   ↓
Policy Evaluation
   ↓
Authorization

Mujeeb لا يعيد تفسير نية العميل.

هو يتحقق من صحة ما اقترحه Gemini.

---

17. Policy Authority

Gemini يستطيع رؤية Policy لفهم السياق.

لكن:

Policy DB
   ↓
PolicyEvaluator

هي السلطة النهائية.

خصوصًا:

requires_approval
allow_handoff
execution permissions

---

18. Effective Decision

بعد نجاح:

Validation
+
Policy
+
Authorization

يصبح لدينا:

Effective Decision

وهذه هي النتيجة الوحيدة التي يمكن أن تدخل التنفيذ.

---

19. Execution Boundary

Effective Decision
        ↓
Application Service
        ↓
Executor
        ↓
Provider Port / Domain Service

Gemini لا يدخل هذه الطبقة.

---

20. Answer Flow

Customer Message
      ↓
Gemini
      ↓
action = answer
      ↓
Validation
      ↓
Policy
      ↓
Authorization
      ↓
Application Service
      ↓
Outbound Message
      ↓
Channel Adapter
      ↓
SocialAPI
      ↓
Customer

---

21. Clarification Flow

Customer
   ↓
Gemini
   ↓
clarification
   ↓
Validation
   ↓
Policy
   ↓
Authorization
   ↓
Send Clarification

---

22. Human Handoff Flow

Customer
   ↓
Gemini
   ↓
human_request
   ↓
Validation
   ↓
Policy
   ↓
Human Handoff

ولا يتجاوز AI حالة التعامل البشري.

---

23. Lead Flow

Customer
   ↓
Gemini
   ↓
lead_draft
   ↓
Validation
   ↓
Authorization
   ↓
Lead Application Flow

ولا يعني ذلك أن Gemini أنشأ Lead مباشرة.

---

24. Order Flow

Customer
   ↓
Gemini
   ↓
order_draft
   ↓
Validation
   ↓
Missing Information?
   ├── YES → Clarification
   └── NO
          ↓
      Confirmation
          ↓
      Order Flow

ولا يوجد Order من التخمين.

---

25. AI Runtime

كل معالجة AI لها:

AI Run

Lifecycle:

RECEIVED
CONTEXT_BUILT
RUNNING
WAITING_TOOL
VALIDATING
AUTHORIZED
EXECUTING
COMPLETED
FAILED
CANCELLED

---

26. Retry

Runtime يفرق بين:

Retryable Failure
Non-Retryable Failure

Retryable أمثلة فئاتها:

Temporary Provider Failure
Temporary Network Failure
Temporary Infrastructure Failure
Rate-limit Condition
Temporary Tool Dependency Failure

Non-Retryable:

Invalid AI Output
Invalid Tool Arguments
Tenant Violation
Reference Violation
Policy Denial
Authorization Denial
Unsupported Action

ولا يوجد Retry إلى ما لا نهاية.

---

27. Timeout

Timeout يطبق على العمليات الخارجية مثل:

Gemini Request
Tool Execution
Provider Execution

لكن قيم الـTimeout تبقى Runtime Configuration، وليست Business Rules.

---

28. Idempotency

إعادة تشغيل العملية لا يجب أن تؤدي إلى:

Duplicate Message
Duplicate Lead
Duplicate Order
Duplicate External Action

ويتم تتبع:

ai_run_id

مع Idempotency المناسب للعملية التنفيذية.

---

29. Observability

على مستوى النظام:

Metrics
Logs
Latency
Errors
Health
Alerts

ونقيس AI:

Input Tokens
Cached Tokens
Output Tokens
Model Requests
Tool Calls
Catalog Projection Tokens
Cost per AI Reply

وهذا مطلوب أصلًا ضمن الـeconomic baseline قبل الإنتاج.

---

30. AI Trace

لكل AI Run نربط:

business_id
conversation_id
message_id
ai_run_id
gemini_interaction_id
tool_call_id

ثم:

Input
↓
Gemini
↓
Tool Calls
↓
Proposal
↓
Validation
↓
Authorization
↓
Execution

---

31. Audit

Audit يسجل الأحداث التي نحتاج إثباتها:

AI Proposal Produced
Validation Result
Policy Evaluation
Authorization
Approval Requested
Human Approved
Human Rejected
Human Takeover
Action Executed
Action Failed

ولا يصبح Audit نسخة من قاعدة البيانات.

---

32. Business Truth vs AI Trace

PostgreSQL
=
Business Truth

AI Trace
=
What AI did/proposed

إذا قال Gemini:

amount = 12000

هذا ليس دليلًا كافيًا على السعر.

السعر الحقيقي يأتي من Catalog/Business Data.

---

33. Final End-to-End Flow

                  CUSTOMER
                      │
                      ▼
              Social Channel
                      │
                      ▼
                  SocialAPI
                      │
                      ▼
                 Mujeeb Go
                      │
          Verify / Normalize / Idempotency
                      │
                      ▼
              Conversation Context
                      │
          ┌───────────┼────────────┐
          │           │            │
          ▼           ▼            ▼
      Business     Conversation  Catalog
      Context        State       Entity Contract
          │           │            │
          └───────────┼────────────┘
                      ▼
                    Gemini
                      │
                      │
             ┌────────┴────────┐
             │                 │
             ▼                 ▼
       No Catalog Need     Catalog Needed
             │                 │
             │                 ▼
             │       Evaluation / Batching
             │                 │
             │        Actual Catalog Data
             │                 │
             │                 ▼
             └──────────► Gemini
                              │
                              ▼
                         AI Proposal
                              │
                              ▼
                         Validation
                              │
                              ▼
                       PolicyEvaluator
                              │
                              ▼
                        Authorization
                              │
                              ▼
                       Effective Decision
                              │
                 ┌────────────┼────────────┐
                 │            │            │
                 ▼            ▼            ▼
               Send         Human       Sales Flow
              Message      Handoff    Lead / Order
                 │
                 ▼
           Application
              Executor
                 │
                 ▼
          Provider Adapter
                 │
                 ▼
             SocialAPI
                 │
                 ▼
              CUSTOMER

---

34. أين يوجد كل شيء؟

Domain

Business
Customer
Conversation
Catalog
Lead
Order
Policies
Business Rules

Application

Context Builder
AI Orchestration
Catalog Evaluation
Validation
Policy Evaluation
Authorization
Execution Coordination

Infrastructure / Adapters

Gemini Adapter
SocialAPI Adapter
PostgreSQL
Redis
Workers
Queue
HTTP
Provider Integration
Observability

AI Runtime

AI Run
Attempts
Tool Loop
Timeout
Retry
Correlation

---

35. ما لا يوجد في Mujeeb

❌ search_catalog(query)
❌ Semantic Search
❌ Vector Search requirement
❌ Product Matching Engine
❌ AI Ranking Engine
❌ LLM Executor
❌ Direct Gemini → Provider access
❌ Gemini → SQL
❌ Gemini → business_id
❌ Chatwoot

---

36. ما هو مصدر الحقيقة لكل طبقة؟

Business Truth
→ PostgreSQL

Conversation Truth
→ Mujeeb

Catalog Truth
→ PostgreSQL / Catalog Domain

Policy Truth
→ Mujeeb Policy

AI Reasoning
→ Gemini

Authorization
→ Mujeeb

Execution Truth
→ Execution Result

---

37. الوضع الاقتصادي

الحد التجاري الخارجي:

AI Replies
+
Catalog Capacity

أما داخليًا:

Model Requests
Tool Calls
Tokens
Cache
Catalog Projection Tokens
AI Cost

ولا نحمّل التاجر تعقيد عدد استدعاءات Gemini الداخلي.

الـbaseline التجاري يفرق صراحة بين metric العميل وتكلفة AI الداخلية.

---

38. Architecture Boundary النهائية

┌──────────────────────────────────────────────┐
│                 MUJEEB 24                   │
│                                              │
│  Presentation                               │
│       ↓                                      │
│  Application                                │
│       ↓                                      │
│  Domain                                     │
│       ↓                                      │
│  Infrastructure / Adapters                 │
│                                              │
│  ┌────────────┐  ┌────────────┐             │
│  │ Gemini     │  │ SocialAPI  │             │
│  │ Adapter    │  │ Adapter    │             │
│  └─────┬──────┘  └─────┬──────┘             │
└────────┼────────────────┼────────────────────┘
         │                │
         ▼                ▼
      Gemini          SocialAPI

والـDatabase داخل حدود Mujeeb:

PostgreSQL
Redis
Workers

---

39. القرار النهائي

① Catalog AI Projection             CLOSED
② Catalog Evaluation + Batching     CLOSED
③ Conversation Context              CLOSED
④ Gemini System/Input/Output        CLOSED
⑤ Catalog Entity/Data Boundary      CLOSED
⑥ Validation + Authorization        CLOSED
⑦ Execution + Actions               CLOSED
⑧ Observability + Audit + Trace     CLOSED
⑨ Runtime + Retry + Timeout         CLOSED
⑩ Final Architecture Map            CLOSED

النتيجة

الآن لدينا خريطة معمارية موحدة:

CHANNEL
   ↓
MUJEEB
   ↓
CONTEXT
   ↓
GEMINI
   ↓
CATALOG DATA
   ↓
AI PROPOSAL
   ↓
VALIDATION
   ↓
POLICY
   ↓
AUTHORIZATION
   ↓
EXECUTION
   ↓
CHANNEL / SALES

مع:

AI Trace
Audit
Metrics
Retry
Timeout
Idempotency
Tenant Isolation

حول المسار كله.

⑩ Final Architecture Map = CLOSED