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
- [ ] إنشاء Postman Collection وEnvironment بلا أسرار مع sequencing للبيانات والـIDs.
- [x] تشغيل API محليًا على PostgreSQL 16 وتنفيذ smoke suite موثق للمصادقة وscope.
- [ ] توثيق endpoints التي تبقى محكومة بتشغيل external live أو auth production.
