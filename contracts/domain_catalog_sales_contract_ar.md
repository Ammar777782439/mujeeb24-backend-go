# Domain Contract — Catalog وSales

## قرار الملكية

هذا العقد يجعل مجيب 24 منصة متعددة القطاعات دون تحويلها إلى خمسة أنظمة منفصلة.

```text
Business
  → Catalog
      → CatalogItem
          → Offer
              → Variant / Attributes
                  → Lead
                      → CommercialTransaction
```

`Product` ليس root model. هو حالة من حالات `CatalogItem` فقط.

## 1. catalog/

### 1.1 Catalog Aggregate

Catalog هو مساحة تنظيمية داخل Business:

```text
Catalog
├── id
├── business_id
├── name
├── status
├── visibility
├── schema_version
├── created_at
└── updated_at
```

القيم:

```text
status: draft | active | archived
visibility: private | customer_visible
```

Catalog لا يحتوي في الذاكرة قائمة كل Items. `CatalogItem` Aggregate مستقل يرتبط بـ`catalog_id`.

### 1.2 CatalogItem Aggregate

يمثل ما يقدمه التاجر، سواء كان سلعة أو خدمة أو رحلة أو موعدًا.

```text
CatalogItem
├── id
├── business_id
├── catalog_id
├── item_type
├── name
├── short_description
├── long_description
├── status
├── category_id
├── media_references
├── attribute_schema_id
├── fulfillment_mode
├── pricing_mode
├── availability_mode
├── requires_customer_confirmation
├── requires_human_review
├── created_at
└── updated_at
```

`item_type`:

```text
physical_good
service
ticket
accommodation
appointment
package
subscription
quote_based
```

`item_type` يصف طبيعة العرض، لكنه لا يحتوي workflow القطاع. Service لصيانة جوال وService لاستشارة قانونية يشتركان في النوع، لكن يختلفان في schema وtransaction workflow.

### 1.3 CatalogItem Invariants

لا يمكن نشر CatalogItem بلا name وitem_type وcatalog صالح. لا نطلب `price` أو `stock` أو `sku` أو `variant` لكل item.

لا يصبح item `active` إذا كان schema المطلوب غير صالح أو Offer المنشور لا يملك Pricing/Availability contract متوافقًا مع نوعه.

لا نحذف item إذا ارتبط بمعاملة؛ نستخدم `archived`. المعاملة القديمة تحتفظ Snapshot ولا تتأثر بتغيير item الحالي.

### 1.4 Offer Aggregate

Offer يجيب: كيف يباع CatalogItem الآن؟

```text
Offer
├── id
├── business_id
├── catalog_item_id
├── name
├── status
├── pricing
├── availability
├── fulfillment
├── valid_from
├── valid_until
├── schema_version
├── created_at
└── updated_at
```

`status`:

```text
draft | active | paused | expired | archived
```

CatalogItem ثابت نسبيًا، بينما Offer يمكن أن يتغير مع الموسم أو المورد أو الحملة.

### 1.5 Pricing Contract

```text
PricingMode
├── fixed
├── starting_from
├── per_unit
├── per_person
├── per_day
├── quote_required
└── dynamic
```

Pricing Value:

```text
Pricing
├── mode
├── amount: optional Money
├── min_amount: optional Money
├── max_amount: optional Money
├── currency: optional
├── source: merchant | supplier | integration | manual
├── status: verified | stale | unknown | requires_check
├── valid_until: optional
└── last_verified_at: optional
```

القواعد:

```text
fixed + verified    → يمكن إظهار سعر مؤكد ضمن صلاحيته
starting_from       → لا يحول إلى سعر نهائي
quote_required      → يحتاج جمع بيانات/مراجعة
 dynamic            → يحتاج مصدر تسعير/تحقق
unknown             → لا يجوز عرضه كرقم مؤكد
```

AI لا يخلق `amount` إذا كان PricingMode لا يسمح بذلك أو إذا لم تكن الحالة verified.

### 1.6 Availability Contract

`availability_mode` يصف طريقة التحقق:

```text
stock
schedule
supplier_check
always_available
unknown
```

