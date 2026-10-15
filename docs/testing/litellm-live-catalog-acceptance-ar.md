# اختبار LiteLLM الحي — بوابة مجيب / نموذج الكتالوج

## مسارات الوصول المعتمدة

- **لوحة LiteLLM Dashboard/UI:** https://webhook.mujeeb24.com/ui/ — هنا تسجيل الدخول إلى لوحة الإدارة باستخدام حساب UI، ولا يُستخدم هذا المسار لاستدعاء النماذج.
- **أساس REST API:** https://webhook.mujeeb24.com (قيمة `LITELLM_BASE_URL` دون `/ui` ودون `/v1`).
- **واجهة النماذج:** `GET https://webhook.mujeeb24.com/v1/models` مع Bearer Virtual Key.
- **استدلال النماذج:** `POST https://webhook.mujeeb24.com/v1/chat/completions` مع Bearer Virtual Key.
- **الجاهزية:** `GET https://webhook.mujeeb24.com/health/readiness`.

**التمييز الأمني:** حساب UI وكلمة مرور اللوحة مختلفان عن LiteLLM Virtual Key. نجاح الوصول إلى `/ui/` أو `/health/readiness` لا يثبت صلاحيات الـAPI. اختبار خارجي بتاريخ 15 أكتوبر 2026 أكد `/health/readiness → 200`، ولكن الاختبار المصادق عليه أعاد `403` لكلٍ من `/v1/models` و`/v1/chat/completions`. صَنَّف التشخيص جسم الخطأ كـJSON API دون ظهور نوع/رمز خطأ موثوق؛ لذلك **مصدر رفض 403 لم يُحدد بعد**، ولا يعدّ اختبار النموذج ناجحًا.

**الحالة:** لا يُعتبر اختبار النموذج الحي ناجحًا حتى يصدر GitHub Actions نتيجة SUCCESS من `Live LiteLLM catalog model acceptance`، وتظهر رسالتا PASS من اختبار المصادقة واستجابة النموذج.

## ما الذي ثَبُت بالفعل؟

- نفق `https://webhook.mujeeb24.com` أجاب من GitHub Actions: `GET /health/readiness → HTTP 200`، و`GET /v1/models` بدون مفتاح → `HTTP 401`. هذا يؤكد توجيه HTTPS إلى خدمة محمية؛ **لا يثبت وجود Alias أو استدعاء نموذج**.
- الاختبارات البرمجية وقاعدة PostgreSQL في Quality Gate منفصلة عن اختبار النموذج الحقيقي.
- موقع `127.0.0.1:4000` صالح للوصول إلى LiteLLM من جهاز المضيف الذي يشغله؛ لا يمكن لبيئة CI الوصول إلى loopback في جهاز المستخدم.

## التشغيل الآمن للاختبار الحقيقي

1. في لوحة LiteLLM أنشئ **Virtual Key جديدًا للاختبار فقط** بصلاحية نموذج `mujeeb-catalog`، وميزانية/مدة قصيرة. لا تستخدم مفتاح المدير، ولا الأسرار القديمة المنشورة.
2. في المستودع الصحيح `Ammar777782439/mujeeb24-backend-go` افتح **Settings → Secrets and variables → Actions → New repository secret**.
3. اختر الاسم بالضبط: `LITELLM_API_KEY`، والصق مفتاح اختبار LiteLLM الجديد في **قيمة السر** هناك فقط. لا تضعه في Issues أو Comments أو Pull Requests أو ملفات أو Logs.
4. افتح **Actions → Live LiteLLM catalog model acceptance → Run workflow**، واختر فرع `feat/catalog-complete-commercial-entity-atomic`، ثم Run.
5. راجع رسائل PASS/FAIL في سجل الوظيفة. لا تنسخ رموز أو Headers مصادقة. بعد الاختبار ألغِ مفتاح الاختبار من LiteLLM وامسح سر GitHub إذا لن يُستخدم مجددًا.

## ما الذي يختبره المسار الحي؟

- المصادقة الحقيقية من الإنترنت في `GET /v1/models`.
- وجود `mujeeb-catalog` ضمن النماذج المصرح بها.
- استدعاء `POST /v1/chat/completions` عبر بوابة LiteLLM مع JSON Schema.
- مثال اصطناعي كامل: Catalog + schema + definitions + item + اثنان من Variants + عروض Variant مرتبطة + عرض على مستوى Item.
- التحقق من رجوع جميع المعرفات المحددة **بالضبط** دون سقوط أو إضافة، ومن إصدار Schema.

**حدود الإثبات:** الاستدعاء يعمل على بيانات اصطناعية واحدة؛ لا يضمن قدرة النموذج على التفكير الصحيح مع كل الكتالوجات، ولا يتحقق من اختيار/سياسات/تكلفة كل fallback. ويلزم اختبار فعلي لمسار مجيب مع PostgreSQL والتحديث المتزامن ومزودي fallback عند توفر بيئة اختبار متصلة، دون إرسال رسائل حقيقية للعملاء.

## ملاحظات أمنية وتشغيلية

- مفاتيح UI الخاصة بالداشبورد ليست مفاتيح REST API للنماذج. لا تستخدم كلمة مرور واجهة الإدارة للـ Bearer Authorization.
- المفتاح وكلمات المرور ومفاتيح التوقيع والتشفير التي تم كشفها في المحادثات تحتاج خطة تدوير. **لا تستبدل مفاتيح تشفير البيانات أو JWT في الإنتاج فجأة**؛ رتب تغييرًا متوافقًا مع تخزين المفاتيح المشفرة وانتهاء صلاحية التوكنات.
- هذه الوظيفة **يدوية عمدًا** لأنها قد تتسبب بتكلفة عند كل تشغيل؛ Quality Gate يختبر Go وPostgreSQL دون تكلفة مزود.
- لا تستخدم Environment Secrets داخل `.env` الحقيقي في GitHub؛ حافظ على مفاتيح التاجر وقنوات SocialAPI خارج اختبار LiteLLM.
