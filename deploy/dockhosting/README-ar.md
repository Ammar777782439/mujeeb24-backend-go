# نشر Mujeeb 24 مجانًا — DockHosting

هذا المسار مخصص للمرحلة الأولى بدون شراء VPS أو Domain وبدون بطاقة ائتمان.

## ما الذي أصبح جاهزًا في المستودع؟

- Dockerfile يبني: API + Worker + Migration + Runtime واحد.
- الـRuntime يشغّل migrations أولًا، ثم API وWorker داخل الحاوية نفسها.
- API يقرأ `PORT` من بيئة المنصة.
- GitHub Actions يبني صورة Docker للنشر على كل تغيير ذي صلة.
- PostgreSQL يمكن ربطه من DockHosting، ويجب أن تكون قيمة الاتصال متاحة للتطبيق في `DATABASE_URL`.

## إعداد DockHosting

1. أنشئ حساب DockHosting على الخطة Free.
2. أنشئ Project باسم مثل `mujeeb24`.
3. اربط المستودع:
   `Ammar777782439/mujeeb24-backend-go`
4. اجعل Root Directory هو جذر المستودع.
5. اترك Build/Start على Dockerfile الموجود في الجذر.
6. أنشئ PostgreSQL 16 داخل DockHosting.
7. اربط قاعدة PostgreSQL بالمشروع.
8. في متغيرات المشروع، اجعل متغير اتصال PostgreSQL الذي يولده DockHosting متاحًا باسم:
   `DATABASE_URL`
9. أضف المتغيرات المطلوبة أدناه.
10. فعّل Auto Deploy من Git إن كان الخيار ظاهرًا؛ بعدها كل Push إلى الفرع المتصل يعيد البناء والنشر.

## متغيرات التشغيل الأساسية

```text
APP_ENV=production
AUTH_ENABLED=true
JWT_ISSUER=mujeeb24

DATABASE_URL=<القيمة التي يولدها DockHosting ويجب وضعها باسم DATABASE_URL>

AI_CONFIG_ENCRYPTION_KEY=<32 bytes أو أكثر، base64 أو نص عشوائي لا يقل عن 32 بايت فعليًا>
JWT_ED25519_PRIVATE_KEY=<base64 خام لمفتاح Ed25519 الخاص 64 bytes>
JWT_ED25519_PUBLIC_KEY=<base64 خام لمفتاح Ed25519 العام 32 bytes>

SOCIALAPI_BASE_URL=https://api.social-api.ai
CHANNEL_PROVISIONING_ENABLED=false
AUTOREPLY_ENABLED=false
LLM_ENABLED=false
```

> `GEMINI_API_KEY` لا يلزم للـsmoke test إذا بقي AutoReply وLLM معطلين، لكن يمكن إضافته لاحقًا عند تفعيل الذكاء الاصطناعي.

## توليد أسرار JWT ومفتاح تشفير AI

شغّل:

```bash
bash scripts/generate-dockhosting-secrets.sh
```

سيولد ملفًا محليًا:

```text
.dockhosting.generated.env
```

لا ترفعه إلى GitHub ولا تشاركه.

## التحقق بعد النشر

اختبر:

```text
GET https://<your-subdomain>.dockhosting.dev/health
GET https://<your-subdomain>.dockhosting.dev/api/v1/health/ready
```

الـ`/health` يجب أن يعيد:

```json
{"status":"ok","service":"mujeeb24-api"}
```

بعد نجاح ذلك، يمكن تفعيل SocialAPI وAI تدريجيًا.
