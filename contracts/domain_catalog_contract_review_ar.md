# مراجعة المدير — Domain Contract 05: Catalog

## الحكم التنفيذي

التحليل المرفق **صحيح ومتوافق مع Universal Business Offering Contract**. وهو يغلق النموذج الذي سيجعل مجيب 24 منصة واحدة لمحلات الإلكترونيات ومكاتب السفر والعيادات والمطاعم والخدمات، دون إنشاء نظام منفصل لكل قطاع.

القرار المركزي:

```text
Business
  → Catalog
      → CatalogItem
          → Variant (optional)
          → Offer
              → Pricing
              → Availability
              → Fulfillment
```

`Product` ليس Domain root. هو Item Type محتمل داخل `CatalogItem`.

## ما نعتمده

| القرار | الحكم |
|---|---|
| Business يملك عدة Catalogs | معتمد |
| Catalog ليس Store أو Order أو Customer | معتمد |
| CatalogItem هو العرض الأساسي العام | معتمد |
| ItemType يصف طبيعة العرض ولا يحدد Workflow كاملًا | معتمد |
| Variant اختيارية | معتمد |
| Offer منفصل عن CatalogItem | معتمد |
| Pricing منفصل ولا يعتمد على float64 | معتمد |
| Availability ليست Stock فقط | معتمد |
| Fulfillment يصف طريقة التنفيذ الأساسية | معتمد |
| AttributeSchema بإصدارات واضحة | معتمد |
| Snapshot داخل Transaction | معتمد لاحقًا في Sales Contract |
| Vertical Templates إعدادات لا Agents أو Services مستقلة | معتمد |
| Catalog لا يعرف Chatwoot أو SocialAPI أو AI Provider | معتمد |

## التصحيحات الإلزامية

### 1. CatalogItem لا يحمل AttributeSchema كاملًا

يحتفظ CatalogItem بـ:

```text
attribute_schema_id
attribute_schema_version
```

أما Definitions والقواعد فتملكها `AttributeSchema`. عند تعديل schema، ننشئ version جديدة ولا نعيد تفسير البيانات القديمة.

### 2. Offer وVariant يحتاجان علاقة مضبوطة

`Variant` يصف اختلافًا في العنصر:

```text
256GB + Black
Economy + Airline X
2-hour service
```

`Offer` يصف حالة البيع التجارية الحالية:

```text
Offer
├── catalog_item_id
├── variant_id?
├── pricing
├── availability
├── fulfillment
└── validity
```

Offer يمكن أن يرتبط بـVariant، لكنه ليس Variant. يجب أن يملك Business وCatalogItem نفس `business_id`، ولا يجوز Offer أن يشير إلى Variant من Item أو Business مختلف.

### 3. Pricing لا يفترض Amount دائمًا

نستخدم:

```text
Pricing
├── mode
├── amount?
├── currency?
├── unit?
├── source
├── verification_status
├── valid_from?
├── valid_until?
└── checked_at?
```

الـ`unit` يكون typed مثل `each` أو `person` أو `day` أو `hour` أو `route`.

القواعد:

```text
fixed + verified        → amount مطلوب
per_unit/per_person     → amount مطلوب + unit مطلوب
starting_from           → amount يمثل بداية وليس نهائيًا
quote_required          → amount غير مطلوب
 dynamic                → amount اختياري ويحتاج مصدر تحقق
unknown                 → لا يجوز عرضه كرقم مؤكد
```

لا ينشئ AI سعرًا من النص، ولا يحول Starting From إلى Final Price.

### 4. Availability ليست حقل stock_quantity

```text
Availability
├── mode
├── status
├── source
├── checked_at
├── valid_until?
└── evidence_reference?
```

`stock_quantity` يمكن أن يظهر داخل Inventory Adapter عندما يكون mode هو Stock، لكنه ليس حقلًا عالميًا في CatalogItem.

الحالات:

```text
available
unavailable
unknown
requires_check
stale
```

القاعدة غير القابلة للكسر:

```text
unknown ≠ available
stale ≠ confirmed
requires_check ≠ confirmed
```

### 5. Fulfillment هو Default لا وعد نهائي

CatalogItem أو Offer يقدمان `fulfillment_mode` الافتراضي:

```text
delivery | pickup | digital | appointment | travel | manual
```

لكن Transaction قد تحتاج تفاصيل إضافية أو Override موثقًا. مثلًا، رحلة السفر تحتاج origin/destination/date، والخدمة تحتاج موعدًا، والإلكترونيات تحتاج عنوان تسليم.

### 6. Attribute Values يجب أن تكون Typed

لا نستخدم `map[string]any` كـDomain primitive. كل قيمة مرتبطة بـDefinition وschema version وdata type:

