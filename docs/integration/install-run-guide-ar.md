# دليل تثبيت وتشغيل Mujeeb 24 وChatwoot محليًا

## أولًا: الحقيقة الحالية

المستودع يحتوي على الكود وملفات Compose والقوالب الآمنة، لكنها ليست حزمة تثبيت بضغطة واحدة. بيئة الاختبار التي شغّلت Chatwoot وPostgreSQL وRedis كانت داخل بيئة عمل مؤقتة، وقد أُوقفت وحُذفت بعد انتهاء الاختبار. لذلك ستحتاج إلى تثبيت Docker وGo وPostgreSQL/Compose على جهازك أو خادمك.

كذلك يجب الانتباه إلى أن المشروع ليس E2E إنتاجيًا كاملًا بعد. Chatwoot API وGo adapter وwebhook verification وEventStore وOutbox storage مثبتة، لكن mapping الدائم بين Mujeeb وChatwoot، وتنفيذ worker الخارجي، وSocialAPI outbound، واستقبال SocialAPI webhook عبر HTTPS عام ما زالت أعمالًا غير مغلقة.

## ثانيًا: إذا كان جهازك هاتفًا فقط

الهاتف ليس البيئة المناسبة لتشغيل Chatwoot مع Docker وPostgreSQL وGo API بشكل مستقر. الحل العملي هو استخدام حاسوب، أو خادم Linux، أو بيئة تطوير سحابية تملك Docker. يمكنك استخدام الهاتف لإدارة GitHub ومراقبة الخادم، لكن الخادم نفسه يجب أن يبقى متصلًا عند تشغيل الخدمات.

## ثالثًا: المتطلبات على حاسوب أو خادم Linux

ثبّت Git وGo 1.22 أو أحدث متوافقًا مع `go.mod`، وثبّت Docker Engine مع Docker Compose plugin. بعد ذلك نفّذ:

```bash
git clone https://github.com/Ammar777782439/mujeeb24-backend-go.git
cd mujeeb24-backend-go
go version
docker compose version
```

لا تنسخ أي token أو password إلى ملفات داخل المستودع. أنشئ ملفات البيئة محليًا فقط، واجعل صلاحياتها `600`، وتأكد أنها غير متتبعة في Git.

## رابعًا: تشغيل Chatwoot Self-Hosted للاختبار المحلي

```bash
cd integration/chatwoot
cp .env.example .env
```

افتح `.env` محليًا واستبدل القيم الثلاث التجريبية فقط بقيم عشوائية محلية: `SECRET_KEY_BASE` و`POSTGRES_PASSWORD` و`REDIS_PASSWORD`. لا ترفع `.env` إلى GitHub ولا ترسله لأحد. بعد ذلك شغّل:

```bash
sudo docker compose -f docker-compose.integration.yml config
sudo docker compose -f docker-compose.integration.yml run --rm rails bundle exec rails db:chatwoot_prepare
sudo docker compose -f docker-compose.integration.yml up -d
curl -fsS -I http://127.0.0.1:3300/api
sudo docker compose -f docker-compose.integration.yml ps
```

لا تعتبر Chatwoot جاهزًا بمجرد ظهور الحالة `running`. يجب أن يعيد `/api` استجابة ناجحة، وأن تكون PostgreSQL وRedis في حالة healthy. افتح `http://127.0.0.1:3300`، وأنشئ حساب اختبار محليًا وAccount وAPI Inbox. استخرج API token من حساب الاختبار فقط، ولا تضعه في Git أو التقرير.

بعد الانتهاء من اختبار Chatwoot، احذف بيانات الاختبار إذا لم تعد تحتاجها:

```bash
sudo docker compose -f docker-compose.integration.yml down -v
rm -f .env
```

## خامسًا: تشغيل PostgreSQL الخاص بـMujeeb

يجب أن تكون قاعدة Mujeeb منفصلة عن قاعدة Chatwoot. يمكنك استخدام PostgreSQL موجود عندك، أو تشغيل حاوية مستقلة باسم مختلف. لا تستخدم قاعدة Chatwoot لتشغيل migrations الخاصة بـMujeeb.

بعد تجهيز قاعدة Mujeeb، عرّف `DATABASE_URL` في جلسة التشغيل أو في ملف بيئة محلي خارج Git. الشكل العام هو:

```bash
export DATABASE_URL='postgres://USER:PASSWORD@HOST:PORT/mujeeb24?sslmode=disable'
export HTTP_ADDR=':3001'
export CHATWOOT_BASE_URL='http://127.0.0.1:3300'
export CHATWOOT_API_TOKEN='ضع token اختبار Chatwoot في بيئة التشغيل فقط'
export CHATWOOT_WEBHOOK_SECRET='ضع secret webhook المحلي في بيئة التشغيل فقط'
```

السطر السابق توضيحي فقط؛ لا تحفظ القيم الحقيقية في المستودع ولا تستبدلها بقيم ظهرت في محادثة سابقة. إذا لم يتوفر Chatwoot token أو webhook secret محليًا، اترك تكامل Chatwoot معطلًا بدل اختراع قيمة.

طبّق migrations ثم شغّل API:

```bash
cd /path/to/mujeeb24-backend-go
go run ./cmd/migrate
go run ./cmd/api
```

في نافذة أخرى شغّل worker:

```bash
cd /path/to/mujeeb24-backend-go
go run ./cmd/worker
```

تحقق من API:

```bash
curl -i http://127.0.0.1:3001/api/v1/health/live
curl -i http://127.0.0.1:3001/api/v1/health/ready
```

## سادسًا: تشغيل اختبار webhook المحلي

بعد تشغيل API وتعريف `CHATWOOT_WEBHOOK_SECRET` في جلسة التشغيل فقط، نفّذ من جذر المستودع:

```bash
export CHATWOOT_WEBHOOK_SECRET='secret المحلي فقط'
integration/chatwoot/verify-webhook.sh
unset CHATWOOT_WEBHOOK_SECRET
```

النتيجة المتوقعة لاختبار boundary هي `202` للتوقيع الصحيح و`401` للتوقيع غير الصحيح. هذا الاختبار يثبت التحقق من التوقيع والـHTTP boundary فقط؛ لا يثبت بعد إنشاء Customer أو Conversation أو Message داخل Mujeeb.

## سابعًا: ما الذي لا تشغله الآن

لا تشغّل SocialAPI outbound أو Facebook outbound على حساب حقيقي. يلزم credential محلي صالح وهدف اختبار آمن ومحدد قبل أي إرسال. كما أن webhook الخارجي يحتاج عنوان HTTPS عامًا، وهو غير موجود في التشغيل المحلي. لذلك ستبقى هذه الأجزاء blocked حتى تجهيز شروطها.

## ثامنًا: أوامر الإيقاف والتنظيف

```bash
# إيقاف API وworker من النوافذ التي يعملان فيها: Ctrl+C
cd integration/chatwoot
sudo docker compose -f docker-compose.integration.yml down -v
rm -f .env
cd ../..
git status --short
```

يجب أن تكون شجرة Git نظيفة، وألا يظهر `.env` أو أي token أو log. لا تنفذ `down -v` على مشروع Docker آخر، لأن ذلك يحذف volumes الخاصة بالمشروع المستهدف.
