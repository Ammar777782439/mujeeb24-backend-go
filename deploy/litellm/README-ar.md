# LiteLLM Gateway — تشغيل تجريبي منفصل عن مجيب


## Render Blueprint: إضافة البوابة بجانب API (تجريبي، 2026-10-14)

- يضم ملف الجذر `render.yaml` خدمتين مستقلتين: `mujeeb24-api` الموجودة، و`mujeeb24-litellm` الجديدة من الصورة الرسمية المثبتة `ghcr.io/berriai/litellm:v1.103.0` (نوع `runtime: image`، خطة `free`). لا يغيّر هذا التعديل فرع خدمة API الموجودة أو يفعّل الردود تلقائيًا.
- أنشئ **قاعدة PostgreSQL مستقلة ودائمة** للبوابة خارج قاعدة مجيب التجارية (مثل Neon)، ثم ضع رابطها المشفّر في متغير خدمة `mujeeb24-litellm` المسمى `DATABASE_URL` داخل Render Dashboard. لا تستخدم `DATABASE_URL` الخاص بمجيب. لا تتضمن هذه الـBlueprint قاعدة PostgreSQL.
- اضبط على خدمة LiteLLM أربعة أسرار من Render Dashboard: `DATABASE_URL` و`LITELLM_MASTER_KEY` و`LITELLM_SALT_KEY` و`UI_PASSWORD`. استخدم مفتاحين عشوائيين منفصلين للمفتاح الرئيسي وSalt ولا تغيّر Salt بعد حفظ مفاتيح المزودين. اسم مستخدم UI هو `admin` في Blueprint.
- **إذا كانت Blueprint موجودة مسبقًا:** Render لا يطالبك تلقائيًا بقيم `sync: false` الجديدة عند مزامنتها لاحقًا. افتح خدمة البوابة الجديدة `Environment` وأدخل القيم يدويًا، ثم انشرها. إن كانت هذه أول Blueprint جديدة، قد تظهر مطالبة بالقيم ضمن خطوات الإنشاء.
- مزامنة `render.yaml` تتم من صفحة Blueprint في Render، وليست من GitHub وحده. لا تفترض أن رفع هذا التعديل أنشأ الخدمة أو بدأ تشغيلها. افحص Logs وتحقق من `https://<gateway-host>/health/readiness` ثم أنشئ النماذج الثلاثة `mujeeb-customer` و`mujeeb-catalog` و`mujeeb-merchant` من لوحة LiteLLM وأنشئ **Virtual Key** لمجيب.
- الخدمة الحالية `mujeeb24-api` تشير داخل Blueprint إلى الفرع `refactor/merchant-catalog-ai-v2-structure`. لم نغيّر فرعها عمدًا. قبل ربطها بالبوابة، انشر نسخة باك تدعم LiteLLM بموافقة صريحة، ثم اضبط `LITELLM_BASE_URL` على رابط البوابة الفعلي و`LITELLM_API_KEY` على Virtual Key (ليس Master Key)، مع بقية متغيرات `LITELLM_*` في ملف الجذر `.env.example`.
- **تحذير الباقة المجانية:** كل خدمة Render Free تنام عند الخمول وتملك ذاكرة محدودة. ربما لا يقلع LiteLLM ضمن 512 MB. البوابة العامة تتطلب تقييد المفاتيح والميزانيات وتدقيق أمان واجهة الإدارة؛ هذا إعداد تجربة لا ضمان إنتاج. صورة `runtime: image` لا تحدث نفسها تلقائيًا بمجرد صدور إصدار جديد.
- المرجع الرسمي: https://render.com/docs/blueprint-spec و https://docs.litellm.ai/docs/proxy/deploy


هذا المجلد يشغّل **LiteLLM + PostgreSQL خاص بإعدادات البوابة**. لا ينشئ قاعدة بيانات تجارية ثانية ولا يغيّر PostgreSQL الخاص بمجيب.

## 1. تشغيل LiteLLM (Windows PowerShell من جذر مستودع الباك)

تأكد أن Docker Desktop يعمل وأن المنفذ 4000 متاح. نفّذ:

