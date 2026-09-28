# مراجعة المدير — Domain Contract 06: Sales

## الحكم التنفيذي

التحليل المرفق **قوي ومتوافق مع Universal Catalog وIdentity وAI Contracts**. وهو يثبت أن مجيب 24 ليس متجرًا فقط؛ بل يملك Sales Domain عامًا يحول إشارات الشراء إلى فرص ثم إلى معاملات من أنواع متعددة.

```text
Customer
  ↓
Conversation / Buying Signal
  ↓
Lead
  ↓
CommercialTransaction
      ├── Order
      ├── Booking
      ├── Appointment
      ├── ServiceRequest
      ├── Reservation
      ├── Quote
      └── Subscription
```

أعتمد الاتجاه العام بعد تصحيحات محددة تمنع تضخم الحالة العامة أو الخلط بين Lead وTransaction وConversation.

## ما نعتمده

| القرار | الحكم |
|---|---|
| Lead فرصة تجارية وليس Conversation | معتمد |
| Customer يمكن أن يملك عدة Leads | معتمد |
| LeadScore قابل للتفسير وليس رقم AI فقط | معتمد |
| Qualification تعتمد على Signals وPolicy وContext | معتمد |
| Lead Attribution تحفظ Conversation/Channel/Catalog context | معتمد |
| CommercialTransaction تجريد عام | معتمد |
| Order ليس نوع المعاملة الوحيد | معتمد |
| Customer يمكن أن يملك عدة Transactions | معتمد |
| Transaction يمكن أن تمتد عبر أكثر من Conversation | معتمد |
| Transaction لا تستخدم inheritance تقنيًا بين الأنواع | معتمد |
| Price/Offer/Variant snapshots | معتمد |
| Payment وDelivery تفاصيل اختيارية وليست عامة لكل Transaction | معتمد |
| Confirmation وHuman Review منفصلان | معتمد |
| Sales لا يتصل مباشرة بـAvailability Provider أو Payment Gateway | معتمد |

## التصحيحات الإلزامية

### 1. Lead Source يجب أن ينقسم إلى Attribution وCreation Actor

لا نضع قيمة واحدة تخبرنا بكل شيء. نحتاج:

```text
LeadAttribution
├── source_conversation_reference?
├── source_channel?
├── source_interaction_reference?
├── catalog_item_reference?
├── offer_reference?
├── campaign_reference?
└── captured_at
```

و:

```text
created_by: ai | human | automation | system
qualified_by: ai | human | automation | system | none
```

بهذا نعرف الفرق بين «اكتشف AI إشارة الشراء» و«اعتمد الموظف إنشاء Lead».

### 2. Lead يحتاج Customer، لكن ليس Conversation واحدة

بعد Identity Resolution، يجب أن يملك Lead `customer_id` داخل Business. أما المصدر فيمكن أن يكون Conversation واحدة أو أكثر:

```text
Lead
  ├── customer_id
  ├── primary_source_conversation_id?
  └── attribution_records[]
```

لا نربط Lead بحوار واحد طوال عمره؛ قد يبدأ في Instagram ويُستكمل في WhatsApp.

### 3. Lead Status وBuying Signal ليسا الشيء نفسه

نعتمد lifecycle مبسطًا:

```text
new → interested → qualified → won
new/interested/qualified → lost
```

لكن `interested` لا ينتج من سؤال سعر عادي فقط. يلزم Buying Signal أقوى مثل «أريد واحدًا» أو «احجزه لي» أو «كيف أطلب؟».

`won` نتيجة تجارية للفرصة، ولا ينشئ Transaction تلقائيًا. قد توجد Transaction pending وLead won، وفق Business Workflow.

### 4. LeadScore ليس مصدر الحقيقة

```text
LeadScore
├── value
├── band
├── factors[]
├── calculated_at
├── rule_version?
└── model_reference?
```

العوامل قابلة للتدقيق:

```text
high_purchase_intent
offering_identified
repeat_customer
quantity_provided
commercial_context_complete
human_confirmed
```

لا نسمح بقاعدة `score >= 80 → qualified` وحدها. الحالة تعتمد على Business Qualification Policy وContext وEvidence.

## CommercialTransaction

### 1. Transaction Aggregate

```text
CommercialTransaction
├── id
├── business_id
├── customer_id
├── lead_id?
├── transaction_type
├── state
├── source_conversation_reference?
├── currency?
├── total_amount?
├── confirmation?
├── human_review?
├── schema_version
├── created_at
└── updated_at
```

`source_conversation_reference` اختياري؛ لأن transaction قد ينشئها موظف أو API أو workflow لاحقًا، وقد تستمر عبر عدة محادثات.

`total_amount` اختياري في Draft/Quote Required/Dynamic، ولا يكون final إلا إذا توفرت Pricing Evidence صحيحة.

### 2. Transaction Types

```text
order
booking
appointment
service_request
reservation
quote
subscription
```

لا نضيف نوعًا جديدًا إلا إذا امتلك workflow وinvariants مختلفين فعليًا. ولا نستخدم `anything` كنوع هروب.

### 3. Transaction State

نعتمد حالات عامة قليلة:

```text
draft
needs_information
awaiting_confirmation
confirmed
in_fulfillment
completed
cancelled
expired
rejected
```

لا نضع كل حالات Order وBooking وAppointment في Enum واحد ضخم. كل نوع يستخدم subset وstate machine خاصة به.

`pending` غير محددة بما يكفي؛ نستخدم `draft` أو `needs_information` أو `awaiting_confirmation` بحسب الواقع.

### 4. Confirmation

