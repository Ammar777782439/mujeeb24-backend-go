# Mujeeb 24 Backend Go

Backend مستقل ونظيف لمنصة **مجيب 24**: نظام تشغيل مبيعات ومحادثات للتجار، مبني بـGo، ويدعم تعدد التجار والقنوات والقطاعات.

> لا تفوّت عميلك، ولو كنت مشغولًا.

## مسؤوليات النظام

يمتلك هذا المستودع **Mujeeb Sales Domain** وطبقة التكامل:

- Channel Orchestration بين Provider خارجي وChatwoot.
- حفظ الأحداث الواردة ومنع التكرار وفقدان الرسائل.
- Customer Identity وConversation References.
- Outbound Delivery وDelivery Status وReconciliation.
- AI Context وIntent وStructured Decisions.
- Commercial Catalog وOffers وAttributes.
- Leads وOrders وBookings وAppointments وQuotes.
- Dashboard API وTenant Isolation وAudit.

## حدود الأنظمة الخارجية

```text
SocialAPI.ai = Channel Transport
Chatwoot     = Internal Conversation Workspace
Mujeeb Go    = Orchestration + AI + Sales Domain
Dashboard    = Merchant Experience
```

Domain لا يعرف أي كائنات خاصة بمزود خارجي أو Workspace داخلي. هذه التفاصيل محصورة داخل adapters.

## التشغيل الأولي

المشروع في مرحلة **Foundation**. نستخدم Modular Monolith منظمًا، مع عمليتي `api` و`worker` من نفس المستودع. Redis/Asynq مسرّع للمهام، وليس مصدر الحقيقة الوحيد. PostgreSQL يعتمد على SQL migrations versioned؛ لا نستخدم `AutoMigrate` في الإنتاج.

## الهيكل

انظر إلى:

- `docs/architecture.md`
- `contracts/`
- `api/openapi.yaml`
- `migrations/`

## المبادئ غير القابلة للكسر

1. نحفظ Webhook بعد التحقق وقبل ACK.
2. نستخدم `At-Least-Once + Idempotent Processing` بدل ادعاء Exactly-Once خارجيًا.
3. نستخدم Outbox حتى لا تضيع المهمة بين database commit وqueue publish.
4. لا نعيد إرسال outbound عندما تكون النتيجة `unknown`؛ نستخدم reconciliation.
5. لا نخلط IDs الخاصة بـMujeeb وProvider وChatwoot والقناة الخارجية.
6. لا يملك AI صلاحية اختراع سعر أو مخزون أو تأكيد حجز دون تحقق وتفويض.
7. التاجر يستخدم Dashboard مجيب 24 ولا يحتاج إلى فتح Chatwoot أو SocialAPI.ai.

## الحالة الحالية

هذا المستودع لا ينسخ Prototype القديم. الخطوة التنفيذية الأولى هي إغلاق عقود القنوات، ثم بناء Provider Simulator وVertical Slice من رسالة واردة إلى رد خارجي وحالة تسليم، قبل إضافة AI التجاري الكامل.