```powershell
Copy-Item deploy/litellm/.env.example deploy/litellm/.env
notepad deploy/litellm/.env
```

لإنشاء ثلاث قيم عشوائية على Windows PowerShell دون تثبيت OpenSSL:

```powershell
function New-LocalSecret {
    $bytes = New-Object byte[] 32
    $rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
    try { $rng.GetBytes($bytes) } finally { $rng.Dispose() }
    return ([BitConverter]::ToString($bytes)).Replace('-', '').ToLowerInvariant()
}
"Master key: sk-$(New-LocalSecret)"
"Salt key:   sk-$(New-LocalSecret)"
"DB secret:  $(New-LocalSecret)"
```

انسخ القيم إلى ملف `deploy/litellm/.env` المحلي فقط. لا تنسخها إلى GitHub أو المحادثات.

ضع قيمًا **سرية عشوائية مختلفة** في `LITELLM_MASTER_KEY` و`LITELLM_SALT_KEY` و`LITELLM_POSTGRES_PASSWORD`.
يبدأ مفتاحا LiteLLM بـ `sk-`. لا تستخدم أمثلة ثابتة، ولا ترسل أسرارك في المحادثة، ولا تغيّر `LITELLM_SALT_KEY` بعد إضافة مفاتيح المزودين المشفرة.

```powershell
docker compose --env-file deploy/litellm/.env -f deploy/litellm/compose.yaml config
docker compose --env-file deploy/litellm/.env -f deploy/litellm/compose.yaml up -d
docker compose --env-file deploy/litellm/.env -f deploy/litellm/compose.yaml ps
curl.exe http://127.0.0.1:4000/health/readiness
```

افتح `http://127.0.0.1:4000/ui` وسجل الدخول بحساب المدير و`LITELLM_MASTER_KEY` من ملف البوابة المحلي.

**هذا ملف تشغيل محلي للتجارب.** المنفذ لا يقبل الاتصال من الإنترنت. لا تعرّض لوحة الإدارة عبر Cloudflare Tunnel، ولا تستخدم صورة `main-stable` المتغيرة في الإنتاج: ثبّت نسخة أو SHA موثوقًا وتأكد من تحديثاتها الأمنية أولًا.

## 2. إعداد النماذج داخل LiteLLM

من لوحة LiteLLM أضف النماذج التي تملك مفاتيحها، وأنشئ ثلاثة aliases:

- `mujeeb-customer` — النموذج يجب أن يدعم tools/Function Calling لاختيار Capability واحدة.
- `mujeeb-catalog` — نموذج يدعم JSON Schema والكتالوج الكامل والدفعات والسياق المطلوب.
- `mujeeb-merchant` — نموذج يدعم Chat Completions + tools + JSON Schema الصارم في نفس الطلب.

ابدأ **بنموذج واحد مؤهل لكل اسم**، ثم أضف مزودين احتياطيين من الواجهة بعد نجاح اختبارات الأدوار الثلاثة. إعداد نماذج متعددة لا يضمن أن جميعها تدعم نفس الأدوات؛ لا تُفعّل احتياطيًا قبل اختبار عقده.

أنشئ **Virtual Key** لمجيب من `Virtual Keys`، بصلاحية الوصول إلى aliases الثلاثة. ضعه في `LITELLM_API_KEY` داخل بيئة الباك، وليس مفتاح مدير LiteLLM ولا مفتاح Gemini/Groq نفسه.

## 3. تشغيل باك إند مجيب باستخدام LiteLLM فقط

ملف الجذر `.env.example` يحتوي الإعدادات، وانسخه إلى `.env` المحلي المُستثنى من Git:

