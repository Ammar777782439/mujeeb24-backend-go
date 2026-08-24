# سياسة تنفيذ PostgreSQL Migrations — Mujeeb 24

## القرار

نستخدم **SQL migrations versioned** محفوظة داخل المستودع، ويُنفذها `cmd/migrate` عبر Migration Runner مكتوب في Go ومضمّن ضمن binary. لا نستخدم ORM AutoMigrate، ولا نضع تغييرات Schema غير قابلة للتتبع داخل startup الخاص بالـAPI.

## مكان الملفات

```text
migrations/
├── 000001_businesses.up.sql
├── 000002_business_policies.up.sql
└── ...
```

يجب أن تكون أسماء الملفات مرتبة رقميًا وبلا فجوات غير مقصودة. كل Migration تمثل تغييرًا صغيرًا قابلًا للمراجعة، ولا نخلط تغييرات Domain مختلفة في ملف واحد دون سبب موثق.

## Forward-only في Production

في بيئة Production نعتمد **forward-only migrations**. لا ينفذ التطبيق `down` تلقائيًا، ولا نستخدم rollback تدميريًا لاسترجاع بيانات. عند الخطأ نكتب Migration تصحيحية جديدة. يمكن توفير `down` للاختبارات المحلية أو بيئات التطوير فقط إذا كان ذلك آمنًا ولا يوحي بإمكانية حذف بيانات Production.

## توليد IDs والوقت

الـApplication هو مصدر UUID وtimestamps عند إنشاء Domain records، باستخدام `Clock` وID generator المعتمدين في Application/Domain. لذلك لا نضيف `DEFAULT now()` أو UUID database defaults إلى الجداول التجارية في هذه المرحلة، حتى لا تختلف دلالة وقت Domain في الاختبارات عن وقت PostgreSQL.

تحتوي جداول التشغيل على حقول وقت `NOT NULL`، ويجب أن يملأها التطبيق أو Migration fixture صراحة. إذا احتجنا لاحقًا إلى UUIDv7، يكون توليده في Application حتى تبقى قاعدة البيانات Provider-neutral.

## Transaction Boundary

كل Migration تُنفذ داخل transaction عندما يسمح PostgreSQL بذلك. لا توجد Network calls داخل Migration. لا نستخدم `CREATE INDEX CONCURRENTLY` داخل Migration transactional؛ إذا احتجناه لاحقًا يكون Migration منفصلًا وموثقًا كاستثناء.

## Schema Rules

يجب أن تطبق كل Migration:

- Composite Tenant FKs حيث تكون العلاقة بين Business-owned rows.
- `RESTRICT` أو lifecycle/archival بدل Cascade من `businesses` إلى السجلات التاريخية.
- Unique keys الخاصة بـProvider وConnection وEvent/Idempotency.
- Leases مع owner وexpiry في Event Ledger وOutbox.
- CHECK constraints للحالات والقيم التي تمثل Invariants ثابتة.
- فهارس تدعم الاستعلامات الفعلية، دون فهارس عشوائية.

## طريقة التحقق

قبل اعتبار المرحلة منتهية:

1. تُشغل Migrations من PostgreSQL فارغة.
2. تُشغل مرة ثانية للتأكد من أن runner يحفظ الإصدار ولا يعيد التنفيذ.
3. تُنفذ اختبارات duplicate وcross-tenant وlease وidempotency.
4. يُراجع `schema_migrations` والـforeign keys والـindexes.
5. يُحدّث `docs/project-status-ar.md` بالنتيجة الفعلية فقط.

## القرار الحالي

اختيار **embedded, versioned SQL + Go runner** مناسب لحجم المشروع الحالي ويمنع اعتماد CLI خارجي غير مثبت في البيئة. عند الحاجة إلى إدارة migrations متقدمة جدًا يمكن استبداله لاحقًا دون تغيير عقود Domain أو Application، لأن SQL files هي المصدر المراجع للتغيير.