`availability_status` يصف النتيجة الحالية:

```text
available
unavailable
unknown
requires_check
stale
```

```text
Availability
├── mode
├── status
├── checked_at
├── valid_until
├── source
└── evidence_reference
```

قاعدة غير قابلة للكسر:

```text
unknown ≠ available
stale ≠ confirmed
requires_check ≠ confirmed
```

### 1.7 Fulfillment Contract

```text
FulfillmentMode
├── delivery
├── pickup
├── digital
├── appointment
├── travel
└── manual
```

لا تنفذ Fulfillment نفسها داخل CatalogItem؛ هي تصف ما يحتاجه transaction بعد التأكيد.

### 1.8 Variant

Variant اختياري. لا نجبر الخدمة أو الاستشارة على Variants.

```text
Variant
├── id
├── catalog_item_id
├── name
├── code: optional
├── attribute_values
├── pricing_override: optional
├── availability_override: optional
├── status
└── created_at
```

الإلكترونيات يمكن أن تستخدم Variant للون/السعة/الذاكرة. السفر قد يستخدم Variant لدرجة السفر أو نوع الباقة. الخدمة قد لا تستخدمه.

### 1.9 AttributeSchema وAttributeDefinition

لا نسمح بـfree-form JSON بلا تعريف.

```text
AttributeSchema
├── id
├── business_id
├── name
├── subject_type
├── version
├── status
├── created_at
└── updated_at
```

```text
AttributeDefinition
├── id
├── schema_id
├── key
├── label
├── data_type
├── required
├── searchable
├── validation_rules
├── display_order
└── version
```

`data_type`:

```text
text | number | boolean | date | datetime | select |
multi_select | location | money
```

الـvalidation rules تحدد enum values أو min/max أو regex أو currency أو timezone. AI لا ينشئ AttributeDefinition تلقائيًا أثناء محادثة العميل؛ التاجر أو Template هو من يعرّف schema.

### 1.10 Schema Versioning

عند إضافة Branch أو Doctor أو Baggage إلى schema، ننشئ version جديدة. السجلات القديمة تحتفظ بالإصدار الذي أنشئت به.

```text
Schema v1 → Schema v2 → Schema v3
```

لا نعيد تفسير Transaction قديمة باستخدام schema حديثة.

## 2. sales/

### 2.1 Lead Aggregate

Lead هو احتمال تجاري ناتج عن Conversation، وليس كل Message.

```text
Lead
├── id
├── business_id
├── customer_id
├── source_conversation_reference_id
├── source_channel
├── intent_reference
├── qualification_state
├── score
├── assigned_ownership_reference
├── next_action_at
├── lost_reason
├── created_at
└── updated_at
```

`qualification_state`:

```text
new | qualified | working | converted | lost | disqualified
```

Lead يمكن أن يتحول إلى أكثر من transaction محتملة، لكن لا ننشئ Leads مكررة عند إعادة معالجة نفس InboundEvent.

### 2.2 CommercialTransaction Aggregate

يمثل العملية التجارية العامة:

```text
CommercialTransaction
├── id
├── business_id
├── customer_id
├── lead_id: optional
├── source_conversation_reference_id
├── transaction_type
├── status
├── currency
├── total_amount: optional
├── requires_human_review
├── schema_version
├── created_at
└── updated_at
```

`transaction_type`:

```text
order
booking
appointment
service_request
reservation
quote
subscription
```

لا نضع `doctor_id` و`flight_number` و`room_number` و`table_number` داخل هذا الكيان العام.

### 2.3 Transaction Details

لكل نوع Details/aggregate فرعي مستقل:

```text
CommercialTransaction
├── OrderDetails
├── BookingDetails
├── AppointmentDetails
├── ServiceRequestDetails
├── ReservationDetails
├── QuoteDetails
└── SubscriptionDetails
```

التفاصيل تشترك في `transaction_id` وschema version، لكنها تملك invariants مختلفة.

### 2.4 Transaction Lines وSnapshot

أي transaction مرتبطة بعرض تحفظ snapshot عند الإنشاء:

