# Mujeeb 24 Backend Go

Backend مستقل ونظيف لمنصة **مجيب 24**: نظام تشغيل مبيعات ومحادثات متعدد التجار والقطاعات والقنوات، مبني بـGo.

> **لا تفوّت عميلك، ولو كنت مشغولًا.**

## ابدأ من هنا

إذا فتحت المستودع لأول مرة، اقرأ الملفات بهذا الترتيب:

```text
1. docs/project-status-ar.md
2. docs/developer-guide-ar.md
3. docs/decision-log-ar.md
4. contracts/domain_contract_index_ar.md
5. docs/implementation-roadmap-ar.md
6. contracts/
```

هذه الملفات تحتوي على ما تم اعتماده وما لم يبدأ، لذلك لا تحتاج إلى الرجوع إلى المحادثات القديمة لفهم نقطة التوقف.

## مسؤوليات النظام

يمتلك هذا المستودع **Mujeeb Sales Domain** وطبقة التكامل:

- Channel Orchestration بين Provider خارجي وCommunication Workspace.
- حفظ الأحداث الواردة ومنع التكرار وفقدان الرسائل.
- Customer Identity وConversation References.
- Outbound Delivery وDelivery Status وReconciliation.
- AI Context وIntent وStructured Decisions.
- Universal Catalog وOffers وAttributes.
- Leads وOrders وBookings وAppointments وQuotes.
- Dashboard API وTenant Isolation وAudit.

## حدود الأنظمة الخارجية

```text
SocialAPI.ai = Channel Transport
Chatwoot     = Internal Communication Workspace
Mujeeb Go    = Orchestration + AI + Sales Domain
Dashboard    = Merchant Experience
```

التاجر يستخدم Dashboard مجيب 24 فقط. لا نطلب منه فتح Chatwoot أو SocialAPI.ai. Domain لا يعرف Provider DTOs أو Chatwoot Models أو HTTP أو PostgreSQL؛ التفاصيل تكون في Adapters وPlatform.

## المعمارية

```text
internal/
├── bootstrap/
├── domain/
├── application/
│   ├── ports/
│   ├── commands/
│   ├── queries/
│   ├── services/
│   └── workers/
├── adapters/
│   ├── primary/http/
│   └── secondary/
│       ├── providers/socialapi/
│       ├── workspaces/chatwoot/
│       ├── persistence/postgres/
│       ├── queue/asynq/
│       ├── ai/
│       ├── storage/
│       ├── secrets/
│       └── observability/
└── platform/
```

نستخدم **Modular Monolith** في البداية. `cmd/api` و`cmd/worker` و`cmd/migrate` نقاط تشغيل رفيعة، و`bootstrap` يركب الاعتماديات.

## المبادئ غير القابلة للكسر

1. نحفظ Webhook بعد التحقق وقبل ACK.
2. نستخدم `At-Least-Once + Idempotent Processing` بدل ادعاء Exactly-Once خارجيًا.
3. نستخدم Event Ledger وOutbox حتى لا تضيع المهمة بين database commit وqueue publish.
4. لا نعيد إرسال outbound عندما تكون النتيجة `unknown`؛ نستخدم reconciliation.
5. لا نخلط IDs الخاصة بـMujeeb وProvider وChatwoot والقناة الخارجية.
6. لا يملك AI صلاحية اختراع سعر أو مخزون أو تأكيد حجز دون تحقق وتفويض.
7. لا نستخدم Product كجذر عالمي؛ نستخدم CatalogItem وOffer وVariant وAttributes.
8. لا نستخدم Order كمعاملة عامة؛ نستخدم CommercialTransaction بأنواع متعددة.
9. لا نضع tokens أو webhook secrets داخل Domain أو Git.
10. لا نضيف طبقات مكررة إذا كانت `application/ports` أو `adapters/primary` أو `adapters/secondary` تغطي المسؤولية.

## الحالة الحالية

المشروع في مرحلة **Domain Contract وFoundation**. تم إغلاق shared وbusiness وchannel وidentity وcommunication وcatalog وsales وai وaudit كتصميمات موثقة، ولم يبدأ بعد تنفيذ Repositories أو SQL migrations أو Provider APIs أو AI runtime.

الأمر التالي المعتمد هو إغلاق `application/ports`، ثم SQL migrations، ثم Reliability Foundation، ثم Provider Simulator، ثم أول Vertical Slice.

## الاختبار الحالي

```bash
go test ./...
go list ./...
```

نجاح الاختبارات الحالية يثبت قابلية Foundation للبناء فقط، ولا يثبت تكامل SocialAPI.ai أو Chatwoot أو القنوات الحقيقية.

## تشغيل Docker موحّد للاختبار

لتشغيل Mujeeb API وWorker مع PostgreSQL الخاصة بهما، إلى جانب Chatwoot CE وPostgreSQL وRedis الخاصة بها، من أمر Docker Compose واحد ودون تثبيت هذه الخدمات يدويًا، راجع [دليل التشغيل الموحّد](deploy/local/README-ar.md). يبقي هذا المسار الأسرار محلية في `deploy/local/.env` ولا يرفعها إلى Git.

## المستودع

هذا المستودع خاص وموجود على branch `main`:

<https://github.com/Ammar777782439/mujeeb24-backend-go>
