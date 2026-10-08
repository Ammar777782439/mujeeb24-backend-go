# Mujeeb 24 — Pricing Contract v1.0
## CLOSED ECONOMIC BASELINE

**Status:** CLOSED  
**Scope:** إطلاق Mujeeb 24 + أول 10 تجار  
**Currency:** YER

---

## 1. Customer Pricing

| Plan | Monthly Price | AI Replies / Month | Active AI Catalog Records | Channels |
|---|---:|---:|---:|---:|
| Mujeeb Basic | 5,000 YER | 500 | 200 | 1 |
| Mujeeb Growth | 10,000 YER | 1,500 | 750 | 2 |
| Mujeeb Business | 20,000 YER | 4,000 | 2,500 | 5 |

هذه هي أسعار الإطلاق المعتمدة.

### Definition: AI Reply
AI Reply = رد نهائي من AI تم قبوله للإرسال إلى العميل.

لا يعني:
- Gemini API request
- model call
- tool call
- token usage

قد يحتاج الرد الواحد إلى عدة model/tool calls، ويظل محسوبًا كتفاعل AI Reply واحد.

---

## 2. AI Catalog Capacity

السعة تعني الحد الأقصى من **السجلات النشطة من بيانات التاجر المتاحة للـAI** ضمن الباقة.

- Basic → 200
- Growth → 750
- Business → 2,500

قاعدة البيانات الكاملة لا تُحذف ولا تُقص؛ الـAI scope هو الذي يخضع للباقة.

---

## 3. AI Architecture

```text
Customer
   ↓
Mujeeb
   ↓
Gemini
   ↓
catalog_discovery عند الحاجة
   ↓
Mujeeb reads merchant-owned truth
   ↓
Gemini continues
   ↓
Final AI response
   ↓
Mujeeb validation
   ↓
Send / Handoff / Action
```

Gemini مسؤول عن:
- فهم رسالة العميل
- فهم السياق
- الاستدلال
- المقارنة
- اختيار ما يناسب الطلب
- تحديد الحاجة إلى بيانات إضافية
- صياغة الرد

Mujeeb مسؤول عن:
- Business / Tenant
- Authorization
- Catalog truth
- Database access
- Tenant isolation
- Validation
- Execution
- Economics / usage enforcement

---

## 4. Catalog Discovery

الأداة المعتمدة:

```text
catalog_discovery
```

وهي read-only boundary:

```text
Gemini
  ↓
catalog_discovery
  ↓
Mujeeb
  ↓
Repository
  ↓
PostgreSQL
```

لا يوجد:

```text
search_catalog(query)
```

ولا semantic search أو product-matching logic داخل Mujeeb كبديل عن فهم Gemini.

Gemini لا يحصل على SQL أو database access، ولا يختار `business_id`.

---

## 5. Discovery Loop

لا يوجد حد اصطناعي ثابت مثل:

```text
max 6 rounds
```

الاستمرار يتوقف عندما يتحقق أحد الآتي:

- sufficient evidence
- clarification needed
- information not found
- policy/security violation
- merchant entitlement exhausted
- provider/resource safety limit

إذا احتاج AI عدة استدعاءات ضرورية ومفيدة، فهذا مسموح.

Cost control يعتمد على:

- Merchant AI allowance
- Provider rate limits
- Maximum response/context size
- Infrastructure resource limits
- Economics of the merchant plan

**القاعدة:** حرية AI داخل حدود اقتصاد الباقة والأمان والمزود.

---

## 6. Catalog Projection

لا نرسل raw PostgreSQL rows إلى Gemini.

يُبنى:

```text
AI Catalog Projection
```

بصيغة compact، مثل:

```json
{
  "ref": "...",
  "type": "...",
  "name": "...",
  "summary": "...",
  "attributes": {}
}
```

والتفاصيل الثقيلة مثل:

```text
offer
availability
variant
details
```

تُستخرج عند الحاجة عبر Discovery / Expansion.

الـProjection هو Read Model للـAI، وليس Domain جديدًا.

---

## 7. Token / Batching Rule

لا يوجد عدد ثابت للعناصر في الـbatch.

التقسيم يكون حسب tokens:

```text
Projection
   ↓
Serialize
   ↓
count_tokens
   ↓
Batching
```

إذا كان النطاق أكبر من قدرة الـcontext:

```text
Batch 1
Batch 2
Batch 3
...
Batch N
```

الـController يضمن أن كل Item داخل النطاق المسموح تم إرساله للتقييم.

Gemini مسؤول عن الفهم والمقارنة والاستدلال؛ Mujeeb مسؤول عن coverage.

---

## 8. AI Usage Policy

لا نبيع للتاجر:

```text
tokens
API calls
model calls
tool calls
Gemini cost
```

التاجر يرى:

```text
AI Replies
372 / 500
```

و:

```text
AI Catalog
145 / 200
```

عند استنفاد الردود:

```text
500 / 500
```

