# سجل القرارات — Mujeeb 24 Backend Go

## طريقة الاستخدام

هذا السجل يمنع إعادة فتح قرارات حُسمت أو تغيير المعمارية بسبب اقتراح عام. أي تغيير لاحق يجب أن يضيف قرارًا جديدًا يشرح المشكلة والدليل والتأثير.

| رقم | القرار | السبب | الحالة |
|---|---|---|---|
| ADR-001 | Go Backend مستقل عن Postiz | Prototype القديم لا يمثل حدود المنتج الجديد | معتمد |
| ADR-002 | Modular Monolith في البداية | نحتاج سرعة وحدود واضحة دون Microservices مبكرة | معتمد |
| ADR-003 | Dashboard التاجر داخل Mujeeb فقط | Chatwoot وSocialAPI خدمات خلفية لا يراها التاجر | معتمد |
| ADR-004 | SocialAPI Provider، وليس Domain | قابلية الاستبدال بـMeta Direct مستقبلًا | معتمد |
| ADR-005 | Chatwoot Communication Workspace Sidecar | يوفر Inbox/Contacts/Conversations/Assignments دون امتلاك Sales Truth | معتمد مشروط |
| ADR-006 | API Channel/Middleware قبل Native Custom Channel | Custom Channel الأصلي يحتاج تطوير وصيانة داخل Chatwoot | معتمد |
| ADR-007 | Mujeeb Domain يملك Sales Truth | Leads وCatalog وAI Decisions وTransactions ليست ملك Chatwoot | معتمد |
| ADR-008 | Typed Internal IDs | منع خلط BusinessID وCustomerID وProvider IDs | معتمد |
| ADR-009 | ExternalIdentity scoped بـBusiness وConnection | external user IDs ليست global | معتمد |
| ADR-010 | عدم الدمج بالاسم | منع كشف بيانات عميل أو دمج شخصين خطأً | معتمد |
| ADR-011 | Universal Catalog | دعم الإلكترونيات والسفر والخدمات دون Product ضيق | معتمد |
| ADR-012 | CommercialTransaction متعدد الأنواع | Order ليس مناسبًا للحجز والموعد وطلب الخدمة | معتمد |
| ADR-013 | AI Decision Structured ومحدود | LLM ليس Executor ولا مصدر سعر/مخزون | معتمد |
| ADR-014 | At-Least-Once + Idempotency | Exactly-Once غير مضمون خارجيًا | معتمد |
| ADR-015 | Event Ledger + Outbox قبل AI | منع فقد الأحداث والمهام بين DB وQueue | معتمد |
| ADR-016 | UNKNOWN + Reconciliation | عدم تكرار الرسالة بعد نتيجة Provider غامضة | معتمد |
| ADR-017 | PostgreSQL + pgx + SQL migrations | وضوح عقود Multi-tenancy وLedger وOutbox | معتمد |
| ADR-018 | Asynq + Redis في V1 | كافٍ للـworkers والـretry قبل الحاجة إلى Kafka/RabbitMQ | معتمد مبدئيًا |
| ADR-019 | Ports داخل application | منع التكرار بين `internal/ports` و`application/ports` | معتمد |
| ADR-020 | Primary/Secondary Adapters | توضيح اتجاه الدخول والاعتماد الخارجي | معتمد |
| ADR-021 | Bootstrap مستقل | منع تضخم `cmd/main.go` بتركيب كل dependencies | معتمد |
| ADR-022 | Domain Review قبل Implementation | الكود يجب أن يترجم تصميمًا مغلقًا لا يخترعه | معتمد |

## قرارات لم تُحسم بعد

هذه ليست فجوات يجب ملؤها بالتخمين:

```text
- SocialAPI test credentials وcapabilities الفعلية
- Native Custom Channel stability مع Chatwoot
- Exact AI model/provider وpricing
- Subscription billing provider
- Auth/Business Membership contract النهائي
- Retention policy للرسائل والوسائط
- التحقق الحقيقي لقنوات Facebook/Instagram/WhatsApp
```

## قاعدة تغيير القرار

لا نغير ADR معتمدًا لأن ملفًا خارجيًا اقترح شجرة مختلفة. نغيره فقط عند ظهور: اختبار فاشل، قيد Provider موثق، مشكلة تشغيل، متطلب تجاري جديد، أو تكلفة/مخاطر مثبتة.
