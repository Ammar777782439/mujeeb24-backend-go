# اختبار LiteLLM الحي — بوابة مجيب / نموذج الكتالوج

## مسارات الوصول المعتمدة

- **لوحة LiteLLM Dashboard/UI:** https://webhook.mujeeb24.com/ui/ — هنا تسجيل الدخول إلى لوحة الإدارة باستخدام حساب UI، ولا يُستخدم هذا المسار لاستدعاء النماذج.
- **أساس REST API:** https://webhook.mujeeb24.com (قيمة `LITELLM_BASE_URL` دون `/ui` ودون `/v1`).
- **واجهة النماذج:** `GET https://webhook.mujeeb24.com/v1/models` مع Bearer Virtual Key.
- **استدلال النماذج:** `POST https://webhook.mujeeb24.com/v1/chat/completions` مع Bearer Virtual Key.
- **الجاهزية:** `GET https://webhook.mujeeb24.com/health/readiness`.

**التمييز الأمني:** حساب UI وكلمة مرور اللوحة مختلفان عن LiteLLM Virtual Key. نجاح الوصول إلى `/ui/` أو `/health/readiness` لا يثبت صلاحيات الـAPI. اختبار خارجي بتاريخ 15 أكتوبر 2026 أكد `/health/readiness → 200`، ولكن الاختبار المصادق عليه أعاد `403` لكلٍ من `/v1/models` و`/v1/chat/completions`. صَنَّف التشخيص جسم الخطأ كـJSON API دون ظهور نوع/رمز خطأ موثوق؛ لذلك **مصدر رفض 403 لم يُحدد بعد**، ولا يعدّ اختبار النموذج ناجحًا.

**الحالة:** لا يُعتبر اختبار النموذج الحي ناجحًا حتى يصدر GitHub Actions نتيجة SUCCESS من `Live LiteLLM catalog model acceptance`، وتظهر رسالتا PASS من اختبار المصادقة واستجابة النموذج.

## تشخيص نهائي موثق — Cloudflare Error 1010 (2026-10-15)

تم إثبات أن رفض الاستدلال الخارجي يحدث في Cloudflare **قبل LiteLLM**. تم تنفيذ اختبارات GET إلى `https://webhook.mujeeb24.com/v1/models` من GitHub Actions دون مفتاح، ثم بنفس المسار بمفتاح `LITELLM_API_KEY` المخزن في GitHub Secrets. النتيجة في الحالتين:

- `HTTP 403`; `server: cloudflare`؛ `content_type: application/json`؛ `cloudflare_error: true`.
- `error_code: 1010`؛ `error_category: access_denied`؛ `error_name: browser_signature_banned`؛ `owner_action_required: true`.
- Ray ID بدون مفتاح: `a4b037edadcaff0f`؛ مع المفتاح: `a4b037ee3c00b642`.
- [اختبار GitHub الحقيقي](https://github.com/Ammar777782439/mujeeb24-backend-go/actions/runs/38769805652). **لا يوجد اختبار نموذج ناجح بعد**.

السبب حسب وثائق Cloudflare: **Browser Integrity Check (BIC)** أو سياسة حظر توقيع العميل في Cloudflare. الصور الخاصة بـ Virtual Key لا تشير إلى حظر الميزانية أو انتهاء الصلاحية، لكنها لا تثبت صلاحية النموذج؛ لا يمكن تقييم ذلك حتى يصل الطلب إلى LiteLLM.

### العلاج المحدد والمفضل

1. في Cloudflare افتح نطاق `mujeeb24.com` ثم **Security → Analytics → Events**، وحدد الطلب بواسطة أحد Ray IDs أعلاه أو الوقت (UTC) لمعرفة سياسة الحظر.
2. في **Security → Security rules → Create rule → Custom rules** أنشئ قاعدة **Skip** مقيّدة بـ `http.host eq "webhook.mujeeb24.com"` و`starts_with(http.request.uri.path, "/v1/")`، واختر تجاوز **Browser Integrity Check فقط**. أبقِ WAF والأجهزة الأخرى وسياسات المعدل والمصادقة قائمة. يمكن إضافة مسار `/health/readiness` عند الحاجة، ولا تضف `/ui/`.
3. بديل مكافئ: Configuration Rule لتعطيل BIC لمسارات API المسموح بها فقط؛ لا تعطّل فحص الموقع كاملًا لمجرد تشغيل اختبار CI.
4. أعد اختبار `/v1/models` دون مفتاح (متوقع أن يصل إلى رفض LiteLLM `401`) ثم بالمفتاح؛ لا تستخدم نتائج BIC القديمة بعد تغيير السياسة. عند نجاح المصادقة انتقل إلى اختبار الكتالوج الحقيقي على `/v1/chat/completions`.

المراجع: https://developers.cloudflare.com/support/troubleshooting/http-status-codes/cloudflare-1xxx-errors/error-1010/ و https://developers.cloudflare.com/waf/tools/browser-integrity-check/ .

---

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
