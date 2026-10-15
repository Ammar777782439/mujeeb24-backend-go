# Mujeeb 24 — اختبار LiteLLM الحقيقي للكتالوج (15 أكتوبر 2026)

**الحالة:** نجح اختبار Go الحقيقي لتقييم دفعة كتالوج والتقييم النهائي في 15 أكتوبر 2026، على الفرع `feat/catalog-complete-commercial-entity-atomic`. اختبارات Go/PostgreSQL المستقلة لا تعني أن جميع شروط التشغيل الإنتاجي أو المزودين الاحتياطيين قد اختُبرت.

## المسارات الصحيحة

- لوحة إدارة LiteLLM: https://webhook.mujeeb24.com/ui/
- `LITELLM_BASE_URL=https://webhook.mujeeb24.com` **دون** `/ui` أو `/v1`.
- التحقق من جاهزية الخدمة: `GET /health/readiness`.
- استكشاف النماذج بالمفتاح: `GET /v1/models`.
- **مسار Customer/Catalog AI الإنتاجي عبر Go:** `POST /v1beta/interactions`.
- مسار OpenAI-compatible الذي تستخدمه أجزاء أخرى من Go: `POST /v1/chat/completions`.
- حساب UI وكلمة مروره لا يمثلان `LITELLM_API_KEY`؛ الأخير GitHub Secret بقيمة Virtual Key.

## لماذا أعاد الاختبار الأول HTTP 403 بينما يعمل البرودكشن؟

ثبت عمليًا أن **بصمة عميل Python** كانت المحظورة، وليس النطاق العام أو Virtual Key بصورة عامة.

في [التشغيل المقارن رقم 38770783777](https://github.com/Ammar777782439/mujeeb24-backend-go/actions/runs/38770783777)، من **نفس GitHub Runner** ونفس النطاق ونفس GitHub Secret:

| المسار | Go net/http مثل باك مجيب | Python urllib |
|---|---|---|
| `GET /health/readiness` | `200` | لم يُختبر في تلك المقارنة |
| `GET /v1/models` بلا مفتاح | `401` | `403` |
| `GET /v1/models` بالمفتاح | `200` | `403` |

خطأ Python كان JSON صادرًا من Cloudflare يحمل `error_code=1010` و`error_name=browser_signature_banned`. بعد تجربة Go لم يعد هناك مبرر لتغيير صلاحيات مفتاح `mujeeb-backend` أو تعطيل Browser Integrity Check لمجرد إصلاح أداة الاختبار. حافظ على سياسات Cloudflare، واستخدم عميل Go للمطابقة مع البرودكشن.

## نتائج الاختبار الحي للكيان التجاري الكامل

نفذ [التشغيل الناجح رقم 38772383313](https://github.com/Ammar777782439/mujeeb24-backend-go/actions/runs/38772383313) عبر `gateway.NewLiteLLMCatalogAdapter` و`litellm.Client` الحقيقيين من المستودع. **لا يوجد Model Mock أو httptest** في هذا التشغيل.

أُرسل **منتج اصطناعي واحد** مع:
- Catalog واحد، Schema بإصدار 2 وتعريفَي خاصية، وحقل `resolved_attributes` يحافظ على معنى كل مفتاح وقيمته.
- Variant أزرق وVariant أسود، `effective_attributes` وقيم موروثة، وعرض مرتبط بكل Variant.
- عرض مباشر ثالث على مستوى المنتج؛ المبالغ الأصلية `12000.0000` و`15000.0000` و`10000.0000`، دون تحويلات float64 أو إسقاط عروض.

تم فحص **طلب HTTP الصادر فعليًا** قبل الإرسال إلى `/v1beta/interactions` للتأكد من وجود الهوية الأصلية وجميع المعرفات والتعريفات والمبالغ وروابط العروض. ذلك الفحص لا يعدّل الـHTTP Request ولا يطبع الأسرار أو الاستجابة الخام.

| المرحلة | نتيجة التنفيذ |
|---|---|
| مقدّر توكنات الدفعة الحقيقي | `12,213` توكن تقديري (PASS) |
| طلب تقيـيم الدفعة | PASS — كل الأدلة التجارية في الطلب |
| استجابة المرشحين | مرشحان؛ معرفات العروض **ثلاثة** في أحدهما، ولا يُلزم العقد كل مرشح منفرد أن يعيد كل العروض |
| مقدّر توكنات المرحلة النهائية | `21,777` توكن تقديري (PASS) |
| طلب التقييم النهائي | PASS — كل العروض والخيارات والمواصفات والأسعار باقية في طلب Go الثاني |
| Final Capability | `respond_without_action` |
| أسعار العروض في الرد النهائي | **3 من 3** (PASS) |

**تنبيه حول الدقة:** اختيار مرشح أول بلا `offer_ids` لا يعني فقدان Offer من `candidateProjection`؛ العقد يجعل تلك المعرفات *إشارة صلة*، و`appendCandidateProjection` يعيد المنتج كاملًا للقرار النهائي. كان الفحص القديم يشترط خطأً أن يعيد **كل مرشح منفرد** جميع عروض المنتج، ولذلك أعطى نتيجة رفض مبكر غير مبررة. الاختبار الحي النهائي تحقق من طلب المرحلة النهائية ومن الأسعار في الرد.

## حدود هذه النتائج

هذا **اختبار حي حقيقي** لمسار Go ضد LiteLLM ونموذجه وضد عقد كامل لمنتج تجريبي، لكنه **ليس**:
- اختبار تشغيل شامل عبر قاعدة بيانات تاجر فعلية ورسائله وقناة SocialAPI.
- إثباتًا بأن النموذج سيفهم كل عنصر في كل كتالوج وعميل بنسبة 100%؛ الضمان البنيوي يختلف عن صحة استدلال النموذج.
- تحققًا حيًا من جميع مزودي fallback/القدرات/نوافذ السياق، أو من كتالوج ضخم ذي صفحات متعددة عبر المزود.
- بديلًا عن الاختبارات المحلية الحقيقية لـPostgreSQL وتغيّر المراجعة والتزامن؛ هذه تنفذ بشكل مستقل في Quality Gate.

## التشغيل عند الحاجة فقط

- Workflow: `.github/workflows/litellm-live-catalog.yml`، **Manual / workflow_dispatch فقط**.
- GitHub → Actions → **Live LiteLLM catalog model acceptance** → **Run workflow** → اختر فرع `feat/catalog-complete-commercial-entity-atomic`.
- يجب أن يكون `LITELLM_API_KEY` مضبوطًا في Repository Secrets. لا تنسخ المفتاح في المستودع أو السجلات.
- هذا الاختبار قد يستهلك رصيد المزود؛ شغّله عند الحاجة فقط، ودوّر المفاتيح المنشورة سابقًا بخطة آمنة (خصوصًا JWT وتشفير البيانات).
- لا تغيّر `main` ولا تعِد إدخال تجزئة المنتج القديم أو ترخيص إرسال أدلة ناقصة.

## روابط الأدلة

- [Go/Python Cloudflare comparison #38770783777](https://github.com/Ammar777782439/mujeeb24-backend-go/actions/runs/38770783777)
- [First real Go batch attempt #38771316662](https://github.com/Ammar777782439/mujeeb24-backend-go/actions/runs/38771316662) — رفض ناتج عن فرض غير صحيح حول معرّفات كل مرشح منفرد.
- [Real Go batch+final accepted #38772383313](https://github.com/Ammar777782439/mujeeb24-backend-go/actions/runs/38772383313)