```text
text
number
boolean
date
datetime
select
multi_select
location
money
```

`RAM` و`Doctor` و`FlightNumber` ليست أنواعًا؛ هي keys/values داخل schema. Validation rules تحدد القيم المقبولة.

### 7. Media تبقى References

CatalogItem يحتفظ بـMedia References، لا ملفات ولا Storage SDK:

```text
MediaReference
├── id/reference
├── type
├── alt_text
├── sort_order
└── status
```

Storage Adapter يملك الملف، وDomain يملك reference وسياسة العرض فقط.

### 8. Category ليست Domain كبيرة في V1

يمكن أن يكون `category_id` اختياريًا، لكن لا نخلق Category tree معقدة قبل وجود Use Case حقيقي. في البداية يكفي Catalog وItem وoptional category reference، مع إمكانية إضافة Category Contract لاحقًا.

## العقد النهائي لـdomain/catalog

```text
internal/domain/catalog/
├── catalog.go
├── catalog_item.go
├── offer.go
├── variant.go
├── attribute_schema.go
├── pricing.go
├── availability.go
└── fulfillment.go
```

### Catalog Invariants

- Catalog ينتمي إلى Business واحد.
- CatalogItem ينتمي إلى Catalog واحد.
- CatalogItem وOffer وVariant لا تتجاوز Business boundary.
- Catalog وItem وOffer لا تُحذف فعليًا إذا دخلت معاملة تاريخية؛ تستخدم archive.
- نشر Catalog لا يضمن أن كل Item أو Offer active.
- Item يحتاج name وitem_type وschema صالح عند الحاجة.

### Offer Invariants

- Offer ينتمي إلى CatalogItem واحد.
- Offer المرتبط بـVariant يستخدم Variant من نفس Item وBusiness.
- Offer active يحتاج Pricing/Availability contract مناسبين لطبيعته.
- Offer المنتهي لا يستخدم في Transaction جديدة.
- تغيير Offer لا يغير Snapshot داخل Transaction قديمة.

### Schema Invariants

- Definition key فريد داخل Schema version.
- required وdata_type وvalidation_rules متسقة.
- لا نغير version منشورة in-place.
- قيم Item/Variant مرتبطة بالنسخة الصحيحة.

## علاقة Catalog مع AI

AI لا يبحث عن جدول Product وحسب. Application يبني Context من:

```text
Business Policy
→ relevant Catalog
→ CatalogItem
→ Variant
→ active Offer
→ Pricing Evidence
→ Availability Evidence
→ Fulfillment Requirements
```

مثال الإلكترونيات:

```text
"آيفون 15 أسود 256؟"
→ Item match
→ Variant attributes
→ active Offer
→ verified price
→ stock check
```

مثال السفر:

```text
"أريد صنعاء إلى القاهرة الخميس"
→ Travel Item/Offer
→ missing year/cabin/passenger data?
→ ask question
→ supplier availability/price check
→ Booking or Quote Draft
```

في الحالتين لا يعلن AI توفرًا أو سعرًا بلا Evidence.

## علاقة Catalog مع Lead وSales

Lead يحتفظ باهتمام تجاري Reference:

```text
catalog_item_id?
offer_id?
variant_id?
interest_attributes_snapshot?
```

لكن Lead ليس جزءًا من Catalog ownership. Transaction لاحقًا تحفظ Snapshot لـItem وOffer وVariant وAttributes وPrice وFulfillment.

## Vertical Templates

نستخدم Templates مبدئية في Application/Configuration:

```text
Electronics:
physical_good + stock + delivery/pickup

Travel:
ticket/package + supplier_check + travel + quote/booking

Clinic:
appointment + schedule + appointment

Restaurant:
physical_good/service + availability + pickup/delivery

Services:
service + schedule/manual + quote/service_request
```

هذه defaults ولا تمنع Business من امتلاك Catalog آخر أو Service إضافية.

## ما لا يدخل Catalog

```text
Customer
Lead
Conversation
Chatwoot
SocialAPI
AI Provider
Payment Gateway
Delivery Provider implementation
```

يمكن وجود references عند الحاجة، لكن الملكية والـworkflows تبقى في Domains/Application المناسبة.

## القرار

**أعتمد تحليل الذكاء الاصطناعي بعد هذه التصحيحات.** أصبح `domain/catalog` مغلقًا كعقد تصميم، ولم نكتب implementation بعد.

الخطوة التالية هي `domain/sales` لإغلاق Lead وCommercialTransaction وOrder وBooking وAppointment وQuote وService Request، مع الالتزام بأن Catalog لا يؤكد عملية البيع ولا يملك Transaction.