```dotenv
LITELLM_GATEWAY_ONLY=true
LITELLM_BASE_URL=http://127.0.0.1:4000
LITELLM_API_KEY=YOUR_GATEWAY_VIRTUAL_KEY
LITELLM_USD_TO_YER_RATE=YOUR_ACCOUNTING_RATE
LITELLM_CUSTOMER_MODEL=mujeeb-customer
LITELLM_CATALOG_MODEL=mujeeb-catalog
LITELLM_MERCHANT_MODEL=mujeeb-merchant
LITELLM_HTTP_TIMEOUT=20s
LITELLM_MAX_OUTPUT_TOKENS=4096
LITELLM_MAX_INPUT_CHARACTERS=48000
LITELLM_MAX_CONTEXT_TOKENS=32768
LITELLM_MERCHANT_MAX_REQUEST_BYTES=524288
LLM_ENABLED=false
GEMINI_API_KEY=
AUTOREPLY_ENABLED=false
```

`DATABASE_URL` و`AI_CONFIG_ENCRYPTION_KEY` لا يزالان مطلوبين كما في مجيب الحالي. لتجربة API دون إرسال رسائل للعملاء اترك `AUTOREPLY_ENABLED=false` و`CUSTOMER_SALES_ENABLED=false` إلى أن تنهي الاختبارات. إذا أردت اختبارًا فعليًا للقنوات، تحتاج أيضًا بيانات SocialAPI وتفعيلها عن قصد.

**مهم عند تشغيل API داخل Docker:** `127.0.0.1` داخل حاوية Go لا يعني جهازك؛ استخدم عنوان بوابة خاصًا قابلًا للوصول من الحاوية/شبكتها، مع اتصال آمن. عند تشغيل Go مباشرة على Windows من خلال `go run` يكون عنوان loopback أعلاه مناسبًا.

## 4. ماذا تعني إزالة القديم؟

`LITELLM_GATEWAY_ONLY=true` يتجاوز استدعاء Gemini المباشر ولا يحتاج `GEMINI_API_KEY` لأعمال B2C أو B2B أو التقييم والتلخيص. لكنه لا يحذف ملفات Gemini القديمة تلقائيًا. الإزالة النهائية للكود المكرر مرهونة بنجاح اختبارات المزودين الحقيقية، وتغطية التكاليف والإجراءات التشغيلية؛ لا نكسر العقود القديمة أو ترحيلات قاعدة البيانات.

## 5. أوامر فحص وإيقاف

```powershell
docker compose --env-file deploy/litellm/.env -f deploy/litellm/compose.yaml logs --tail=100 litellm
docker compose --env-file deploy/litellm/.env -f deploy/litellm/compose.yaml down
```

لا تستخدم `down -v` على LiteLLM أو مجيب؛ فهذا يحذف بيانات PostgreSQL وإعدادات المفاتيح.

المرجع الرسمي: https://docs.litellm.ai/docs/proxy/docker_quick_start

### ميزانية الكتالوج الكامل دون فقدان البيانات

- عند تشغيل مسار LiteLLM، تحسب ميزانية طلبات الكتالوج من `LITELLM_MAX_CONTEXT_TOKENS` مطروحًا منها `LITELLM_MAX_OUTPUT_TOKENS` وهامش أمان 10% من السياق (على الأقل 2048 Token). لم يعد حد 8000 ثابتًا لبوابة LiteLLM؛ المسار المباشر القديم لم يتغير.
- عيّن `LITELLM_MAX_CONTEXT_TOKENS` حسب *أصغر نافذة سياق حقيقية* للنماذج المعتمدة في مجموعة `mujeeb-catalog`، بما فيها جميع fallback. اسم النموذج العام وحده لا يخبر Go بقدرة النماذج الخلفية؛ يجب تدقيق إعداد LiteLLM عند إضافة أي مزود.
- عدّ الرموز في LiteLLM **تقديري محافظ** لا يمثل عدًا دقيقًا عند جميع المزودين؛ لذلك تبقى حدود القبول أكثر تحفظًا. لا يقصّ مجيب النصوص التجارية ولا يحذف الأسعار والمواصفات لتجاوز هذا الحد.
- أي جزء تجاري غير قابل للتجزئة ويتجاوز الميزانية يجب توجيهه لمسار معتمد بسياق أكبر، أو رفضه بوضوح. لا ترفع حدود السياق المعلنة دون التحقق من جميع النماذج البديلة.
