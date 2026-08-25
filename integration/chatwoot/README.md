# Chatwoot integration environment

هذه البيئة مخصصة لاختبارات التكامل فقط. تستخدم Chatwoot CE مع PostgreSQL وRedis منفصلين عن قاعدة بيانات Mujeeb. القالب المرفوع هو `.env.example`، أما `.env` فيُنشأ محليًا ولا يجوز commit أو upload.

## التشغيل المحلي

```bash
cd integration/chatwoot
cp .env.example .env
# Replace the three local placeholder values with random local values.
sudo docker compose -f docker-compose.integration.yml config
sudo docker compose -f docker-compose.integration.yml run --rm rails bundle exec rails db:chatwoot_prepare
sudo docker compose -f docker-compose.integration.yml up -d
curl -fsS -I http://127.0.0.1:3300/api
```

لا تُعتبر الحاويات جاهزة لمجرد أنها `running`. يجب أن تكون PostgreSQL وRedis healthy وأن يعيد Chatwoot API استجابة ناجحة. للإيقاف بعد الاختبار:

```bash
sudo docker compose -f docker-compose.integration.yml down
```

ولحذف بيانات الاختبار المحلية بالكامل فقط:

```bash
sudo docker compose -f docker-compose.integration.yml down -v
```

## حدود الاختبار

هذا التشغيل لا يخلط Chatwoot PostgreSQL مع PostgreSQL الخاص بـMujeeb. إنشاء Account/User/Inbox/Contact/Conversation/API token يحتاج تهيئة Chatwoot عبر الواجهة أو آلية API/Platform API المناسبة للإصدار المثبت؛ لا تُخترع IDs أو tokens داخل هذا المستودع.