```text
Confirmation
├── status: not_required | pending | confirmed | rejected
├── confirmed_by: customer | human_agent | system_policy
├── confirmed_at?
├── evidence_reference?
└── policy_version?
```

لا تتحول Transaction إلى `confirmed` لأن AI استنتج «أريد اثنين». يلزم confirmation صريح حيث تنص BusinessPolicy أو نوع العملية.

### 5. Human Review

```text
HumanReview
├── required
├── reason_codes[]
├── status: pending | approved | rejected | bypassed
├── reviewer_reference?
└── decided_at?
```

أمثلة:

```text
custom_quote
high_value_transaction
medical_sensitive_flow
refund_or_adjustment
unknown_availability
```

## النوع الأول: Order

Order هو specialization سلوكي وليس inheritance تقنيًا:

```text
OrderDetails
├── lines[]
├── delivery_details?
├── payment_details?
└── order_state
```

```text
OrderLine
├── catalog_item_reference
├── offer_reference
├── variant_reference?
├── item_name_snapshot
├── selected_attributes_snapshot
├── quantity
├── unit_price_snapshot
└── line_total_snapshot
```

Order lifecycle:

```text
draft → awaiting_confirmation → confirmed
→ preparing → out_for_delivery → delivered
```

أو:

```text
draft → cancelled
```

### DeliveryDetails

اختياري حسب Fulfillment:

```text
DeliveryDetails
├── address_snapshot
├── city
├── phone_snapshot
├── notes
└── delivery_fee_snapshot?
```

لا نضعه في كل Transaction.

### PaymentDetails

Mujeeb يسجل commercial payment intent/status ولا يصبح Payment Gateway:

```text
PaymentDetails
├── method: cash | bank_transfer | wallet | cod | other
├── amount_snapshot
├── status: unpaid | pending | paid | failed | refunded
└── reference?
```

التكامل مع بنك أو محفظة يبقى Port/Adapter.

## النوع الثاني: Booking

```text
BookingDetails
├── offering_reference
├── participants_snapshot
├── schedule_request
├── availability_reference?
├── price_snapshot?
└── confirmation
```

مكتب السفر قد يحتاج origin/destination/departure/return/passengers/cabin. هذه بيانات Booking/Attribute Schema، لا حقول Order عامة.

Booking lifecycle:

```text
draft → needs_information → availability_check
→ awaiting_confirmation → confirmed → fulfilled
```

لا يصبح Booking confirmed إذا كانت Availability المطلوبة unknown أو stale.

## النوع الثالث: Appointment

```text
AppointmentDetails
├── offering_reference
├── provider_reference?
├── location_reference?
├── requested_slot
├── confirmed_slot?
├── duration?
└── confirmation
```

لا يحجز AI موعدًا غير متاح. فحص slot يأتي من Availability Port/Application.

## النوع الرابع: ServiceRequest

ServiceRequest يبدأ بجمع المتطلبات وقد ينتج Quote أو Schedule أو Human Handoff:

```text
ServiceRequestDetails
├── offering_reference?
├── requirement_snapshot
├── inspection_reference?
├── quote_reference?
└── fulfillment_notes
```

لا نضع `service_request.go` كملف تنفيذ الآن إلا بعد وجود سلوك مستقل؛ لكن المفهوم مغلق داخل Transaction Type Contract.

## النوع الخامس: Quote

```text
QuoteDetails
├── lines_or_scope_snapshot
├── proposed_amount?
├── assumptions[]
├── validity
├── requires_human_review
└── confirmation
```

Quote lifecycle:

```text
draft → issued → accepted
issued → declined
issued → expired
```

لا يقبل Quote منتهي الصلاحية. و`quote_required` لا يملك سعرًا نهائيًا قبل إعداد العرض.

## Snapshots

عند إنشاء Transaction Line أو Details، نحفظ:

```text
CatalogItem reference + name snapshot
Offer reference + pricing snapshot
Variant reference + selected attributes snapshot
Availability evidence snapshot
Fulfillment snapshot
```

تغيير Catalog أو Offer لاحقًا لا يغيّر التاريخ التجاري. Snapshot ليس بديلًا عن Reference؛ نحتاج الاثنين للتدقيق.

## Invariants

- Lead وCustomer وTransaction وCatalog references داخل Business واحد.
- Customer يمكنه امتلاك عدة Leads وTransactions.
- Lead لا ينشئ Transaction تلقائيًا عند `won`.
- Transaction لا تعتمد على Conversation واحدة.
- Transaction type يحدد allowed states.
- Confirmation مطلوبة قبل التأكيد حيث تنص Policy.
- Availability وPricing evidence يجب أن تكون صالحة عند التأكيد.
- Quote المنتهي لا يقبل.
- Order لا يثبت Delivered عبر كلام AI.
- Payment method لا يعني أن الدفع تم.
- لا يصل Sales Domain مباشرة إلى SocialAPI أو Chatwoot أو Payment Provider.

## القرار

**أعتمد تحليل الذكاء الاصطناعي بعد هذه التصحيحات.** أصبح Sales Contract مغلقًا كتصميم، مع إبقاء ServiceRequest وReservation كأنواع مفاهيمية حتى يظهر لكل منها workflow مستقل، بدل إنشاء ملفات فارغة.

الآن أصبحت Domains التجارية الأساسية مغلقة:

```text
shared ✅
business ✅
channel ✅
identity ✅
communication ✅
catalog ✅
sales ✅
```

الخطوة التالية المنطقية ليست كتابة AI implementation؛ بل مراجعة `domain/ai` و`domain/audit` بشكل نهائي ثم إغلاق `application/ports`.