```text
TransactionLine
├── id
├── transaction_id
├── catalog_item_id
├── offer_id
├── variant_id: optional
├── item_name_snapshot
├── selected_attributes_snapshot
├── pricing_snapshot
├── availability_snapshot
├── quantity: optional
└── fulfillment_snapshot
```

تغيير CatalogItem أو Offer بعد ذلك لا يغير ما اتفق عليه العميل في transaction التاريخية.

### 2.5 Common Lifecycle

```text
identified
→ needs_information
→ qualified
→ quote_or_availability_check
→ awaiting_customer_confirmation
→ confirmed
→ in_fulfillment
→ completed
```

الحالات الجانبية:

```text
cancelled | expired | lost | rejected
```

ليس مطلوبًا أن تمر كل transaction بكل الحالات.

### 2.6 Type-specific Lifecycles

Order:

```text
pending → confirmed → preparing → out_for_delivery → delivered
pending → cancelled
```

Booking:

```text
draft → needs_information → availability_check → quoted
→ awaiting_confirmation → confirmed → fulfilled
```

Appointment:

```text
requested → needs_information → slot_check → scheduled
→ attended | no_show | cancelled
```

Service Request:

```text
received → triaged → quoted → approved → in_progress
→ completed | rejected | cancelled
```

Quote:

```text
draft → issued → viewed → accepted | declined | expired
```

Subscription:

```text
trial → active → past_due → suspended → cancelled | expired
```

### 2.7 Invariants

لا تصبح Transaction `confirmed` إذا كانت Pricing أو Availability `unknown` عندما يتطلب النوع تأكيدًا. لا تتحول Quote إلى accepted دون customer confirmation الموثقة. لا ينشئ AI Order مؤكدًا من رسالة غامضة؛ أقصى ما ينشئه هو Draft إذا سمحت Policy.

لا يصبح Order Delivered لأن AI قال ذلك؛ Delivery status يحتاج حدث fulfillment أو تحديثًا بشريًا/تكاملًا موثوقًا.

### 2.8 Sales Domain Events

```text
LeadCreated
LeadQualified
LeadConverted
LeadLost
TransactionDraftCreated
TransactionInformationRequested
TransactionQuoted
TransactionAwaitingConfirmation
TransactionConfirmed
TransactionCancelled
TransactionExpired
TransactionFulfillmentStarted
TransactionCompleted
```

كل حدث يذكر aggregate ID وbusiness ID وschema version وoccurred_at، ولا يحتوي raw provider payload أو secrets.

## 3. Vertical Templates

Template إعداد، وليس Engine جديدًا:

```text
Retail/Electronics:
item_type=physical_good
pricing=fixed/per_unit
availability=stock
fulfillment=delivery/pickup
default_transaction=order

Travel:
item_type=ticket/package
pricing=starting_from/dynamic/quote_required
availability=supplier_check
fulfillment=travel
default_transaction=booking/quote
```

يمكن لمكتب السفر إضافة Visa Service، ويمكن لمتجر الإلكترونيات إضافة Repair Service. `vertical_type` يعطي defaults ولا يحبس Business داخل قطاع واحد.

## 4. معايير إغلاق catalog وsales

نعتبر العقد مغلقًا عندما تثبت القرارات التالية:

1. `CatalogItem` و`Offer` ليسا كيانًا واحدًا.
2. السعر والتوفر لهما mode وstatus وsource وvalidity.
3. Variants اختيارية.
4. Attributes typed ومتحققة ومؤرخة بإصدار schema.
5. Transaction تحفظ snapshots.
6. Order ليس transaction universal.
7. كل Transaction type يملك details وlifecycle مخصصًا.
8. UNKNOWN لا يساوي AVAILABLE أو CONFIRMED.
9. AI ينشئ Draft أو يطلب معلومات، ولا يؤكد دون evidence وpolicy.
10. Template إعداد قابل للتخصيص، وليس codebase مستقلًا لكل قطاع.

بعد هذا الجزء ننتقل إلى `domain/ai` و`domain/audit` لتحديد كيف يستخدم AI هذه العقود دون أن يصبح مالكًا للقرار التجاري.
