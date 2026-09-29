MUJEEB 24

Platform Administration Contract V1

العقد النهائي لمدير النظام — Design CLOSED

الحالة: تصميم نهائي مغلق
دور المنفذ: تنفيذ هذا العقد كما هو
دور Platform Architect: تحديد المسؤوليات والحدود والقواعد والـAPI والحالات والأمان
لا توجد قرارات مفتوحة داخل هذا العقد.

---

1. تعريف مدير النظام

مدير النظام في Mujeeb 24 هو:

platform_super_admin

وهوية مدير النظام مستقلة عن:

merchant owner
merchant admin
manager
agent
analyst
viewer

ولا تصبح هوية "platform_super_admin" عضوًا في Business لمجرد أنها مديرة للمنصة.

الحساب الحالي للـPlatform Super Admin موجود أصلًا ضمن "platform_super_admins" ومسار bootstrap المركزي، وهذه القاعدة تبقى كما هي.

---

2. إنشاء حساب مدير النظام

لا يوجد:

Public Platform Admin Signup

ولا:

Register as Super Admin

ولا زر داخل الواجهة ينشئ Super Admin.

إنشاء مدير النظام يتم من Backend فقط عبر:

cmd/bootstrap-platform-admin

قواعد الـBootstrap

1. بيانات الدخول تأتي من Secret/Environment وليس من الكود.
2. كلمة المرور لا تحفظ كنص صريح.
3. تستخدم نفس آلية Bcrypt المعتمدة في النظام.
4. يتم إنشاء سجل "platform_super_admin".
5. إذا كان السجل موجودًا بالفعل، لا يعاد إنشاء حساب آخر بنفس الهوية.
6. الـBootstrap لا يعدل كلمة مرور حساب موجود تلقائيًا.
7. لا توجد كلمة مرور افتراضية.
8. لا يسمح بتشغيل الـBootstrap بدون Email + Password صالحين.
9. عملية الـBootstrap يجب أن تكون Idempotent من ناحية الإنشاء، ولكنها لا تقوم بعملية Password Reset صامتة.

---

3. Authentication الخاصة بمدير النظام

مدير النظام يدخل من:

/admin

والواجهة تستخدم Platform Authentication.

العقد المنطقي للتوكن:

principal_type = platform_super_admin
principal_id   = <platform_admin_id>

ويستخدم النظام الحالي:

Ed25519 JWT
+
Stateful Refresh Session

ولا ننشئ نظام Authentication ثانٍ مختلف عن البنية الحالية.

لكن:

platform token
        ≠
merchant token

والـPlatform token لا يتحول إلى Merchant Membership.

---

4. قاعدة أمنية مركزية

هذه أهم قاعدة في العقد:

«Platform Super Admin يملك Platform Scope وليس Merchant Scope.»

أي:

Platform Admin
      ↓
Platform Services

وليس:

Platform Admin
      ↓
business_id
      ↓
Merchant Repository

ولا يسمح للـPlatform Admin باختراع "business_id" واستخدامه لتجاوز Tenant Isolation.

Tenant Isolation الحالي مغلق كقاعدة أساسية في النظام.

---

5. نطاق مدير النظام الكامل

مدير النظام V1 يملك ستة Domains فقط:

Platform Administration
├── Platform Identity
├── Business Management
├── Plan Management
├── Subscription Management
├── Support Management
└── Platform Audit

وهناك:

Platform Overview

وهي View مشتقة من هذه الـDomains وليست Domain مستقلة.

---

6. ما ليس من مسؤولية مدير النظام

مدير النظام لا يملك من خلال Platform API صلاحية:

Merchant Catalog Editing
Customer Editing
Lead Editing
Transaction Editing
Conversation Reading
Message Sending
Merchant Team Management
Merchant Policy Editing
Merchant AI Decision Editing
Merchant Channel Configuration
Merchant Social Credentials
Merchant Business Content

مدير النظام لا يستخدم PostgreSQL كـSuperuser للوصول إلى بيانات التجار.

كذلك لا نبني في V1 داخل Platform Administration:

AI Provider Management UI
Social Provider Management UI
Infrastructure Control Panel
Secrets Management UI
Generic Feature Flag System
Generic Workflow Builder
Generic CRM

إعدادات الخوادم والمفاتيح السرية والبنية التحتية تبقى ضمن Deployment/Operations، وليست ضمن Dashboard مدير النظام.

---

7. Platform Overview

واجهة:

/admin

تبدأ بـ:

Overview
Businesses
Subscriptions
Plans
Support
Audit

Overview يعرض فقط معلومات Platform-owned:

Total Businesses
Active Businesses
Suspended Businesses
Archived Businesses

Active Subscriptions
Expired Subscriptions

Open Support Tickets
High/Critical Support Tickets

Subscription Payments Recorded

ولا يعرض:

Customer Count
Message Content
Conversation Content
Merchant Catalog Details
Private AI Context
Merchant Orders

لأن Overview منصة وليس Merchant Analytics.

---

8. Business Management

مسؤولية Domain

Business Management مسؤول عن وجود التاجر على المنصة وحالته الإدارية.

لا مسؤولية له عن المحتوى الداخلي للتاجر.

---

9. إنشاء Business

مدير النظام يستطيع إنشاء:

Business
+
Initial Owner Invitation

ولا يدخل مدير النظام كلمة مرور المالك.

العملية:

