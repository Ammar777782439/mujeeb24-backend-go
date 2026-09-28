


 نغلق ② Catalog Evaluation + Batching الآن كقرار معماري نهائي.

والأهم: سأفصل بين ما تضمنه Google API فعليًا وبين ما نضمنه نحن في Mujeeb. Google توفّر function calling وإرجاع function_call/function_result، وتوفر count_tokens لمعرفة حجم الإدخال؛ لكنها لا توثّق ضمانًا يقول إن النموذج سيفحص كل عنصر في قائمة طويلة عنصرًا عنصرًا. 

القرار النهائي

1. من يضمن ماذا؟

Mujeeb
  ├── يحدد نطاق Catalog المسموح للتاجر
  ├── يبني Catalog Projection
  ├── يقسمه إلى Batches حسب TOKENS
  ├── يضمن أن كل Item في النطاق أُرسل إلى Gemini
  ├── يتتبع الـBatches التي اكتملت
  └── يجمع نتائج التقييم

Gemini
  ├── يفهم رسالة العميل
  ├── يفهم بيانات كل Batch
  ├── يستدل ويقارن
  ├── يحدد المرشحين
  └── يشرح لماذا دخل Item ضمن المرشحين

وهذا متوافق مع قرارنا السابق أن Mujeeb لا يحتوي semantic search أو product-matching logic؛ الوثائق الداخلية نفسها تقر أن نطاق الكتالوج المتاح للـAI يجب أن يكون كاملًا، وأن الـAI هو الذي يقرر متى يحتاج بيانات إضافية. 


---

2. لا يوجد عدد ثابت للمنتجات في الـBatch

لن نقول:

100 products = batch

ولا:

50 products = batch

لأن حجم المنتج ليس ثابتًا.

بدل ذلك:

Projection
    ↓
Serialize
    ↓
count_tokens
    ↓
هل يدخل ضمن حد الإدخال؟
    ├── نعم → Batch
    └── لا → split

Google توفر count_tokens لهذا الغرض تحديدًا، وحدود الـContext تختلف حسب النموذج، ويمكن معرفة الحد برمجيًا من معلومات النموذج
نعرف من خلاله كم الحد المسموح جوجل بتوفره لازم نطلع علي التوثيق 
إذن حجم الـBatch = Token-based، وليس Item-count-based.


---

3. كل Item يدخل في التقييم مرة واحدة على الأقل

مثال:

Catalog = 1000 Items

Batch 001 → Items 1..180
Batch 002 → Items 181..355
Batch 003 → Items 356..520
...
Batch N   → آخر Items

الـController يعرف مسبقًا:

Total items = 1000

ثم يحافظ على:

Sent = 1000
Completed = 1000

ولا يسمح بانتهاء عملية التقييم قبل اكتمال جميع الـBatches المطلوبة.

هذه هي ضمانة Mujeeb.

لكن TRUTHMODE مهم هنا:

> نستطيع أن نضمن أن كل Item أُرسل إلى Gemini ضمن Batch.
لا نستطيع أن نزعم أن Google تضمن أن النموذج داخليًا "ركّز معنويًا" على كل عنصر؛ لا يوجد في توثيق Google ضمان بهذا الشكل.




---

4. الـSchema مع الـBatch

لا نعيد كل AttributeSchemas الموجودة لدى التاجر.

لكل Batch:

Items في Batch
      ↓
اجمع الـSchemas التي تستخدمها هذه Items
      ↓
attribute_schemas[]
      +
items[]

وبذلك Gemini يملك:

AttributeDefinition
      ↓
attribute_key
      ↓
attributes

وكل Batch مكتمل بذاته من ناحية فهم الخصائص التي يستخدمها.


---

5. ماذا يرجع Gemini من كل Batch؟

لا نحتاج منه إعادة الـ100 أو الـ200 Item كاملة.

نحتاج نتيجة تقييم فقط.

الشكل المنطقي:

{
  "candidates": [
    {
      "item_id": "UUID",
      "variant_ids": ["UUID"],
      "offer_ids": ["UUID"],
      "reason": "..."
    }
  ]
}

