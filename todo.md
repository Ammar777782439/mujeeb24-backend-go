# Backend readiness — current scope

- [x] تثبيت baseline بعد PR-028 والتأكد من `HEAD == origin/main` ونظافة worktree.
- [x] مراجعة worker lifecycle: graceful shutdown، leases المنتهية، وdead-letter visibility دون retry أعمى.
- [x] مراجعة configuration validation للـproduction: feature flags، HTTPS URLs، وعدم قبول secrets أو live enablement غير الكامل.
- [x] توثيق وإثبات observability للـwebhooks وoutbox وChatwoot mirror وdelivery status.
- [x] إعداد runbook عربي للتشغيل والاستعادة وreconciliation وdead-letter.
- [x] تنفيذ validation شامل محلي وPostgreSQL 16 وOpenAPI/schema/secret scan.
- [ ] توثيق متطلبات موافقة التشغيل الحي بوضوح؛ لا SocialAPI/Chatwoot/Facebook/WhatsApp live ضمن هذه المرحلة.

## Postman API readiness

- [x] جرد OpenAPI وruntime وتصنيف كل endpoint: قابل للاختبار أو يحتاج prerequisite أو غير منفذ.
- [x] إصلاح أول فجوة تمنع اختبار HTTP محليًا بصورة صادقة: JWT Ed25519 + principal/membership + PostgreSQL ScopeProvider.
- [x] إنشاء Postman Collection وEnvironment بلا أسرار مع sequencing للبيانات والـIDs.
- [x] تشغيل API محليًا على PostgreSQL 16 وتنفيذ smoke suite موثق للمصادقة وscope.
- [ ] توثيق endpoints التي تبقى محكومة بتشغيل external live أو auth production.

## إزالة 501 من runtime API

- [x] إكمال مصفوفة كل operation يرجع 501 وربطه بالـaggregate والـapplication handler الموجود أو التنفيذ المطلوب.
- [x] وصل Business profile/policy/dashboard runtime مع PostgreSQL واختبارات HTTP scope.
- [x] وصل Conversations وCustomers وCommunication Message/Outbound Message runtime مع PostgreSQL واختبارات HTTP.
- [x] وصل Channel Connections (read/reconnect/disconnect) وMetrics runtime دون تشغيل provider live.
- [x] إعادة اختبار جميع عمليات OpenAPI على PostgreSQL 16 وتحديث Postman لإزالة folders 501 المكتملة فقط.

## Chatwoot Self-Hosted local stack

- [x] إعداد Docker Compose مع Chatwoot وPostgreSQL وRedis وSidekiq بصورة محلية معزولة.
- [x] إضافة `.env.example` خالٍ من الأسرار مع أوامر توليد secrets محليًا فقط.
- [x] إضافة تعليمات Arabic لتشغيل/تهيئة/إيقاف Chatwoot وربطه مع Mujeeb محليًا دون provider live.
- [ ] التحقق من compose config وبناء الخدمات محليًا دون تثبيت أي بيانات دخول داخل المستودع.
- [x] التحقق من commits المعتمدة محليًا ودفعها إلى `origin/main` بعد مراجعة البعيد.

## SocialAPI webhook exposure

- [x] مراجعة متطلبات webhook داخل لوحة SocialAPI وتسجيل الحقول المطلوبة فقط.
- [x] تجهيز عنوان HTTPS خارجي بديل لـNgrok لاختبار webhook دون أسرار داخل المستودع.
- [x] تشغيل مستقبِل Mujeeb مؤقتًا وتمرير `webhook.test` فقط وفق بروتوكول التسجيل الرسمي، بلا تخزين أو معالجة لحدث الاختبار.
- [x] إدخال endpoint secret الصادر مرة واحدة في runtime آمن فقط، ثم التحقق من HMAC وحدث اختبار دون رسالة أو قناة حية.
- [x] التحقق من الاستقبال الخارجي بتوقيع صحيح وحدث `dm.received`، قبل أي رسالة أو قناة حية.
- [x] إنشاء ChannelConnection وbinding مطابقين في PostgreSQL قبل اختبار materialization الكامل؛ لا يُحل الحدث الخارجي إلى Customer/Conversation بلا tenant/connection scope مثبت.
- [x] تنفيذ اختبار واقعي محدود من `dm.received` موقّع إلى Customer/Conversation/CommunicationMessage وEvent Ledger بعد اكتمال بيانات الربط الداخلية، من دون Chatwoot أو auto-reply أو إرسال خارجي.
- [x] جرد وتشغيل تكامل حي: PostgreSQL وMujeeb وChatwoot وSocialAPI مع تحديد كل credential وbinding مطلوب وعدم افتراض الجاهزية.
- [x] اختبار التزامن الحي بين Mujeeb وChatwoot بعد تهيئة Chatwoot وربط القناة بنجاح.
- [ ] طلب موافقة منفصلة قبل إرسال رسالة اختبار فعلية من حساب SocialAPI أو تشغيل auto-reply/LLM في قناة حية.
- [x] تنفيذ Mujeeb → Chatwoot mirroring كـOutbox job قابل لإعادة المحاولة وidempotent، ثم اختباره قبل إدخال Chatwoot في مسار حي.
- [ ] تثبيت ودفع تغيير فصل Chatwoot mirror عن secret SocialAPI في commit مستقل بعد تحقق Git والأسرار.
- [ ] إضافة bootstrap محلي آمن يولد إعداد Chatwoot على جهاز المستخدم دون رفع أي secret حقيقي إلى Git.
- [x] التحقق من إعداد Cloudflare Tunnel المحدود لمسار SocialAPI webhook ودفعه إلى GitHub.
- [x] إضافة Compose موحّد لتشغيل Mujeeb API وWorker وPostgreSQL مع Chatwoot وRedis دون تثبيت خدمات يدويًا.
- [ ] بناء والتحقق من ملف Compose اختباري موحد يعمل بأمر واحد على Windows Docker Desktop بعد تهيئة ملفه المحلي.