Platform Admin
      ↓
Create Business
      ↓
Create Owner Invitation
      ↓
Owner accepts invitation
      ↓
Owner sets own password
      ↓
Merchant onboarding

نستخدم آلية الدعوات الموجودة في النظام بدل مشاركة كلمات المرور. عقد الدعوات الحالي يستخدم رمزًا لمرة واحدة وتخزين SHA-256 للرمز.

---

10. Business Platform Status

حالة Business على مستوى المنصة هي:

ACTIVE
SUSPENDED
ARCHIVED

هذه هي Platform Lifecycle Status.

أما جاهزية التاجر الداخلية مثل القنوات والسياسات والإعدادات فهي Merchant Onboarding وليست Platform Status.

---

11. انتقالات Business Status

ACTIVE
   │
   ├── suspend ──→ SUSPENDED
   │                    │
   │                    └── reactivate ──→ ACTIVE
   │
   └── archive ──→ ARCHIVED

ومن:

SUSPENDED
   └── archive ──→ ARCHIVED

والحالة:

ARCHIVED

نهائية في V1 ولا يوجد Command عادي:

restore archived business

---

12. معنى Business Suspension

عند:

SUSPENDED

يمنع النظام العمليات التجارية التي تحتاج Business Active.

ويظل الحساب موجودًا وبياناته محفوظة.

لا يعني Suspend:



ولا:



ولا:



ولا:



---

13. Business Directory

API:

POST /api/v1/platform/businesses
GET  /api/v1/platform/businesses
GET  /api/v1/platform/businesses/{business_id}

List

تدعم:

Pagination
Search
Status Filter
Created Date Filter

Platform Business View

يعرض فقط:

business_id
business_name
owner_identity_summary
platform_status
created_at
updated_at
subscription_summary

ولا يعرض محتوى التاجر.

---

14. Business Commands

POST /api/v1/platform/businesses/{business_id}/suspend
POST /api/v1/platform/businesses/{business_id}/reactivate
POST /api/v1/platform/businesses/{business_id}/archive

لا نستخدم:

PATCH /business/{id}

لتغيير الحالة.

الحالات Domain Commands.

---

15. Plan Management

الـPlan هو تعريف تجاري مملوك للمنصة.

الـPlan يحدد:

Price
Billing Interval
AI Reply Limit
AI Catalog Limit
Channel Limit

---

16. Plans المعتمدة

هذه هي باقات Mujeeb 24 المعتمدة:

Basic

Price:
5,000 YER / month

AI Replies:
500

AI Catalog Records:
200

Channels:
1

Growth

Price:
10,000 YER / month

AI Replies:
1,500

AI Catalog Records:
750

Channels:
2

Business

Price:
20,000 YER / month

AI Replies:
4,000

AI Catalog Records:
2,500

Channels:
5

هذه هي الـcommercial baseline المعتمدة في المشروع.

---

17. Plan Versioning Rule

الـPlan لا يعدّل إذا أصبح مستخدمًا في Subscription.

بمجرد ارتباط Plan باشتراك:

price
limits
entitlements

تصبح تاريخية.

أي تغيير في السعر أو الحدود ينشئ:

New Plan Version

ولا نغير التاريخ القديم.

---

18. Plan Status

DRAFT
ACTIVE
RETIRED

DRAFT

غير متاح للاشتراك.

ACTIVE

يمكن إنشاء اشتراك جديد عليه.

RETIRED

لا يمكن إنشاء اشتراكات جديدة عليه، لكن الاشتراكات التاريخية المرتبطة به تبقى صحيحة.

---

19. Plan Commands

POST /api/v1/platform/plans
GET  /api/v1/platform/plans
GET  /api/v1/platform/plans/{plan_id}

POST /api/v1/platform/plans/{plan_id}/activate
POST /api/v1/platform/plans/{plan_id}/retire

لا يوجد:

DELETE /platform/plans/{id}

ولا تعديل تاريخي يكسر Subscriptions السابقة.

---

20. Subscription Management

الـSubscription تربط:

Business
      ↓
Subscription Period
      ↓
Plan

كل Subscription تمثل فترة اشتراك واضحة.

---

21. Subscription States

PENDING
ACTIVE
EXPIRED
CANCELLED

PENDING

تم إنشاء الاشتراك لكن لم يتم تسجيل الدفع المطلوب.

ACTIVE

الدفع مسجل والاشتراك فعّال.

EXPIRED

انتهت فترة الاشتراك.

CANCELLED

أوقفه Platform Admin قبل انتهاء الفترة.

---

22. قاعدة تاريخ الاشتراكات

لا يتم إعادة استخدام Subscription القديمة لدورة جديدة.

التجديد ينشئ:

New Subscription Period

وبالتالي:

Subscription #1
    ↓
expired

Subscription #2
    ↓
active

هذا يحافظ على تاريخ الاشتراكات بدون تعديل سجلات الماضي.

---

23. Billing Interval

V1 يستخدم:

MONTH

الاشتراك الشهري يحسب بفترة شهر تقويمي، وليس بعدد Calls أو عدد أيام تقريبية.

---

24. تغيير الباقة

V1 لا يدعم Proration.

ولا نحسب:

Partial Refund
Unused Days Credit
Prorated Upgrade

تغيير الباقة يتم عند إنشاء Subscription Period جديد.

وبالتالي:

Current Subscription
      ↓
