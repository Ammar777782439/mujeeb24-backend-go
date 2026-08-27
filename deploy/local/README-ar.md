# تشغيل Mujeeb 24 وChatwoot بأمر Docker Compose واحد

هذا هو ملف التشغيل **الاختباري الموحّد**. لا تحتاج إلى تثبيت PostgreSQL أو Redis أو Chatwoot أو Go يدويًا؛ تحتاج فقط إلى Docker Desktop مع Docker Compose.

ينشئ الملف حاويات منفصلة للخدمات التالية:

| الخدمة | الدور | الوصول من جهازك |
|---|---|---|
| `mujeeb-postgres` | مصدر حقيقة Mujeeb | داخلي فقط |
| `mujeeb-migrate` | يطبق migrations المضمّنة في binary قبل الإقلاع | داخلي ومؤقت |
| `mujeeb-api` | HTTP API وSocialAPI webhook receiver | `127.0.0.1:3001` |
| `mujeeb-worker` | يعالج Outbox وChatwoot mirror | داخلي فقط |
| `chatwoot-postgres` و`chatwoot-redis` | اعتمادات Chatwoot المعزولة | داخلية فقط |
| `chatwoot-rails` و`chatwoot-sidekiq` | Chatwoot CE ومساراته الخلفية | `127.0.0.1:3002` |
| `webhook-gateway` | proxy يقبل SocialAPI webhook فقط | داخلي فقط |

## التشغيل الأول على Windows Docker Desktop

من جذر المستودع في PowerShell، نفذ هذه الأوامر مرة واحدة:

```powershell
powershell -ExecutionPolicy Bypass -File scripts/init-test-env.ps1
docker compose --env-file deploy/local/.env -f deploy/local/compose.yaml up -d --build
docker compose --env-file deploy/local/.env -f deploy/local/compose.yaml ps
```

يفترض أن تكون الحالات التالية صحيحة قبل اختبار أي تكامل خارجي:

```powershell
curl http://127.0.0.1:3001/health
curl http://127.0.0.1:3002/api
docker compose --env-file deploy/local/.env -f deploy/local/compose.yaml logs --tail=100 mujeeb-api mujeeb-worker chatwoot-rails
```

`mujeeb-migrate` يجب أن ينتهي بـexit code `0`؛ لأنه job وليس خدمة دائمة. أما بقية الخدمات فتظهر `running` أو `healthy` حسب نوعها.

تم التحقق من صياغة Compose وCaddyfile وبناء صورة Mujeeb في بيئة التطوير. لا يمكن لتلك البيئة تشغيل Compose بكامل شبكته الافتراضية بسبب قيد kernel محلي في Docker bridge؛ هذا القيد ليس جزءًا من الملفات المرفوعة ولا ينطبق على Docker Desktop المعتاد. لذلك شغّل الأمر أعلاه عندك ثم افحص `ps` وlogs، ولا تفترض اكتمال التشغيل من بناء الصورة وحده.

## HTTPS مجاني لاختبار SocialAPI فقط

بعد نجاح الـhealth checks شغّل Quick Tunnel داخل **نفس Compose**:

```powershell
docker compose --env-file deploy/local/.env -f deploy/local/compose.yaml --profile quick-tunnel up -d
docker compose --env-file deploy/local/.env -f deploy/local/compose.yaml logs -f quick-tunnel
```

خذ رابط `https://...trycloudflare.com` من السجل، وأضف إليه مسار SocialAPI الحقيقي:

```text
https://...trycloudflare.com/api/v1/webhooks/socialapi/{route_key}
```

الرابط عام ومجاني لكنه مؤقت ويتغير عندما تتوقف حاوية Quick Tunnel. لا يشغل Chatwoot أو Mujeeb outbound تلقائيًا، ولا يجب وضعه في SocialAPI قبل أن يكون `mujeeb-api` شغالًا وأن يكون لديك `route_key` صحيح لا قيمة مخمّنة.

## Named Tunnel برابط ثابت

بعد إنشاء tunnel ونطاق في Cloudflare، ضع token محليًا في `deploy/local/.env` تحت `CLOUDFLARE_TUNNEL_TOKEN` ثم شغّل:

```powershell
docker compose --env-file deploy/local/.env -f deploy/local/compose.yaml --profile named-tunnel up -d
```

لا ترفع `deploy/local/.env`، حتى لو كان المستودع خاصًا. ستحتاج أيضًا إلى إعداد Published application في Cloudflare ليربط hostname الخاص بك بالخدمة `http://webhook-gateway:8080`.

## ما يظل معطّلًا عمدًا

القيم الافتراضية تبقي `CHATWOOT_MIRROR_ENABLED` و`CHATWOOT_AUTOREPLY_ENABLED` و`CHATWOOT_PROVISIONING_ENABLED` و`LLM_ENABLED` معطلة. لكي تشغّل mirroring لاحقًا، أنشئ Chatwoot Account/Inbox وApplication API token ثم ضعه محليًا في `CHATWOOT_API_TOKEN` فقط، مع binding صحيح في Mujeeb. لا تفعّل SocialAPI outbound أو LLM أو auto-reply لمجرد أن الحاويات شغالة.

## الإيقاف وإعادة التهيئة

```powershell
docker compose --env-file deploy/local/.env -f deploy/local/compose.yaml down
```

لإزالة **بيانات الاختبار المحلية فقط** وإعادة البدء من الصفر:

```powershell
docker compose --env-file deploy/local/.env -f deploy/local/compose.yaml down -v
```

## لماذا توجد قاعدتا PostgreSQL؟

`mujeeb-postgres` تملك بيانات Mujeeb التجارية وevent ledger وoutbox. أما `chatwoot-postgres` فتخص مساحة العمل الداخلية لـChatwoot. عدم خلطهما يحافظ على ownership والعزل ويمكّن Mujeeb من البقاء مصدر الحقيقة حتى لو تعطلت Chatwoot.

## مراجع

إعداد Chatwoot الذاتي الاستضافة يحتاج Rails وSidekiq مع PostgreSQL وRedis، ثم `db:chatwoot_prepare` قبل التشغيل. وتوضح Cloudflare أن Quick Tunnel للاختبار فقط، بينما يحتاج النفق الثابت حساب Cloudflare ونطاقًا وtoken محليًا.

[1]: https://www.chatwoot.com/docs/self-hosted/deployment/docker/ "Chatwoot Docker deployment"
[2]: https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/do-more-with-tunnels/trycloudflare/ "Cloudflare Quick Tunnels"
[3]: https://developers.cloudflare.com/tunnel/setup/ "Cloudflare Named Tunnel setup"
