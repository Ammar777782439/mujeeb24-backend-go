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
| ADR-023 | Dashboard HTTP API Contract مستقل عن Providers | التاجر يرى Mujeeb فقط؛ Webhooks وSocialAPI وChatwoot حدود داخلية منفصلة | معتمد تصميميًا |
| ADR-024 | JWT Access Authentication لـDashboard V1 | Token قصير العمر، Business Scope من Membership لا من Claims، وفصل Auth عن Providers | معتمد تصميميًا |
| ADR-025 | Catalog AI Projection v1 — Read Model لـ AI فقط، attribute_schemas[] + items[] مع variants+offers nested، لا business_id/SQL/search/semantic | العقود ① ⑤ تحسم الشكل النهائي وتمنع اختلاط Entity Contract ببيانات التاجر | مغلق |
| ADR-026 | Catalog Evaluation + Batching — Token-based، Mujeeb يضمن Coverage، Gemini يرجع candidates فقط، لا previous_interaction_id بين Batches | العقد ② يمنع semantic search وعدد ثابت للـItems في Batch | مغلق |
| ADR-027 | Conversation Context Contract — Mujeeb = canonical conversation state، Gemini = previous_interaction_id، ConversationState = focus/previous/comparison/preferences/constraints/pending/version | العقد ③ يمنع استخدام Gemini كمخزن للحقيقة | مغلق |
| ADR-028 | Gemini System + AI I/O Contract — Output = status+action+response_text+selected، Actions = answer/clarification/human_request/lead_draft/order_draft، لا requires_approval من Gemini | العقد ④ يجعل PolicyEvaluator هو صاحب القرار النهائي | مغلق |
| ADR-029 | AI Validation + Authorization Boundary — Structural→Reference→Tenant→Ownership→Policy→Authorization→Effective Decision، لا semantic re-matching داخل Mujeeb، لا AI ثانٍ للتحقق من AI | العقد ⑥ يفصل Effective Decision عن AI Proposal | مغلق |
| ADR-030 | Observability + Audit + AI Trace — ثلاث طبقات منفصلة، ai_run_id يربط Business→Conversation→Message→Run→Interaction→ToolCall→Validation→Auth→Execution | العقد ⑧ يمنع وضع Metrics داخل Domain ويمنع أسرار داخل Trace | مغلق |
| ADR-031 | AI Runtime Lifecycle — RECEIVED→CONTEXT_BUILT→RUNNING→WAITING_TOOL→VALIDATING→AUTHORIZED→EXECUTING→COMPLETED/FAILED/CANCELLED، Retryable vs Non-Retryable، لا رقم ثابت للـRetries في Domain | العقد ⑨ يجعل Retry/Backoff/Timeout مسؤولية Runtime فقط | مغلق |
| ADR-032 | Merchant Catalog AI Authoring v2 — وكيل محادثي مستقل يبني CatalogOperationProposal (create/update/delete)، يستبدل AIAuthoringProposal/GenerateCatalogDraft القديمة | العقد 11 يلغي v1 ويفرض فصل Customer Sales AI عن Merchant Catalog AI | مغلق |
| ADR-033 | Breaking change على ai_decisions.requested_action — استبدال القيم القديمة بـ answer/clarification/human_request/lead_draft/order_draft وفقًا للعقد ④ | العقد ④ يفرض قيمًا محددة فقط؛ لا حاجة للتوافق التشغيلي القديم | مغلق |
| ADR-034 | AI Run + Attempt + Tool Call + Gemini Interaction + Catalog Batch + Usage Telemetry — جداول تشغيلية منفصلة عن ai_decisions التجارية | العقود ⑧ ⑨ تفرض الفصل بين الـRun والقرار التجاري | مغلق |

## قرارات لم تُحسم بعد

هذه ليست فجوات يجب ملؤها بالتخمين:

```text
- SocialAPI test credentials وcapabilities الفعلية
- Native Custom Channel stability مع Chatwoot
- Exact AI model/provider وpricing
- Subscription billing provider
- Auth provider وUser/Membership storage وPassword/SSO policy؛ أما JWT transport وPrincipal وBusiness Scope وRole/Permission فهي ثابتة في HTTP API Contract
- Retention policy للرسائل والوسائط
- التحقق الحقيقي لقنوات Facebook/Instagram/WhatsApp
```

## قاعدة تغيير القرار

لا نغير ADR معتمدًا لأن ملفًا خارجيًا اقترح شجرة مختلفة. نغيره فقط عند ظهور: اختبار فاشل، قيد Provider موثق، مشكلة تشغيل، متطلب تجاري جديد، أو تكلفة/مخاطر مثبتة.