يتم إيقاف الردود الآلية وفق سياسة الباقة، ويظهر للتاجر أنه يحتاج إلى ترقية الباقة لمواصلة الردود الآلية.

لا توجد فواتير AI غير محدودة.

---

## 9. Internal AI Cost Budget

هذه **ميزانيات إدارية للتخطيط** وليست تكلفة إنتاج مقاسة:

| Plan | Internal AI Budget / Business / Month |
|---|---:|
| Basic | ≈ 1,000 YER |
| Growth | ≈ 3,500 YER |
| Business | ≈ 9,000 YER |

Target mix:

```text
5 Basic  × 1,000  = 5,000
4 Growth × 3,500  = 14,000
1 Business × 9,000 = 9,000
--------------------------------
Total AI budget     = 28,000 YER/month
```

قبل الإطلاق العام يجب قياس:
- input tokens
- cached tokens
- output tokens
- model requests
- tool calls
- catalog projection tokens
- cost per AI reply

---

## 10. First 10 Merchants — Target Mix

```text
5 Basic
4 Growth
1 Business
```

Monthly revenue:

```text
5 × 5,000   = 25,000
4 × 10,000  = 40,000
1 × 20,000  = 20,000
----------------------
TOTAL       = 85,000 YER/month
```

Total included AI replies:

```text
5 × 500    = 2,500
4 × 1,500  = 6,000
1 × 4,000  = 4,000
----------------------
TOTAL      = 12,500 AI Replies
```

Total AI catalog capacity:

```text
5 × 200    = 1,000
4 × 750    = 3,000
1 × 2,500  = 2,500
----------------------
TOTAL      = 6,500 active AI records
```

هذه ليست 6,500 record في prompt واحد؛ لكل Business scope مستقل.

---

## 11. Planned Infrastructure Cost — First 10 Merchants

```text
SocialAPI   15,457 YER/month
VPS         10,000 YER/month
Domain         500 YER/month allocation
Backup        1,000 YER/month
--------------------------------
TOTAL        26,957 YER/month
```

Architecture:

```text
VPS واحد
├── Next.js
├── Go API
├── Workers
├── PostgreSQL
├── Redis
├── Media
├── Reverse Proxy
├── Monitoring
└── Backup agent
```

الخدمات الخارجية:

```text
Gemini
SocialAPI
Meta
```

لا Chatwoot.

لا Vector DB requirement في هذا baseline.

---

## 12. Planned COGS

```text
AI            28,000
SocialAPI     15,457
VPS           10,000
Domain           500
Backup          1,000
---------------------
TOTAL         54,957 YER/month
```

Contribution:

```text
Revenue = 85,000
COGS    = 54,957
----------------
Contribution = 30,043 YER/month
```

هذا **ليس راتبًا**؛ هو contribution قبل المصاريف العامة، ورسوم التحصيل، والتسويق، والرواتب، والمصاريف القانونية/المحاسبية وغيرها.

---

## 13. Economic Guardrail

لا نوقف AI لأن عدد الجولات وصل إلى رقم اعتباطي.

القاعدة:

```text
AI usage normal
    → continue

AI usage expensive but useful
    → continue

AI usage breaks merchant economic allowance
    → controlled degradation / upgrade / policy
```

Pricing لا يتغير لأن ردًا فرديًا احتاج عدة model/tool calls.

---

## 14. Final Closed Baseline

```text
MUJEEB 24 — PRICING CONTRACT v1.0

CUSTOMER PRICING
Basic:
  5,000 YER/month
  500 AI Replies
  200 active AI catalog records
  1 channel

Growth:
  10,000 YER/month
  1,500 AI Replies
  750 active AI catalog records
  2 channels

Business:
  20,000 YER/month
  4,000 AI Replies
  2,500 active AI catalog records
  5 channels

TARGET MIX
  5 Basic
  4 Growth
  1 Business

MONTHLY REVENUE
  85,000 YER

AI BILLING
  Merchant-facing metric = final AI Replies only.
  Internal model requests/tool calls are hidden.

CATALOG
  Plan capacity controls maximum active catalog records
  available to AI.
  Compact AI Catalog Projection.
  Full entitled catalog scope must be available to AI.
  Discovery Tool for additional evidence/detail.
  No fixed round ceiling.

DISCOVERY
  Read-only catalog_discovery.
  Gemini decides when to call it.
  Mujeeb executes against authenticated business only.
  No SQL/database access for Gemini.
  Structured JSON.

INFRASTRUCTURE
  First 10 merchants = one VPS.
  PostgreSQL + Redis + Go API + Workers + Next.js + Media.
  NO CHATWOOT.
  NO VECTOR DB requirement.

PLANNED COGS
  AI              28,000
  SocialAPI       15,457
  VPS             10,000
  Domain             500
  Backup            1,000
  TOTAL            54,957 YER/month

PLANNED CONTRIBUTION
  30,043 YER/month

STATUS
  CLOSED
```