Ends
      ↓
New Subscription
      ↓
New Plan

هذا يمنع الحسابات التجارية المعقدة ويجعل السجل المالي deterministic.

---

25. Subscription Payment Model

Mujeeb V1 لا يعتمد على:

Visa Checkout
Stripe Checkout
PayPal
Online Card Gateway

الدفع التجاري في V1:

MANUAL PAYMENT

ويتولى Platform Admin تسجيل الدفع بعد التحقق منه خارج Mujeeb.

---

26. Payment Methods

القيمة المعتمدة:

CASH
BANK_TRANSFER
MOBILE_MONEY
OTHER

ولا نضع Provider-specific payment logic داخل Subscription Domain.

---

27. Payment Record

كل عملية دفع تسجل:

payment_id
subscription_id
business_id
amount_yer
method
reference
paid_at
recorded_by
created_at

Payment Record append-only.

لا يتم تعديل مبلغ دفع مسجل.

التصحيح يتم عبر عملية تصحيح موثقة وليس تعديل التاريخ بصمت.

---

28. تفعيل Subscription

العملية:

Create Subscription
        ↓
PENDING
        ↓
Record Payment
        ↓
ACTIVE

لا يصبح Subscription:

ACTIVE

بدون تحقق من عملية الدفع أو قرار تفعيل إداري موثق.

---

29. انتهاء Subscription

عند:

period_end

تتحول Subscription إلى:

EXPIRED

ولا نحذف:

Business
Catalog
Customers
Conversations
Leads
Transactions

---

30. تأثير Expired Subscription

انتهاء الاشتراك لا يعني حذف الحساب.

لكن Entitlements التجارية تتوقف حسب العقد.

أي أن النظام يمنع العمليات التي تتطلب Subscription فعالة، مع إبقاء بيانات الحساب محفوظة وإتاحة الدخول لواجهة الحساب وإتمام التجديد.

---

31. AI Reply Entitlement

الـAI Reply Limit يحسب:

Final AI Replies

وليس:

Model Requests
Tool Calls
Internal Reasoning
Context Builds
Provider Calls

وهذا مطابق للـbaseline الاقتصادي المعتمد.

---

32. AI Usage

لكل Subscription فترة:

ai_replies_used

ويزداد فقط عند اعتماد/إرسال Final AI Reply الذي يدخل ضمن الفاتورة.

إذا وصل:

ai_replies_used >= plan.ai_reply_limit

فإن:

AI Auto Reply

يتوقف.

ولا يمنع:

Human Reply

---

33. Catalog Entitlement

"AI Catalog Limit" يحدد الحد الأقصى لسجلات الكتالوج المتاحة للـAI ضمن Projection.

لا يعني ذلك حذف سجلات Merchant Catalog من PostgreSQL.

الحد هو:

AI Entitlement

وليس:

Database Storage Limit

---

34. قاعدة تجاوز Catalog Limit

لا يسمح النظام برفع عدد السجلات الداخلة في AI-active scope فوق الحد الخاص بالـPlan.

وعند خفض الباقة لا يتم حذف بيانات التاجر تلقائيًا.

إذا كان الوضع الحالي أعلى من الحد الجديد، فإن تغيير الاشتراك إلى الخطة الأقل لا يصبح ACTIVE حتى يُعالج التجاوز.

لا يوجد:

Silent Data Deletion

---

35. Channel Entitlement

كل Subscription تحدد:

maximum active channels

إذا كانت الخطة:

1 channel

لا يسمح بوجود أكثر من قناة نشطة ضمن entitlement.

إذا تم الانتقال إلى Plan أقل والعدد الحالي أعلى من حد الخطة الجديدة، لا يتم تفعيل الاشتراك الجديد حتى يعالج التاجر التجاوز.

لا نفصل قناة تلقائيًا.

---

36. Subscription APIs

GET  /api/v1/platform/subscriptions
GET  /api/v1/platform/subscriptions/{subscription_id}

POST /api/v1/platform/businesses/{business_id}/subscriptions
POST /api/v1/platform/subscriptions/{subscription_id}/payments
POST /api/v1/platform/subscriptions/{subscription_id}/cancel

والتجديد:

POST /api/v1/platform/businesses/{business_id}/subscriptions

ينشئ Period جديدًا.

---

37. Support Management

Support هو Domain رسمي لمدير النظام لأن مسؤول النظام مسؤول عن الدعم.

لا نستخدم Chatwoot كـSupport System.

Mujeeb يملك Support Ticket Domain.

---

38. Support Ticket

الـTicket مرتبط بـBusiness:

Business
   ↓
Support Ticket
   ↓
Support Messages

---

39. Ticket States

OPEN
IN_PROGRESS
WAITING_MERCHANT
RESOLVED
CLOSED

OPEN

تذكرة جديدة لم تبدأ معالجتها.

IN_PROGRESS

يعمل عليها Support.

WAITING_MERCHANT

بانتظار رد التاجر.

RESOLVED

المشكلة عولجت.

CLOSED

أغلقت نهائيًا.

---

40. Ticket Priority

NORMAL
HIGH
CRITICAL

---

41. Ticket Categories

ACCOUNT
CHANNEL
AI
CATALOG
SALES
BILLING
OTHER

---

42. Support Message

كل Ticket يحتوي Messages:

support_message_id
ticket_id
author_type
author_id
body
created_at

والـ"author_type":

