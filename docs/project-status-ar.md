# حالة مشروع Mujeeb 24 Backend — مرجع التطوير الحالي

> **النطاق الزمني:** 2026-10-29. **مرجع حالة مسار Merchant Catalog AI في هذا الملف:** `fix/merchant-catalog-strict-wire-v3` / PR #18. هذه حالة **الفرع البرمجي** وليست إثباتًا لنشر `main` أو Render. حالة بقية الوحدات تُستخرج من كود HEAD وإعداد بيئة التشغيل المعنية، ولا يجوز استنتاجها من تقرير فرع سابق.

## المعمارية العامة المثبتة بالعقود

- Mujeeb 24 هو Go Backend متعدد التجار، وPostgreSQL مصدر الحقيقة لبيانات التجارة والمحادثات والمعاملات والسجلات.
- Catalog Domain يتكوّن من `Catalog → CatalogItem → Attributes/Variants → Offers`؛ الأسعار والتوفر التجاري في Offers.
- API وDTO وOpenAPI مصدرها العقود المسجّلة في Go، ومصدر آخر نسخة مولّدة `api/openapi/mujeeb24-dashboard-v1.generated.yaml`.
- SocialAPI ناقل للقنوات وليس مصدر الحقيقة التجارية. عمليات الإرسال والردود لها تحقق وتفويض ومسارات تشغيل مستقلة عن عقد Merchant Catalog.
- لا تغير ملفات migrations المطبقة سابقًا لتعديل سلوك مساعد الكتالوج.

## مساعد التاجر للكتالوج — السلوك الموثق على PR #18

| الطبقة | السلوك |
| --- | --- |
| Discovery | أدوات قراءة الكتالوج والقوالب المسموح بها |
| Model | يفهم التاجر، يكتب `label` العربي و`data_type` و`validation_rules` والقيم، ويختار قالبًا موجودًا بمعرّف مثبت أو يقدم اسم قالب جديدًا |
| Output | `merchantCatalogWireSchema` و`catalogWireParse` → Canonical Proposal V3 |
| Approval | تخزين المقترح للمراجعة فقط؛ لا تعديل على المنتج قبل موافقة التاجر |
| Go executor | يتحقق من المراجع والبيانات والقواعد، وينشئ/يعيد استخدام تعريفات بقيم `attr_...`، ويربط قالب العنصر وإصداره وينفذ ضمن معاملة |
| PostgreSQL | `attribute_definitions(attribute_key,label,data_type,validation_rules)`، و`catalog_items.attributes` و`variants.attributes` كقيم JSONB |
| React | تعرض التسميات من `AttributeDefinition.label`، لا من ترجمة مفاتيح JSON |

- `validation_rules.enum` تحوي القيم المسموحة؛ القيم المختارة لعنصر/خيار محفوظة تحت المفتاح `attr_...` في JSONB. `select/multi_select` لهما قواعد تحقق، ولا تنشئ القوالب Variants تلقائيًا.
- `AttributeSchema` اختياري في Domain؛ المنتجات القديمة بلا قالب ومفاتيحها الحرة لا تُعرَّب تلقائيًا.
- هذا الفرع منفصل عن `main`؛ لا يثبت التوثيق بمفرده نجاح الاتصال الحي مع DeepSeek أو نشره.

## المراجع الحالية

- [Merchant Catalog AI — Authoring](../contracts/ai/11-merchant-catalog-ai-authoring-closed.md)
- [Merchant Catalog AI — Proposal V3](../contracts/ai/12-merchant-catalog-ai-proposal-contract-closed.md)
- [عقد القوالب العربية وKey/Label/Value](architecture/merchant-catalog-attribute-schema-ai-contract-ar.md)
- [إضافة مزود AI](architecture/adding-ai-provider-runbook-ar.md)
- [Catalog Contract](../مخطط%20مهم/Mujeeb%2024%20%E2%80%94%20Catalog%20Contract.md)
- [الهيكلة والمسؤوليات](structure_decision_ar.md)
- [سجل القرارات المعمارية](decision-log-ar.md)

**قاعدة للصيانة:** عند تغيير Merchant Catalog، حدّث المرجع الموحّد وعقد Proposal ثم الفهرس المعني في نفس PR. لا تضف نسخة ثانية من مستند سابق أو ملحقًا يحافظ على تعليمات متعارضة.
