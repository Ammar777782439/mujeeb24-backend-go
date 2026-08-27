# Cloudflare Tunnel لاستقبال SocialAPI Webhook

يوفر هذا المجلد **proxy محدود المسار** بين الإنترنت وMujeeb. لا يعرّض Chatwoot أو PostgreSQL أو بقية Mujeeb API؛ يسمح فقط بطلبات المسار التالي:

```text
/api/v1/webhooks/socialapi/{route_key}
```

يشترط أن تكون Mujeeb API تعمل على نفس الجهاز وتستمع محليًا على `127.0.0.1:3001`. لا تشغّل API على `0.0.0.0` للاختبار المحلي إلا بعد ضبط جدار الحماية وقرار تشغيل منفصل.

## 1. اختبار مجاني مؤقت: Quick Tunnel

شغّل Mujeeb محليًا أولًا، ثم نفّذ:

```bash
cd deploy/cloudflare-tunnel
docker compose --profile quick up
```

سيظهر في السجل عنوان عشوائي بصيغة `https://...trycloudflare.com`. أضف إليه المسار الحقيقي، مثل:

```text
https://random-name.trycloudflare.com/api/v1/webhooks/socialapi/{route_key}
```

ضع العنوان الكامل في SocialAPI، ثم أرسل **test delivery**. لا تستخدم `{route_key}` ثابتًا أو مخمّنًا؛ يجب أن يطابق binding/connection في Mujeeb.

هذا الخيار عام وHTTPS ومجاني ولا يحتاج حساب Cloudflare أو فتح منفذ في الراوتر، لكنه يتغير عند توقف الحاوية ولا يملك Cloudflare له SLA. لذلك هو للاختبار فقط.

## 2. رابط ثابت على نطاقك: Named Tunnel

يلزم لهذا الخيار حساب Cloudflare ونطاق تمت إضافته إلى Cloudflare. من لوحة Cloudflare أنشئ Tunnel ثم أضف Published application، مثل `hooks.example.com`، واجعل Service URL:

```text
http://webhook-gateway:8080
```

انسخ token الذي تعرضه Cloudflare إلى ملف محلي فقط:

```bash
cd deploy/cloudflare-tunnel
cp tunnel.env.example tunnel.env
# ضع قيمة CLOUDFLARE_TUNNEL_TOKEN في tunnel.env محليًا فقط
docker compose --profile named --env-file tunnel.env up -d
```

بعد أن تصبح حالة tunnel في Cloudflare **Healthy**، يكون Webhook URL:

```text
https://hooks.example.com/api/v1/webhooks/socialapi/{route_key}
```

لا ترفع `tunnel.env` ولا token إلى Git، حتى لو كان المستودع خاصًا. يحتاج `cloudflared` إلى اتصال صادر إلى Cloudflare، ولا يحتاج منفذًا واردًا مفتوحًا في جهازك.

## التحقق والإيقاف

استخدم من جهازك:

```bash
curl -i https://YOUR-HOST/api/v1/webhooks/socialapi/not-a-real-route
docker compose ps
docker compose logs --tail=100 quick-tunnel named-tunnel webhook-gateway
docker compose down
```

نتيجة `404` لمسار غير صحيح تثبت أن proxy لا يكشف بقية Mujeeb API. لا تشغّل auto-reply أو SocialAPI outbound لمجرد نجاح النفق؛ يلزم test delivery موقّع ثم تحقق event ledger وtenant binding أولًا.

## مراجع رسمية

توضح Cloudflare أن Quick Tunnels تنشئ subdomain عشوائيًا على `trycloudflare.com` للاختبار والتطوير فقط، وأنها محدودة إلى 200 طلب متزامن ولا تدعم SSE. أما النفق المُدار فيحتاج حساب Cloudflare ونطاقًا منشورًا وtoken خاصًا بالنفق.

[1]: https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/do-more-with-tunnels/trycloudflare/ "Cloudflare Quick Tunnels"
[2]: https://developers.cloudflare.com/tunnel/setup/ "Cloudflare Tunnel setup"
