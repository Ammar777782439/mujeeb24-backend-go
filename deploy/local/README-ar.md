# التشغيل المحلي: Mujeeb فقط

هذا Compose هو ملف تشغيل اختبار محلي موحّد لـ**Mujeeb 24**. لا يشغّل Chatwoot أو Redis، ولا يحتاج تثبيت PostgreSQL أو Go يدويًا؛ يكفي Docker Desktop مع Docker Compose.

| الخدمة | الدور | الوصول من جهازك |
| --- | --- | --- |
| `mujeeb-postgres` | قاعدة بيانات Mujeeb ومصدر الحقيقة التجاري | داخلي فقط |
| `mujeeb-migrate` | يطبق migrations المضمّنة قبل تشغيل API | داخلي ومؤقت |
| `mujeeb-api` | API واستقبال SocialAPI webhook وإتمام OAuth | `127.0.0.1:3001` |
| `mujeeb-worker` | يستهلك Outbox ويرسل عبر SocialAPI عند تهيئته | داخلي فقط |
| `webhook-gateway` | بوابة محدودة لمساري SocialAPI وOAuth فقط | داخلي فقط |
| `quick-tunnel` / `named-tunnel` | كشف اختياري للبوابة عبر HTTPS | اختياري |

## التشغيل الأول في Windows Terminal (CMD)

من جذر المستودع، انسخ القالب إلى ملف محلي ثم افتحه وعدّل قيمة `MUJEEB_POSTGRES_PASSWORD` إلى كلمة مرور محلية قوية. لا تضع مفاتيح SocialAPI أو LLM في Git.

```cmd
copy deploy\local\.env.example deploy\local\.env
notepad deploy\local\.env
docker compose --env-file deploy\local\.env -f deploy\local\compose.yaml config
docker compose --env-file deploy\local\.env -f deploy\local\compose.yaml up -d --build
docker compose --env-file deploy\local\.env -f deploy\local\compose.yaml ps
curl.exe -sS http://127.0.0.1:3001/health
curl.exe -sS http://127.0.0.1:3001/api/v1/health/ready
```

يجب أن تنتهي خدمة `mujeeb-migrate` بالحالة `Exited (0)` لأنها job قصيرة، بينما يبقى `mujeeb-api` و`mujeeb-worker` و`mujeeb-postgres` قيد التشغيل. عند الحاجة للفحص، استخدم:

```cmd
docker compose --env-file deploy\local\.env -f deploy\local\compose.yaml logs --tail=100 mujeeb-api mujeeb-worker mujeeb-migrate
```

## HTTPS لاختبار SocialAPI وOAuth

بعد نجاح health checks، شغّل النفق المؤقت داخل Compose نفسه:

```cmd
docker compose --env-file deploy\local\.env -f deploy\local\compose.yaml --profile quick-tunnel up -d
docker compose --env-file deploy\local\.env -f deploy\local\compose.yaml logs -f quick-tunnel
```

أضف إلى رابط HTTPS الناتج أحد المسارين الآتيين حسب الغرض:

```text
https://YOUR-HOST/api/v1/webhooks/socialapi/{route_key}
https://YOUR-HOST/oauth/socialapi/callback
```

النفق المؤقت للاختبار فقط، ويتغير عنوانه بعد توقفه. لا تضف مسارات عامة أخرى إلى Caddy بلا قرار أمني مستقل.

## ربط قناة تاجر

عند تفعيل provisioning، ينشئ Mujeeb جلسة OAuth لدى SocialAPI. بعد callback موثّق، يُنشئ أو يفعّل `ChannelConnection` داخل PostgreSQL. لا يوجد Account أو Inbox أو binding خارجي في هذا المسار.

ضع القيم المتاحة محليًا فقط في `deploy\local\.env` ثم أعد بناء API وworker:

```text
CHANNEL_PROVISIONING_ENABLED=true
SOCIALAPI_API_KEY=...
CHANNEL_PROVISIONING_REDIRECT_URI=https://YOUR-HOST/oauth/socialapi/callback
```

## الرد الآلي

الرد الآلي معطل افتراضيًا. عند تفعيله، يبدأ الطلب من SocialAPI webhook الموثّق ثم materialization داخل Mujeeb، ثم Context/Policy/Outbox، ويقوم الـworker بالإرسال إلى SocialAPI. لا تنفذ API أو LLM حيًا بمجرد نجاح تشغيل الحاويات؛ فعّل كل متطلبات runtime محليًا عن قصد واختبرها في بيئة آمنة أولًا.

## الإيقاف

هذا الأمر يوقف الحاويات الخاصة بهذا Compose فقط، ولا يحذف volumes أو قاعدة بيانات Mujeeb:

```cmd
docker compose --env-file deploy\local\.env -f deploy\local\compose.yaml down
```

لا تستخدم أوامر حذف volumes أو قواعد البيانات لإعادة الاختبار من الصفر إلا بعد أخذ نسخة احتياطية ومراجعة أثر الحذف صراحة.

## المراجع

يوضح Cloudflare أن Quick Tunnels مخصص للاختبار، بينما يتطلب الرابط الثابت إعداد Tunnel ونطاق وtoken محليين.[1] [2]

[1]: https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/do-more-with-tunnels/trycloudflare/ "Cloudflare Quick Tunnels"
[2]: https://developers.cloudflare.com/tunnel/setup/ "Cloudflare Named Tunnel setup"
