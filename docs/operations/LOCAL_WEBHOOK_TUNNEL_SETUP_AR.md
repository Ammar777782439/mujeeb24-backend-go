# البنية المعتمدة لربط الـ Webhook المحلي والنفق الثابت (Local Webhook & Cloudflare Tunnel Architecture)

هذا المستند هو **المرجع التشغيلي الدائم** لكيفية استقبال رسائل وأحداث التواصل الاجتماعي (Facebook, Instagram, WhatsApp) من منصة **SocialAPI** وتوجيهها إلى باك إند **مجيب (Go Backend)** المحلي على جهاز التطوير دون الحاجة لـ Public IP أو فتح منافذ (Port Forwarding) في الراوتر.

---

## 1. الدومين الثابت ونقاط النهاية (Permanent Endpoints)

| الإعداد | القيمة الرسمية المعتمدة |
| :--- | :--- |
| **الدومين العام الثابت** | `https://webhook.mujeeb24.com` |
| **نقطة استقبال الـ Webhooks** | `https://webhook.mujeeb24.com/api/v1/webhooks/socialapi*` |
| **نقطة الـ OAuth Callback** | `https://webhook.mujeeb24.com/oauth/socialapi/callback` |
| **الخدمة المستهدفة داخل Docker** | `http://webhook-gateway:8080` (بوابة Caddy) |
| **الباك إند المحلي** | `http://host.docker.internal:3001` (تطبيق Go على الويندوز) |

> [!IMPORTANT]
> فتح الرابط العام مباشرة `https://webhook.mujeeb24.com/` أو أي مسار خارج المسارين المذكورين أعلاه سيعيد **`404 Not Found` عمداً وبشكل مقصود**. هذا ليس خطأ، بل هو سلوك بوابة Caddy الأمنية لمنع كشف أي مسارات داخلية للإنترنت.

---

## 2. سلسلة نقل البيانات (End-to-End Pipeline)

```text
رسالة العميل على Facebook Messenger
                 ↓
منصة SocialAPI (تلتقط الرسالة وترسل HTTP POST)
                 ↓
الدومين الثابت: https://webhook.mujeeb24.com
                 ↓
Cloudflare Zero Trust (نفق named-tunnel)
                 ↓
اتصال صادر (Outbound Tunnel) إلى حاوية local-named-tunnel-1
                 ↓
بوابة Caddy العازلة: local-webhook-gateway-1 (:8080)
                 ↓ (فحص المسار المسموح به)
باك إند مجيب Go المحلي: host.docker.internal:3001
                 ↓
التحقق من توقيع HMAC-SHA256 عبر SOCIALAPI_WEBHOOK_SECRET
                 ↓
إرجاع HTTP 202 Accepted إلى SocialAPI فوراً
                 ↓
تشغيل مسار المبيعات والرد الآلي (AutoReply & Gemini)
```

---

## 3. وظيفة كل تقنية ومكون في السلسلة

1. **Namecheap:** مزود النطاق الرئيسي (`mujeeb24.com`).
2. **Cloudflare DNS & Tunnel:**
   - يوفر سجل `CNAME` يربط الدومين الفرعي `webhook.mujeeb24.com` بنفق Cloudflare المسمى (`mujeeb24-local`).
   - ينشئ اتصالاً صادراً مشفراً (Outbound) بين حاوية د your local Docker وبين سيرفرات Cloudflare دون الحاجة لأي IP ثابت أو إعدادات راوتر.
3. **حاوية `named-tunnel` (`cloudflared`):**
   - تُشغّل بالرمز السري `CLOUDFLARE_TUNNEL_TOKEN`.
   - تستقبل الطلبات من Cloudflare وتوجهها محلياً إلى `http://webhook-gateway:8080`.
4. **حاوية `webhook-gateway` (Caddy Reverse Proxy):**
   - بوابة حماية وعزل (Zero Trust Isolation Gateway) تستمع على `:8080`.
   - ملف الإعداد الحاكم هو `deploy/local/Caddyfile`:
     ```caddy
     :8080 {
         @socialapi_webhooks path /api/v1/webhooks/socialapi*
         handle @socialapi_webhooks {
             reverse_proxy host.docker.internal:3001
         }

         @socialapi_oauth_callback path /oauth/socialapi/callback
         handle @socialapi_oauth_callback {
             reverse_proxy host.docker.internal:3001
         }

         handle {
             respond "not found" 404
         }
     }
     ```
5. **سيرفر مجيب Go (`:3001`):**
   - يعمل محلياً عبر السكربت `.\scripts\dev-local.ps1 -NoAir`.
   - يستقبل الطلب، ويتحقق من التوقيع، ويسجل الأحداث في PostgreSQL.

---

## 4. إعدادات البيئة في `deploy/local/.env`

تم تثبيت المتغيرات التالية ولا يتم تغييرها:

```env
# رابط الـ Callback الثابت والدائم
CHANNEL_PROVISIONING_ENABLED=true
CHANNEL_PROVISIONING_REDIRECT_URI=https://webhook.mujeeb24.com/oauth/socialapi/callback

# مفتاح النفق الثابت المعتمد
CLOUDFLARE_TUNNEL_TOKEN=<YOUR_NAMED_TUNNEL_TOKEN>
```

---

## 5. أوامر التشغيل والتحقق (Runbook)

### تشغيل النفق الثابت والبوابة:
```powershell
docker compose -f deploy/local/compose.yaml --profile named-tunnel up -d
```

### فحص حالة الحاويات:
```powershell
docker ps --filter "name=local-named-tunnel-1" --filter "name=local-webhook-gateway-1"
```

### فحص سجلات النفق وبوابة Caddy:
```powershell
docker logs local-named-tunnel-1 --tail 50
docker logs local-webhook-gateway-1 --tail 50
```

### تشغيل سيرفر الباك إند المحلي:
```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\dev-local.ps1 -NoAir
```

---

## 6. استكشاف الأخطاء الشائعة (Troubleshooting)

- **ظهور `404 Not Found` عند فتح `https://webhook.mujeeb24.com/`:**
  - هذا السلوك **طبيعي ومقصود 100%** لأن المسار المسموح به فقط هو مسارات `/api/v1/webhooks/socialapi*` و `/oauth/socialapi/callback`.
- **ظهور `502 Bad Gateway`:**
  - معناه أن Caddy يعمل، ولكن سيرفر Go على جهازك متوقف (`:3001`) أو لم يتم تشغيله بعد عبر `dev-local.ps1`.
- **ظهور `202 Accepted` في منصة SocialAPI:**
  - معناه أن سيرفر Go استلم الرسالة بنجاح وبدأ معالجتها داخلياً.
