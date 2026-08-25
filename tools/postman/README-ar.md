# تشغيل Postman محليًا — Mujeeb 24

استورد بالترتيب `Mujeeb24-Local.postman_environment.json` ثم `Mujeeb24-Local.postman_collection.json`. لا يحتوي أي منهما كلمة مرور أو JWT أو refresh token أو مفتاح مزود خارجي.

| متغير | من أين يأتي | هل يُحفظ في Git؟ |
|---|---|---|
| `base_url` | عنوان API المحلي، افتراضيًا `http://127.0.0.1:3001` | نعم، ليس سرًا. |
| `login_email` و`login_password` | principal الذي تنشئه محليًا بأمر bootstrap | لا؛ أدخلهما في البيئة المحلية فقط. |
| `access_token` | يلتقطه اختبار Login تلقائيًا داخل collection | لا؛ collection variable مؤقت. |
| `business_id` | يلتقطه `List accessible businesses` تلقائيًا | لا؛ collection variable مؤقت. |
| `other_business_id` | UUID لنشاط لا توجد للـprincipal عضوية نشطة فيه | اختياري؛ لاختبار 403 فقط. |

## ترتيب الإعداد

شغّل migrations، ثم وفّر `AUTH_ENABLED=true` و`JWT_ISSUER` و`JWT_ACCESS_TTL` و`REFRESH_SESSION_TTL` ومفاتيح Ed25519 Base64 في environment **خارج Git**. لا تنسخ مفاتيح حقيقية إلى هذا الملف أو إلى Collection أو إلى `.env.example`.

بعد إنشاء business، شغّل `cmd/bootstrap-principal` مرة واحدة من environment محلي يحتوي القيم التالية فقط وقت التنفيذ: `DATABASE_URL` و`BOOTSTRAP_EMAIL` و`BOOTSTRAP_PASSWORD` و`BOOTSTRAP_DISPLAY_NAME` و`BOOTSTRAP_BUSINESS_ID` و`BOOTSTRAP_ROLE` و`BOOTSTRAP_PERMISSIONS_JSON`. الأمر يطبع IDs وemail فقط، ولا يطبع password أو JWT keys.

ابدأ API محليًا، ثم نفّذ folders بالترتيب: **01 Public health**، ثم **02 Authentication and identity**، ثم **03 Scoped Catalog**. يعتمد refresh على Postman Cookie Jar؛ لا تحاول نسخ قيمة `mujeeb_refresh` إلى متغير أو حفظها في Collection.

> Folder **Known 501** توثيقي واختباري مقصود. لا تنقل طلباته إلى smoke PASS ولا تغيّر tests لتوقع 200 قبل توصيل dependencies الفعلية في runtime.

## حدود هذه الـCollection

الـCollection لا تشغّل SocialAPI أو Chatwoot أو Facebook/Instagram/WhatsApp أو LLM live، ولا تحتوي webhook signatures. يمكن استيراد OpenAPI المتولد للاطلاع على كل العمليات، لكن التقرير `docs/operations/postman-api-audit-ar.md` هو المرجع لحالة التنفيذ الفعلية.