BUSINESS_USER
PLATFORM_ADMIN

لا توجد ملفات أو Attachments داخل Support V1 لأن Media Upload API ليس جزءًا من العقد الحالي.

---

43. Merchant Support Flow

التاجر:

Create Ticket
      ↓
OPEN
      ↓
Platform Admin replies
      ↓
IN_PROGRESS
      ↓
Merchant replies
      ↓
WAITING/IN_PROGRESS
      ↓
RESOLVED
      ↓
CLOSED

---

44. Platform Support APIs

GET  /api/v1/platform/support/tickets
GET  /api/v1/platform/support/tickets/{ticket_id}

POST /api/v1/platform/support/tickets/{ticket_id}/messages

POST /api/v1/platform/support/tickets/{ticket_id}/start
POST /api/v1/platform/support/tickets/{ticket_id}/resolve
POST /api/v1/platform/support/tickets/{ticket_id}/close

والبحث يدعم:

status
priority
category
business
date range

---

45. Platform Audit

Platform Audit منفصل تمامًا عن Merchant Audit.

Merchant Audit
    ↓
Business Scope

Platform Audit
    ↓
Platform Scope

---

46. ما الذي يسجل في Platform Audit؟

كل Platform Command:

business.created
business.suspended
business.reactivated
business.archived

plan.created
plan.activated
plan.retired

subscription.created
subscription.activated
subscription.cancelled
subscription.expired

payment.recorded

support.ticket.started
support.ticket.resolved
support.ticket.closed
support.message.created

وكذلك العمليات الحساسة التي تقرأ Platform-level information.

---

47. Audit Event

event_id
actor_platform_admin_id
action
target_type
target_id
business_id nullable
result
correlation_id
occurred_at
metadata

---

48. Audit Rules

Audit:

APPEND ONLY

ولا يوجد:

PATCH audit event
DELETE audit event

ولا نحفظ في Audit:

password
JWT
refresh token
provider secret
social token

ولا نضع نص محادثات العملاء في Platform Audit.

---

49. Platform Audit API

GET /api/v1/platform/audit-events
GET /api/v1/platform/audit-events/{event_id}

مع:

Pagination
Actor Filter
Action Filter
Target Filter
Result Filter
Date Range

---

50. Platform Dashboard UI

الواجهة النهائية:

/admin
│
├── Overview
│
├── Businesses
│   ├── List
│   └── Business Details
│
├── Plans
│   ├── List
│   └── Plan Details
│
├── Subscriptions
│   ├── List
│   └── Subscription Details
│
├── Support
│   ├── Tickets
│   └── Ticket Details
│
└── Audit
    ├── Events
    └── Event Details

لا توجد قائمة:

Customers
Conversations
Orders
Catalog
AI Conversations
Merchant Team

داخل Platform Admin.

---

51. Business Details في Admin

Business Details يعرض:

Business Identity
Owner Identity Summary
Platform Status
Onboarding Summary
Subscription Summary
Created At
Last Updated

ولا يعرض:

Customer Records
Conversation Messages
Order Contents
Full Catalog
AI Context
Merchant Secrets

---

52. Subscription Details

يعرض:

Business
Plan
Price
Period Start
Period End
Status
AI Replies Used
AI Reply Limit
Catalog Entitlement
Channel Entitlement
Recorded Payments

ولا يعرض Token-level AI provider accounting.

---

53. Plan Details

يعرض:

Plan Code
Version
Price
Billing Interval
AI Reply Limit
AI Catalog Limit
Channel Limit
Status
Usage History / References

Plan التاريخي لا يتم تغييره بعد استخدامه.

---

54. Support Details

يعرض:

Business
Ticket
Priority
Category
Status
Conversation
Messages
Timeline

الوصول هنا مشروع لأن البيانات التي يقرأها Platform Admin هي Support data التي دخلت إلى منصة الدعم نفسها.

هذا لا يعطيه تلقائيًا صلاحية قراءة Merchant Conversations.

---

55. Authorization Model

V1 لا يبني نظام Roles معقدًا للـPlatform.

لدينا:

platform_super_admin

كـPlatform Role.

وجميع Platform APIs تتحقق من:

authenticated
+
platform principal
+
platform scope

ولا يوجد:

merchant membership fallback

---

56. Platform API Boundary

كل واجهات مدير النظام تقع تحت:

