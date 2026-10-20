# حالة العمل — Universal Catalog AI v3

الفرع: `feat/universal-catalog-ai-v3`. لا دمج إلى `main` أو `refactor/merchant-catalog-ai-v2-structure` ضمن هذه المهمة.

## التنظيف الجاري

- [x] حذف مسار Customer Sales tool-loop القديم وregistry وأداة catalog_data واختباراتها.
- [x] حذف fallback تجميع الكتالوج الكامل وProjectionBuilder غير المستخدم.
- [x] إزالة بقايا searchable والدوال غير المستدعاة.
- [x] فصل الدوال المشتركة المستخدمة فعليًا عن الكود المحذوف.
- [x] تثبيت Go وتشغيل البناء وvet محليًا.
- [x] إكمال التحقق من جميع الاختبارات المحلية بعد تحديث fixtures للعقود الحالية.
- [x] إزالة fallback الـWebhook غير المحدود واعتماد Worker Pool فقط.
- [ ] مراجعة كل المسارات والحقول المتبقية قبل تصنيفها غير مستخدمة.
- [x] تحديث وثائق الحالة وتعليمات الفرع الحالي وإزالة مهام Chatwoot الملغاة.

## تحقق مستقل متبقٍ

- [ ] تشغيل PostgreSQL integration باستخدام POSTGRES_TEST_DSN على قاعدة معزولة.
- [ ] تشغيل OpenAPI drift وبوابة المستودع.
- [ ] التحقق من التشغيل المحلي على Windows وقاعدة البيانات.
- [ ] اختبارات المزود الحية عند التصريح بها وتهيئة بيانات الاعتماد خارج Git.

Chatwoot غير موجود في runtime ولا توجد مهام لإعادته أو تشغيله. migrations التاريخية محفوظة؛ لا حذف للبيانات ضمن تنظيف الكود.

## متابعة مؤجلة — Sales Lifecycle E2E

- [ ] عند التفرغ، أكمل قائمة التحقق الشاملة لدورة حياة البيع على فرع `feat/customer-sales-start-consent`: [قائمة اختبار Sales Lifecycle E2E](docs/testing/sales-lifecycle-e2e-checklist-ar.md).
- [ ] تشمل القائمة موافقة بدء البيع، وربط العميل والمحادثة، والتحقق من الكتالوج والعرض والسعر والتوفر، وإنشاء الـ Lead والـ `CommercialTransaction` وفق التنفيذ الفعلي، والرد/Outbox، وHuman Handoff، والتكرار، والـ leases، وإعادة المحاولة والتعافي.
- [ ] تحقق من نجاح `bash scripts/verify-repository.sh` مع `RUN_INTEGRATION=1` و`POSTGRES_TEST_DSN` على PostgreSQL معزولة، ثم تحقق من نتيجة GitHub Actions.
- [ ] لا تعتبر المسار مغلقًا ولا تعلّم هذه البنود كمكتملة حتى تُنفّذ الاختبارات فعليًا وتُراجع نتائجها. الاختبارات المتخطاة ليست ناجحة.