هذه ليست Google Schema؛ هذه Mujeeb Contract خاص بنا.

Mujeeb يعرف أصلًا Items التي أرسلها في Batch، لذلك لا نعتمد على Gemini ليخبرنا هل أرسلنا كل العناصر. الـController هو صاحب الـcoverage.


---

6. بعد انتهاء جميع الـBatches

نصبح أمام:

1000 Items
   ↓
Gemini Batch Evaluation
   ↓
Candidate Results
   ↓
Final Gemini Evaluation
   ↓
Final Response

الـFinal Gemini لا يحتاج أن يرى الـ1000 منتج مرة أخرى.

يرى:

Customer Message
+
Conversation Context
+
Candidate Results
+
الدليل التجاري المرتبط بالمرشحين

ثم يقوم بالقرار النهائي وصياغة الرد.


---

7. لو كان عدد الـCandidates كبيرًا

نستخدم نفس قاعدة الـToken Budget.

لا نضع:

maximum candidates = 20

ولا:

maximum batches = 6

الوثيقة الحالية  نفسها تقفل عدم وضع حد جولات اعتباطي، وتربط الاستمرار بكون العمل مفيدًا وحدود الاقتصاد/الأمان/مزود الخدمة. 

إذن:

Token limit
    ↓
إذا دخلت النتائج → Final Gemini
إذا لم تدخل → تقسيم النتائج


---

8. لا نستخدم previous_interaction_id بين كل Batch

هذه نقطة مهمة.

Google Interactions API يدعم previous_interaction_id لمواصلة المحادثة وحفظ تاريخها على الخادم. كما أن الأدوات وsystem_instruction وgeneration_config مرتبطة بالتفاعل الحالي ويجب إعادة تحديدها عند الحاجة. 

لكن لا نربط Batch 1 → Batch 2 → Batch 3 كسلسلة محادثة ضخمة.

لأننا لا نريد أن تصبح:

Batch 1
 + Batch 2
 + Batch 3
 + ...

كلها تاريخًا متراكمًا لكل طلب لاحق.

الأفضل:

Conversation Context
       │
       ├── Batch 1 → Gemini
       ├── Batch 2 → Gemini
       ├── Batch 3 → Gemini
       └── Batch N → Gemini
                    ↓
              Final Gemini

أي أن كل Batch مستقل في عملية تقييم الكتالوج، ثم توجد مرحلة Final مستقلة.


---

9. العقد النهائي

Customer Message
      +
Conversation Context
      +
AI Instructions
      ↓
Gemini
      ↓
هل يحتاج Catalog؟
      ↓
Mujeeb يبني Projection
      ↓
Token Counting
      ↓
Batching
      ↓
┌───────────────┐
│ Batch 1       │ → Gemini
│ Batch 2       │ → Gemini
│ Batch 3       │ → Gemini
│ ...           │
│ Batch N       │ → Gemini
└───────────────┘
      ↓
Candidate Results
      ↓
Final Gemini
      ↓
AI Proposal / Response
      ↓
Mujeeb Validation
      ↓
Send / Handoff / Action

ما أغلقناه

مغلق نهائيًا:

لا search_catalog(query).

لا Semantic Search داخل Mujeeb.

لا Product Matching داخل Mujeeb.

لا عدد ثابت للـItems في Batch.

لا عدد جولات ثابت مثل 6.

الـBatching بالـTokens.

الـController يضمن أن كل Item في النطاق دخل التقييم.

Gemini مسؤول عن الفهم والمقارنة والاستدلال.

AttributeSchema + AttributeDefinition جزء من السياق.

كل Batch يحصل على الـSchemas التي يحتاجها.

النتائج تُجمع ثم تحدث Final Evaluation.

Mujeeb يبقى مصدر الحقيقة والـtenant isolation والـvalidation والتنفيذ. 


بهذا انتهى موضوع Evaluation + Batching. لا نحتاج نرجع له.

والخطوة التالية المنطقية هي ③ Conversation Context Contract؛ لأنها الشيء الذي سيجعل Gemini يفهم: «طيب الأسود؟»، «والثاني؟»، «وكم؟» بدون فقدان سياق العميل.