# Chatwoot Self-Hosted — بيئة محلية لمجيب 24

هذه البيئة تشغّل **نسخة Chatwoot CE ذاتية الاستضافة** كمساحة عمل داخلية فقط. لها PostgreSQL وRedis خاصان بها داخل volumes مستقلة، ولا تشارك قاعدة بيانات Mujeeb التجارية. يبقى PostgreSQL الخاص بـMujeeb هو مصدر الحقيقة التجاري؛ Chatwoot لا يصبح مصدرًا بديلًا للـCustomers أو Conversations أو Messages.

| الخدمة | المسؤولية | الوصول من الجهاز |
|---|---|---|
| `rails` | واجهة Chatwoot وAPI الداخلية | `http://localhost:3001` افتراضيًا |
| `sidekiq` | أعمال Chatwoot الخلفية | لا منفذ خارجي |
| `postgres` | بيانات Chatwoot فقط | لا منفذ خارجي |
| `redis` | queues/cache لـChatwoot فقط | لا منفذ خارجي |

## إعداد محلي

نفّذ من جذر المستودع. انسخ القالب إلى ملف محلي غير مرفوع، ثم استبدل قيم `REPLACE_WITH...` بقيم مولّدة محليًا وفريدة. لا تستخدم كلمات مرور أو مفاتيح Mujeeb أو SocialAPI أو مفاتيح سابقة.

```bash
cp deploy/chatwoot/.env.example deploy/chatwoot/.env
# عدّل deploy/chatwoot/.env محليًا فقط
docker compose --env-file deploy/chatwoot/.env -f deploy/chatwoot/compose.yaml up -d postgres redis
docker compose --env-file deploy/chatwoot/.env -f deploy/chatwoot/compose.yaml run --rm rails bundle exec rails db:chatwoot_prepare
docker compose --env-file deploy/chatwoot/.env -f deploy/chatwoot/compose.yaml up -d rails sidekiq
curl -I http://localhost:3001/api
```

بعد ظهور خدمة Chatwoot، أنشئ أول مستخدم مسؤول من واجهتها المحلية. يبقى `ENABLE_ACCOUNT_SIGNUP=false` لمنع التسجيل المفتوح بعد ذلك. لا تعرض منفذ `3001` للإنترنت؛ في الإنتاج يوضع خلف reverse proxy مع HTTPS و`FRONTEND_URL=https://chat.example.tld` و`FORCE_SSL=true`.

## أوامر الإدارة المحلية

```bash
# الحالة والسجلات
docker compose --env-file deploy/chatwoot/.env -f deploy/chatwoot/compose.yaml ps
docker compose --env-file deploy/chatwoot/.env -f deploy/chatwoot/compose.yaml logs -f rails sidekiq

# إيقاف stack مع الإبقاء على البيانات المحلية
docker compose --env-file deploy/chatwoot/.env -f deploy/chatwoot/compose.yaml down

# تحديث image مع migrations صريحة بعد مراجعة إصدار Chatwoot
docker compose --env-file deploy/chatwoot/.env -f deploy/chatwoot/compose.yaml pull
docker compose --env-file deploy/chatwoot/.env -f deploy/chatwoot/compose.yaml run --rm rails bundle exec rails db:chatwoot_prepare
docker compose --env-file deploy/chatwoot/.env -f deploy/chatwoot/compose.yaml up -d
```

لا تشغّل `down -v` إلا إذا كنت تقصد حذف بيانات Chatwoot المحلية. ولا تشغّل provisioning أو SocialAPI/Meta live بمجرد قيام هذه البيئة؛ ذلك يحتاج إعدادات مستقلة وموافقة تشغيلية واضحة.

## حدود هذه الدفعة

الملفات تضيف بيئة محلية قابلة للتشغيل والتحقق فقط. لم يتم تشغيل Chatwoot live، ولم تُدخل credentials، ولم يُنشأ Account أو Inbox أو Channel حي. وصل Mujeeb مع Chatwoot يتطلب لاحقًا أن تضبط `CHATWOOT_BASE_URL` وsecret/webhook مخصصين في environment محلي/إنتاجي خارج Git، ثم تتبع اختبارات التكامل المعتمدة.

## مراجع رسمية

اتبعت الملفات تعليمات Docker الرسمية لـChatwoot بشأن `rails db:chatwoot_prepare` وفصل Rails وSidekiq مع PostgreSQL وRedis، ومتغيرات البيئة الرسمية.

[1]: https://developers.chatwoot.com/self-hosted/deployment/docker "Docker Chatwoot Production deployment guide"
[2]: https://developers.chatwoot.com/self-hosted/configuration/environment-variables "Chatwoot Environment Variables"
