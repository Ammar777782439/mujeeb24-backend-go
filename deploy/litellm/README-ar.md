# LiteLLM Gateway — تشغيل تجريبي منفصل عن مجيب

> **Render مهم:** لإنشاء LiteLLM بجانب خدمة مجيب الحالية دون إنشاء/تعديل باك إند ثانٍ، استخدم **Blueprint Path: `deploy/litellm/render.blueprint.yaml`** عند إنشاء Blueprint جديدة. لا تستخدم `render.yaml` في جذر المستودع لهذا الغرض؛ لأنه يصف `mujeeb24-api` أيضًا وقد يؤثر على إعدادات خدمتها. تغيير Branch لـ`mujeeb24-api` ثم Manual Deploy لا ينفذ `generateValue` ولا ينشئ Environment variables داخل Render.
>
> **ما يُضبط تلقائيًا:** Render ينشئ `UI_PASSWORD` و`LITELLM_MASTER_KEY_SEED` و`LITELLM_SALT_KEY_SEED` عند إنشاء Blueprint ويحفظها؛ كود Go يستخدم أسماء المسارات الافتراضية `mujeeb-customer` و`mujeeb-catalog` و`mujeeb-merchant` إن لم تُحدد. **ما لا يمكن اختراعه تلقائيًا:** قاعدة مستقلة دائمة لتخزين إعدادات LiteLLM (`DATABASE_URL` الخاص بالبوابة)، مفتاح Virtual Key صالح صادر من البوابة (`LITELLM_API_KEY` في باك مجيب)، عنوان البوابة، مفاتيح مزودي النماذج، وسعر تحويل الدولار إلى الريال المعتمد. لا تستخدم `DATABASE_URL` الخاص بتجار مجيب للبوابة. لا تفعّل `AUTOREPLY_ENABLED=true` إلا بعد اكتمال هذه القيم واختبارها؛ ففشل التهيئة يجب أن يوقف الردود بأمان ولا يولد مفاتيح ثابتة أو وهمية.

## Render Blueprint: نشر LiteLLM من الهاتف بأسرار تتولد تلقائيًا (2026-10-14)

- ملف `deploy/litellm/render.blueprint.yaml` **المخصص لهذا النشر** يحتوي خدمة واحدة فقط هي `mujeeb24-litellm` على `free`، ولا يصف `mujeeb24-api`. تُبنى البوابة من `deploy/litellm/Dockerfile.render` فوق الصورة الرسمية الثابتة `ghcr.io/berriai/litellm:v1.103.0`، من فرع `feat/ai-litellm-multi-provider-gateway`. ملف الجذر `render.yaml` قديم ويصف باك مجيب أيضًا؛ لا تستخدمه لهذا النشر.
- **الأسرار الثلاثة تلقائية:** Render يولّد ويحفظ `LITELLM_MASTER_KEY_SEED` و`LITELLM_SALT_KEY_SEED` و`UI_PASSWORD` باستخدام `generateValue: true`. ملف `render-entrypoint.sh` يحول أول قيمتين عند تشغيل البوابة إلى `LITELLM_MASTER_KEY=sk-<seed>` و`LITELLM_SALT_KEY=sk-<seed>`، ثم يستدعي entrypoint الأصلي. لا تُكتب أي قيمة سرية في GitHub أو Logs. تظل القيم ثابتة بين إعادة النشر، ولا تُبدَّل عند إعادة مزامنة Blueprint.
- **المطلوب إدخاله يدويًا مرة واحدة:** `DATABASE_URL` فقط، ويكون رابط PostgreSQL مباشرًا أو متوافقًا من **مشروع Neon مستقل خاص بـLiteLLM**، وليس قاعدة تجار مجيب. لإنشاء Blueprint جديدة سيطلب Render الرابط أثناء الإنشاء؛ عند إضافة المتغير إلى خدمة/Blueprint قائمة، افتح Environment للخدمة وضع الرابط هناك لأن `sync: false` لا يطالبك به مجددًا عند تحديث Blueprint موجودة.
- **بيانات دخول اللوحة:** `UI_USERNAME=admin`، وكلمة المرور تُولَّد تلقائيًا تحت `UI_PASSWORD`. من الجوال: Render > خدمة `mujeeb24-litellm` > Environment > إظهار القيمة ونسخها إلى تسجيل الدخول في `https://<gateway>.onrender.com/ui`. لا تشارك هذه القيمة في المحادثات. إذا احتجت مفتاح المدير API، قيمته **`sk-` متبوعة بالقيمة المحفوظة في `LITELLM_MASTER_KEY_SEED`**. استخدم Virtual Key منفصلًا داخل مجيب، لا Master Key.
- **حماية البيانات القديمة:** `render-entrypoint.sh` يحترم قيمة `LITELLM_MASTER_KEY` أو `LITELLM_SALT_KEY` الموجودة مسبقًا إن ضُبطت يدويًا، ولا يستبدل Salt سبق أن شفّر مفاتيح مزودين بها. لا تعمد إلى تدوير الـSalt بعد تخزين مفاتيح المزودين في Neon.
- **التحقق:** من Render > Blueprints أنشئ Blueprint باستخدام **Blueprint Path: `deploy/litellm/render.blueprint.yaml`** من نفس الفرع، ثم نفّذ Sync لهذا الملف فقط. راجع الخدمات المعروضة قبل التأكيد: يجب أن تظهر `mujeeb24-litellm` فقط دون إضافة/تعديل `mujeeb24-api`. بعد النشر افحص Logs و`/health/readiness`. `autoDeployTrigger: off` للبوابة، لذا تُنشر تحديثاتها يدويًا بعد مراجعتها. رفع الملف إلى GitHub **ليس نشرًا فعليًا في حساب Render**.
- **ثم الربط:** أضف نماذج وaliases `mujeeb-customer` و`mujeeb-catalog` و`mujeeb-merchant` وأنشئ Virtual Key من LiteLLM، وبعد تحديث باك مجيب إلى النسخة الداعمة، اضبط `LITELLM_BASE_URL=https://<gateway>.onrender.com` و`LITELLM_API_KEY=<virtual-key>` و`LITELLM_USD_TO_YER_RATE` وبقية إعدادات `LITELLM_*` في باك Render. لا تفعّل الردود الحية قبل اختبارها.
- **تحذير Free:** قد تنام البوابة والباك عند الخمول، وقد تتجاوز LiteLLM ذاكرة Render Free؛ نجاح كتابة Blueprint لا يضمن إقلاع الصورة. HTTPS العام والمفاتيح لا يغنيان عن تدقيق أمان واجهة الإدارة وحدود الإنفاق. هذا نشر تجربة فقط.
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
