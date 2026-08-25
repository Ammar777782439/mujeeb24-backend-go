# Backend readiness — current scope

- [x] تثبيت baseline بعد PR-028 والتأكد من `HEAD == origin/main` ونظافة worktree.
- [x] مراجعة worker lifecycle: graceful shutdown، leases المنتهية، وdead-letter visibility دون retry أعمى.
- [x] مراجعة configuration validation للـproduction: feature flags، HTTPS URLs، وعدم قبول secrets أو live enablement غير الكامل.
- [x] توثيق وإثبات observability للـwebhooks وoutbox وChatwoot mirror وdelivery status.
- [x] إعداد runbook عربي للتشغيل والاستعادة وreconciliation وdead-letter.
- [x] تنفيذ validation شامل محلي وPostgreSQL 16 وOpenAPI/schema/secret scan.
- [ ] توثيق متطلبات موافقة التشغيل الحي بوضوح؛ لا SocialAPI/Chatwoot/Facebook/WhatsApp live ضمن هذه المرحلة.