/api/v1/platform/*

ولا نخلطها مع:

/api/v1/businesses/*

Merchant API:

/api/v1/businesses/{business_id}/...

Platform API:

/api/v1/platform/...

وهذا الفصل إلزامي.

---

57. Error Contract

Platform API يستخدم نفس Error Contract العام الموجود في Mujeeb.

الحالات الأساسية:

401
Authenticated identity missing/invalid

403
Identity authenticated but not Platform Authorized

404
Platform resource does not exist

409
Invalid lifecycle transition / conflict

422
Invalid business rule

500
Unexpected system failure

---

58. Pagination

كل List endpoint يستخدم Pagination Contract الموجود في Mujeeb.

لا نبني Pagination مختلفة للـPlatform.

---

59. Idempotency

Commands الحساسة التي قد يعاد إرسالها بسبب retry تدعم Idempotency حيث تكون العملية قابلة للتكرار.

خصوصًا:

Business creation
Subscription creation
Payment recording
Support message creation

ولا ينتج عن retry غير المقصود:

duplicate subscription
duplicate business
duplicate support message

---

60. Database Ownership

الجداول التي يملكها Platform Administration:

platform_super_admins
platform_audit_events
plans
subscriptions
subscription_payments
support_tickets
support_messages
subscription_usage

أما:

Business
Customers
Catalog
Conversations
Messages
Leads
Transactions
Policies
Channels

فتبقى ملك Merchant Domains الخاصة بها.

---

61. قاعدة Foreign Keys

كل Platform resource مرتبط بـBusiness، عند الحاجة، يحتفظ بالـBusiness relation بشكل صريح وآمن.

مثلاً:

subscriptions.business_id
support_tickets.business_id
platform_audit_events.business_id

لكن هذا لا يحول Platform Admin إلى Merchant Member.

"business_id" هنا مرجع إداري وليس Merchant Scope Authorization.

---

62. Separation of Authorization

لدينا مستويان منفصلان:

Merchant Authorization
    ↓
Business Membership + Permissions

و:

Platform Authorization
    ↓
Platform Super Admin

ولا يسمح للنظام باستخدام أحدهما بدل الآخر.

---

63. Platform Admin لا يستطيع تنفيذ Merchant Command

مثال:

POST /api/v1/businesses/BUSINESS_ID/leads

بتوكن Platform Admin فقط:

403

حتى لو كان التوكن صالحًا.

هذه قاعدة مقصودة.

---

64. Merchant Owner لا يستطيع تنفيذ Platform Command

مثال:

POST /api/v1/platform/businesses/{id}/suspend

بتوكن Merchant Owner:

403

حتى لو كان Owner.

---

65. Business Suspension لا يساوي Subscription Expiration

نفصل الاثنين:

Business Status
    ACTIVE / SUSPENDED / ARCHIVED

و:

Subscription Status
    PENDING / ACTIVE / EXPIRED / CANCELLED

السبب:

قد يكون الحساب نشطًا لكن الاشتراك منتهيًا.

وقد يكون الاشتراك مدفوعًا لكن الحساب موقوفًا إداريًا.

ولا نخلط الحالتين في حقل واحد.

---

66. سبب وجود هذا الفصل

مثال:

Business = ACTIVE
Subscription = EXPIRED

معناه:

الحساب موجود
بياناته موجودة
لكن لا توجد Entitlement تجارية فعالة

بينما:

Business = SUSPENDED
Subscription = ACTIVE

معناه:

الاشتراك موجود
لكن المنصة أوقفت الحساب

هذه حالتان مختلفتان قانونيًا وتجاريًا وتقنيًا.

---

67. Merchant Data Protection

Platform Dashboard لا يحتوي على:

Raw SQL
Database Explorer
Table Explorer
Impersonation

ولا نضيف زر:

Login as Merchant

ضمن V1.

لا يوجد Silent Impersonation.

---

68. أسرار التكامل

Platform Admin UI لا يعرض:

SocialAPI Secret
Meta Token
Gemini API Key
JWT Signing Private Key
Database Password
Redis Password

هذه أسرار Deployment/Secrets Management.

---

69. مصدر الحقيقة في التنفيذ

عند تنفيذ هذا العقد:

Go HTTP DTOs
+
Registered Operations
=
HTTP API Contract

ولا يتم تحرير OpenAPI يدويًا.

والكود + الاختبارات هما الحكم على حالة التنفيذ الفعلية.

---

70. Implementation Order

تنفيذ Platform Administration يكون بهذا الترتيب فقط:

1. Platform Authentication
2. Platform Admin Guard
3. Platform Audit Foundation
4. Business Management
5. Plan Management
6. Subscription Management
7. Subscription Usage
8. Manual Payment Recording
9. Support Management
10. Platform Overview
11. Platform Dashboard

ولا نبدأ Dashboard قبل إغلاق الـApplication/API contracts.

---

71. Contract Closure Checklist

العقد يعتبر CLOSED تصميميًا عندما تكون العناصر التالية موجودة:

[✓] Platform identity
[✓] Bootstrap
[✓] Authentication boundary
[✓] Authorization boundary
[✓] Business lifecycle
[✓] Business commands
[✓] Business API
[✓] Plans
[✓] Plan versioning
[✓] Subscription lifecycle
[✓] Entitlements
[✓] AI usage rule
[✓] Catalog entitlement
[✓] Channel entitlement
[✓] Manual payment
[✓] Support ticket lifecycle
[✓] Support API
[✓] Platform audit
[✓] Audit API
[✓] Platform dashboard
[✓] Tenant boundary
[✓] Error semantics
[✓] Pagination
[✓] Idempotency
[✓] Database ownership
[✓] Explicit exclusions

---

72. القرار النهائي

من الآن Platform Administration Contract V1 هو المرجع المعتمد.

المسؤوليات الوحيدة لمدير النظام:

PLATFORM
BUSINESSES
PLANS
SUBSCRIPTIONS
PAYMENTS
SUPPORT
AUDIT

ولا يوجد ضمنه صلاحية صامتة على بيانات التاجر.

---

73. حالة التنفيذ

يجب التفريق بين:

DESIGN

و:

IMPLEMENTATION

القرار المعماري الآن:

Platform Administration Contract
✅ CLOSED


MUJEEB 24

Platform Administration Contract V1

الإضافة النهائية: Platform Operations

الحالة: CLOSED

هذه الإضافة جزء رسمي من "Platform Administration Contract V1".

أصبح نطاق مدير النظام النهائي:

Platform Administration
├── Platform Identity
├── Business Management
├── Plan Management
├── Subscription Management
├── Support Management
├── Platform Operations
└── Platform Audit

---

73. Platform Operations

"Platform Operations" هو المجال المسؤول عن صحة وتشغيل البنية المنطقية التي يعتمد عليها Mujeeb 24.

وهو لا يدير بيانات التجار التجارية، بل يراقب:

AI Runtime
Channel Connections
External Providers

وبذلك يستطيع مدير النظام معرفة:

هل AI يعمل؟
هل القنوات تعمل؟
هل SocialAPI يعمل؟
هل Gemini يعمل؟
هل توجد أعطال؟
هل توجد مشاكل واسعة على مستوى المنصة؟

---

74. Platform Operations لا يملك Merchant Data

هذه القاعدة إلزامية:

Platform Operations
        ↓
Operational metadata
        ↓
Health / Status / Usage / Failures

وليس:

Platform Operations
        ↓
Merchant messages
Merchant customers
Merchant catalog
Merchant orders
Merchant conversations

مدير النظام يرى أن قناة التاجر متوقفة، لكنه لا يحتاج إلى قراءة محادثات العميل لمعرفة ذلك.

---

75. AI Management

مدير النظام يحصل على شاشة:

/admin/ai

وتحتوي على:

AI Runtime
AI Provider
Model
Health
Usage
Failures

---

76. AI Provider الحالي

المزود الإنتاجي المعتمد حاليًا:

Provider:
Google Gemini

والـModel الإنتاجي الافتراضي (fallback bootstrap):

gemini-3.5-flash-lite

(إذا تم تعيين GEMINI_MODEL في الـenv يُستخدم ذلك بدلًا منه.)

> **CONTRACT DRIFT NOTE (P2-15 fix, 2026-09-29):**
> النص الأصلي للعقد (§76) أشار إلى `gemini-3.1-flash-lite` كنموذج إنتاجي — لكن
> التطبيق الفعلي يستخدم `gemini-3.5-flash-lite` كـfallback (انظر
> `internal/bootstrap/api.go:186`). تم تحديث العقد ليعكس التطبيق الحقيقي.
>
> كذلك أشار العقد الأصلي (§86-87) إلى أن تغيير الـmodel/credential من
> الـDashboard غير مدعوم في V1 — لكن إضافة `AI Provider Configuration`
> (المسارات `/api/v1/platform/operations/ai/*`) تجعل تغيير الـmodel
> والـcredential مدعومًا وقت التشغيل عبر `platformUpdateAIConfiguration`
> و `platformAddAICredential`. هذه الإضافة **تُلغي** قيد §86-87 القديم
> (الـAPI الحالي هو المصدر الموثوق).
>
> لم يتم حذف أي API موجود — التحديث يعكس فقط الواقع الحالي للتنفيذ.

وهو الـbaseline الاقتصادي/التقني المعتمد حاليًا للمشروع.

---

77. AI Provider Architecture

لا نربط Domain مباشرة بـGemini.

الطبقة:

Mujeeb AI Runtime
        ↓
AI Provider Port
        ↓
Gemini Adapter
        ↓
Google Gemini

والـbaseline الحالي يثبت وجود "gemini.Client" وOpenAI-compatible client ضمن تكاملات AI.

---

78. Provider Registry

داخل Platform Operations يوجد:

AI Providers

وكل Provider له:

provider_id
provider_type
display_name
enabled
health_state
last_health_check
last_success
last_failure

لا يخزن الـDashboard:

API Key
Secret
Private Credential

---

79. AI Provider States

نستخدم حالتين مستقلتين.

Administrative State

ENABLED
DISABLED

Health State

HEALTHY
DEGRADED
DOWN
UNKNOWN

ولا نخلط:

DISABLED

مع:

DOWN

مثلاً:

Provider = ENABLED
Health = DOWN

معناه أن Mujeeb يريد استخدامه، لكن المزود حاليًا غير متاح.

أما:

Provider = DISABLED
Health = UNKNOWN

فهو معطل إداريًا ولا يعتمد عليه Runtime.

---

80. AI Runtime State

AI Runtime نفسه يملك:

ENABLED
DISABLED

ويملك Health مستقل:

HEALTHY
DEGRADED
DOWN

الـRuntime هو الذي يقرر هل عمليات AI يمكن أن تبدأ أصلًا.

---

81. AI Emergency Kill Switch

نغلق هذه الوظيفة رسميًا.

مدير النظام يستطيع:

Disable AI Runtime
Enable AI Runtime

الاستخدام:

AI Provider failure
AI safety incident
Unexpected AI behavior
Cost protection
Operational emergency

عند:

AI Runtime = DISABLED

يتوقف:

AI Auto Reply
AI Automated Sales Decisions
AI AI-driven Automation

لكن لا يتوقف:

Human Reply
Merchant Dashboard
Customer Data
Leads
Orders
Channel Reception

وكل عملية Disable / Enable تسجل Platform Audit.

---

82. AI Usage Monitoring

مدير النظام يرى الاستخدام المجمع على مستوى المنصة:

AI Replies
Model Requests
Provider Requests
Tool Calls
Input Usage
Output Usage
Failures

لكن يجب التمييز بين:

Merchant Billing Usage

و:

Provider Operational Usage

التاجر يحاسب حسب:

Final AI Replies

أما Platform Admin فيستطيع مراقبة الاستهلاك الداخلي الذي يستخدمه النظام للتكلفة والتشغيل.

---

83. AI Usage لا يعرض محتوى العميل

Monitoring يعرض:

counts
rates
latency
errors
provider/model

ولا يعرض:

Customer Message
Full AI Prompt
Private Business Knowledge
Merchant Secret

إلا عبر Merchant-specific surfaces التي يملكها ذلك الـDomain وبصلاحياتها الخاصة.

---

84. AI Health Check

يوجد:

POST /api/v1/platform/ai/health-check

الـHealth Check يستخدم Probe مستقل لا يعتمد على محادثة تاجر.

ولا يستخدم:

merchant_id
business_id
customer data
merchant catalog

والـProbe نتيجته:

HEALTHY
DEGRADED
DOWN

وتسجل النتيجة:

provider
model
checked_at
latency
result
failure_code

---

85. AI API

GET  /api/v1/platform/ai
GET  /api/v1/platform/ai/providers
GET  /api/v1/platform/ai/providers/{provider_id}

POST /api/v1/platform/ai/health-check
POST /api/v1/platform/ai/disable
POST /api/v1/platform/ai/enable

ولا يوجد:

PATCH /api/v1/platform/ai

لتغيير إعدادات AI الحساسة بشكل عام.

---

86. Model Switching

V1 لا يسمح لمدير النظام بتغيير Model من Dashboard بشكل حر.

لا يوجد:

PATCH model = ...

الـModel الإنتاجي الحالي:

gemini-3.1-flash-lite

وأي تغيير في Model هو Deployment Configuration Change يراجع ويطبق في Backend/Config وليس Toggle تجاري في Dashboard.

السبب: تغيير Model قد يغير:

behavior
cost
context characteristics
structured output behavior
latency

ولذلك لا نجعله زرًا تشغيليًا بسيطًا.

---

87. Provider Switching

نفس القاعدة.

مدير النظام يستطيع:

View active provider
View health
Run health check
View failures
Disable AI Runtime
Enable AI Runtime

لكن لا يستطيع من Dashboard تغيير:

Gemini → Provider X

مباشرة.

Provider selection جزء من Runtime Configuration، وليس Merchant-facing configuration.

---

88. OpenAI-Compatible Adapter

وجود OpenAI-compatible client في البنية لا يعني أنه Provider إنتاجي فعال.

لذلك:

Gemini
    = ACTIVE PRODUCTION AI PROVIDER

بينما:

OpenAI-Compatible
    = AVAILABLE PROVIDER ADAPTER
    = NOT ACTIVE PRODUCTION PROVIDER

ولا تظهره Dashboard على أنه Active.

---

89. Channel Operations

مدير النظام يحصل على:

/admin/channels

وتعرض:

All Channel Connections

على مستوى المنصة.

---

90. Channel Provider الحالي

مزود Transport المعتمد:

SocialAPI

والقنوات:

Facebook
Instagram
WhatsApp

الـbaseline الحالي يحدد SocialAPI كمزود نقل القنوات وليس كمخزن Domain.

---

91. Channel Connection Status

الـChannelConnection يستخدم الحالات الموجودة فعليًا في schema الحالية:

pending
active
disconnected
failed
reconnect_required
archived

وهذا مثبت في قاعدة البيانات الحالية.

ولا ننشئ Status جديدًا لمجرد Dashboard.

---

92. Channel Health

نضيف مفهومًا منفصلًا عن Lifecycle Status:

HEALTHY
DEGRADED
DOWN
UNKNOWN

لأن:

status = active

لا يعني أن القناة تعمل الآن.

مثلاً:

Connection Status = active
Health = DOWN

يعني أن الاتصال ما زال مسجلًا كاتصال صالح، لكن التشغيل الحالي فيه مشكلة.

---

93. Channel Operations View

لكل قناة يعرض Platform Admin:

business_id
connection_id
channel
provider
status
health
provider_account_ref
provider_connection_ref
last_health_check_at
last_success
last_failure
failure_code

ولا يعرض:

access_token
secret
customer messages
conversation body
merchant catalog

---

94. Channel Health Check

POST /api/v1/platform/channels/{connection_id}/health-check

يقوم بفحص الاتصال مع Provider وفق الـAdapter.

لا يعدل Merchant Content.

---

95. Channel Reconnect

مدير النظام لا ينفذ OAuth نيابة عن التاجر.

لا يستطيع إنشاء credential باسم التاجر.

إذا كانت القناة:

reconnect_required

يعرض النظام:

Reconnect Required

ويمكن Platform Support بدء:

Reconnect Request

لكن authorization الفعلي يبقى ضمن Merchant Connection flow.

---

96. Channel Disconnect

مدير النظام لا يستخدم Disconnect كأداة يومية لإدارة التجار.

الفصل الطبيعي يتم من:

Merchant Owner/Admin

أما Platform Admin فله مسار إيقاف تشغيلي فقط عند الحاجة الأمنية/التشغيلية، ويسجل كـPlatform Audit.

ولا نستخدم:

DELETE Channel Connection

لإخفاء المشكلة.

---

97. Provider Operations

يوجد قسم:

/admin/providers

يعرض مزودي النظام الخارجيين.

في V1:

AI Provider
    └── Google Gemini

Channel Provider
    └── SocialAPI

ولا نعتبر:

Chatwoot

Provider Runtime؛ الـverified baseline الحالي أغلق Chatwoot-free boundary نهائيًا.

---

98. Provider Health

لكل Provider:

provider
category
enabled
health
last_check
last_success
last_failure
failure_rate

Category:

AI
CHANNEL_TRANSPORT

---

99. Provider Failure Isolation

إذا:

SocialAPI = DOWN

فلا يعني:

AI = DOWN

وإذا:

Gemini = DOWN

فلا يعني:

Facebook Transport = DOWN

كل Provider له Health مستقل.

---

100. Platform Operations Overview

صفحة:

/admin/operations

تعرض:

AI
├── Runtime
├── Provider
├── Model
└── Health

Channels
├── Active Connections
├── Failed Connections
├── Reconnect Required
└── Channel Health

Providers
├── Gemini
└── SocialAPI

---

101. Operational Alerts

Platform Operations يعرض حالات تحتاج تدخلًا:

AI Provider Down
AI Runtime Disabled
Channel Failed
Channel Reconnect Required
Provider Degraded
Repeated Delivery Failures
Repeated AI Failures

هذه Alerts تشغيلية.

ليست Notifications للتاجر.

---

102. Operational Event

كل تغيير إداري:

ai.disabled
ai.enabled
provider.health_checked
channel.health_checked
platform.channel_action

يسجل في:

platform_audit_events

---

103. لا يوجد Auto-Healing غير محسوب

لا يقوم Platform Dashboard تلقائيًا بـ:

random reconnect
random provider switch
random credential refresh
random channel deletion

الـWorkers والـAdapters مسؤولة عن Retry وفق عقود التشغيل.

Platform Admin يرى النتيجة ويتدخل عندما تتطلب الحالة قرارًا إداريًا.

---

104. Provider Secrets Boundary

الأسرار:

Gemini API Key
SocialAPI Secret
Signing Keys
Database Credentials

تبقى خارج Platform UI.

Dashboard يرى:

configured = true

ولا يرى السر نفسه.

---

105. AI + Subscription Boundary

AI Management وSubscription Management لا يخلطان.

Subscription تحدد:

Merchant Entitlement

AI Operations تراقب:

Platform Runtime
Provider Health
Operational Usage

مثال:

Merchant AI allowance exhausted

هذه مشكلة Entitlement.

بينما:

Gemini unavailable

هذه مشكلة Provider.

لا نسجل الاثنين تحت Status واحد.

---

106. Channel + Business Boundary

إذا:

Business = ACTIVE
Channel = FAILED

الحساب نفسه ليس Suspended.

وإذا:

Business = SUSPENDED
Channel = ACTIVE

فالقناة ما زالت مرتبطة تقنيًا، لكن Business restrictions تمنع العمليات التجارية حسب Business lifecycle.

لا نغير Connection status فقط لأن Business status تغير.

---

107. Platform Operations APIs — Final

AI

GET  /api/v1/platform/ai
GET  /api/v1/platform/ai/providers
GET  /api/v1/platform/ai/providers/{provider_id}

POST /api/v1/platform/ai/health-check
POST /api/v1/platform/ai/disable
POST /api/v1/platform/ai/enable

Channels

GET  /api/v1/platform/channels
GET  /api/v1/platform/channels/{connection_id}

POST /api/v1/platform/channels/{connection_id}/health-check

Providers

GET /api/v1/platform/providers
GET /api/v1/platform/providers/{provider_id}
POST /api/v1/platform/providers/{provider_id}/health-check

ولا نضيف API عامة لتغيير Provider أو Model من Dashboard.

---

108. Platform Operations UI

الـSidebar النهائي يصبح:

/admin
│
├── Overview
├── Businesses
├── Plans
├── Subscriptions
├── Operations
│   ├── AI
│   ├── Channels
│   └── Providers
├── Support
└── Audit

وهذا الآن هو الشكل الذي أعتمده لمدير النظام.

---

109. Contract Closure

أصبح Platform Administration كاملًا:

[✓] Platform Identity
[✓] Bootstrap
[✓] Authentication
[✓] Authorization
[✓] Business Management
[✓] Business Lifecycle
[✓] Plans
[✓] Subscriptions
[✓] Payments
[✓] Entitlements
[✓] Support
[✓] AI Operations
[✓] AI Provider Monitoring
[✓] AI Runtime Kill Switch
[✓] Channel Operations
[✓] Channel Health
[✓] Provider Monitoring
[✓] Platform Audit
[✓] Tenant Boundary
[✓] Secrets Boundary
[✓] Error Contract
[✓] Pagination
[✓] Idempotency

FINAL STATE

Platform Administration
        │
        ├── Governance
        ├── Commercial
        ├── Support
        └── Operations
             ├── AI
             ├── Channels
             └── Providers

هذا هو العقد الذي نعتمده.

والقرار المهم جدًا هنا:

Platform Admin
    ↓
يراقب ويُدير Platform Operations

لكن:

Platform Admin
    ✕
لا يصبح Merchant Superuser

وبالنسبة للـruntime الحالي:

AI Provider
= Google Gemini

Channel Transport Provider
= SocialAPI

Chatwoot
= ليس جزءًا من Runtime الحالي

وكل هذا يطابق الـverified backend baseline الأحدث للمشروع.