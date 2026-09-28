
تم. نغلق موضوع Catalog AI Projection الآن ولا نطيل فيه.

القرار النهائي — Catalog AI Projection v1

الـProjection هو Read Model للـAI فقط، ومبني 100% على Catalog Contract الذي أعطيتني إياه.

1. البنية

Catalog AI Projection
│
├── attribute_schemas[]
│     ├── id
│     ├── name
│     ├── version
│     └── definitions[]
│           ├── id
│           ├── schema_id
│           ├── attribute_key
│           ├── label
│           ├── data_type
│           ├── is_required
│           ├── is_searchable
│           ├── validation_rules
│           └── display_order
│
└── items[]
      ├── id
      ├── catalog_id
      ├── attribute_schema_id
      ├── attribute_schema_version
      ├── item_type
      ├── name
      ├── short_description
      ├── long_description
      ├── status
      ├── pricing_mode
      ├── availability_mode
      ├── fulfillment_mode
      ├── requires_confirmation
      ├── attributes
      │
      ├── variants[]
      │     ├── id
      │     ├── name
      │     ├── attributes
      │     └── status
      │
      └── offers[]
            ├── id
            ├── variant_id
            ├── name
            ├── pricing_mode
            ├── amount
            ├── currency
            ├── pricing_unit
            ├── price_source
            ├── price_verification_status
            ├── price_checked_at
            ├── availability_mode
            ├── availability_status
            ├── availability_source
            ├── availability_checked_at
            ├── availability_valid_until
            ├── availability_evidence_ref
            ├── fulfillment_mode
            ├── validity_from
            ├── validity_until
            └── status

2. قاعدة الـSchemas

نرسل فقط AttributeSchemas المستخدمة فعليًا بواسطة الـItems الموجودة في الـProjection.

لا نرسل Schemas غير مستخدمة.

3. لا نكرر الـSchema مع كل Item

الـSchema موجودة مرة واحدة في:

attribute_schemas[]

والـItem يشير إليها بواسطة:

attribute_schema_id
attribute_schema_version

4. الـBatches

إذا أصبح الكتالوج أكبر من قدرة الـContext:

Catalog
   ↓
Projection
   ↓
Batch 1
Batch 2
Batch 3
...

كل Batch يحتوي Items الخاصة به، وتكون الـSchemas المطلوبة لهذه الـItems متاحة للـAI.

لا نضع نظامًا معقدًا لإدارة الـSchemas.

5. ماذا لا يدخل؟

لا يدخل:

business_id
SQL
database metadata
created_at
updated_at

ولا يدخل أي:

search query
semantic search
matching logic
ranking logic

6. المسؤوليات

Mujeeb
  ↓
يقرأ Catalog الحقيقي
  ↓
يبني Projection
  ↓
يضمن Tenant Isolation
  ↓
يرسل البيانات

Gemini
  ↓
يفهم البيانات
  ↓
يفهم العميل
  ↓
يستدل
  ↓
يقارن
  ↓
يقرر ما يناسب الطلب
  ↓
يصيغ النتيجة

هذا هو Catalog AI Projection v1. مغلق.

ولا نحتاج نقاشًا إضافيًا حوله الآن.

التالي مباشرة

ننتقل إلى ② Catalog Evaluation / Batching:

> كيف نضمن أن Gemini يفحص كل الـCatalog Items المطلوبة، وكيف نحسب الـTokens ونقسم 1000+ منتج عند الحاجة، بدون أن نضع منطق مطابقة المنتجات داخل Mujeeb.



هذا هو الجزء الذي سيحدد فعليًا طريقة تشغيل الذكاء الاصطناعي في Mujeeb 24.