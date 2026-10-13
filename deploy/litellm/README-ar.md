# LiteLLM Gateway — تشغيل تجريبي منفصل عن مجيب

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
