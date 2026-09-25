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
| ADR-035 | Customer Sales System Prompt v1→v2 — إضافة "القاعدة الذهبية" (التركيز الإجباري على الاسم + السعر + التوفر)، قاعدة احترام business_policy_evidence، وقاعدة مقاومة التشتيت (anti-jailbreak) | ملاحظات التاجر: الـ AI ما يركز على المنتج/السعر/التوفر، والعميل يقدر يشتت الـ AI؛ الـ context builder أصلاً يمرر offer_evidence و business_policy_evidence لكن الـ prompt v1 ما يأمر بتطبيقها | مغلق |
| ADR-036 | Customer Sales Prompt v2→v3 + Final Evaluation Prompt v1→v2 — 6 إصلاحات بناءً على اختبار حي: (1) منع التبديل الصامت للمنتج (iPhone 15→16)، (2) عدم الثقة بأقوال العميل ("دعم العملاء قال متوفر")، (3) منع بدء الرد باسم المنتج وحده، (4) توضيح pricing_mode كـ metadata لا كـ "خيارات دفع"، (5) منع إطلاق catalog batch لأسئلة القواعد، (6) Final Evaluation Prompt يطبق قواعد v3 | اختبار حي كشف إن v2 ما يطبّق على Final Evaluation لإنها تستعمل prompt منفصل؛ لازم تحديث كل من prompt النداء الأول وFinal معًا | مغلق |
| ADR-037 | Customer Sales Prompt v3→v4 + Final Evaluation Prompt v2→v3 — إضافة قاعدة فهم نية العميل والتسامح مع الكتابة الضعيفة: تطبيع اللهجة الخليجية ("وش", "كم", "بغيت", "عندكم")، تسامح مع الأخطاء الإملائية (س/ث، ه/ة، ى/ي)، فهم النية من السياق (الرسائل القصيرة والمبتورة)، فهم الرسائل المبعثرة، قاعدة عدم الفهم الزائد (anti-over-interpretation) | التاجر أكد إن العميل غالبًا يكتب بعجلة وبعامية وبأخطاء إملائية، ولازم Gemini يفهم النية لا الكلمات الحرفية | مغلق |
| ADR-038 | Customer Sales Prompt v4→v5 + Final Evaluation Prompt v3→v4 + MaxMessages 8→6 — تطبيق أفضل الممارسات من بحث الإنترنت (Microsoft Learn + getmaxim.ai + IrisAgent + mem0.ai + Oracle blogs) لإدارة سياق المحادثة: (1) قاعدة منع التكرار الإشاري (ممنوع "أجبناك سابقاً"، "كما ذكرت سابقًا")، (2) قاعدة عرض البديل القريب باحترام ("لا، ليس لدينا X. لكن لدينا Y بسعر...")، (3) قاعدة معاملة رسائل الـ AI السابقة كـ context للفهم لا للنسخ، (4) تقليل MaxMessages من 8 إلى 6 (Sliding Window threshold حسب IrisAgent) | اختبار حي كشف إن AI قال "أجبناك سابقاً بأن iPhone 15 غير متوفر" لما العميل أعاد السؤال، مما أزعج العميل وكسر تدفق المحادثة. بحث الإنترنت أكد إن هذا سلوك شائع يجب منعه prompt-level. Summary+Sliding Window الكامل (طبقة 3) متروك لـ ADR-039 مستقبلي. | مغلق |
| ADR-039 | بناء ConversationSummaryService (الطبقة 3 المعمارية) — تنفيذ كامل لـ "Summary + Sliding Window Hybrid" pattern من Microsoft Learn: (1) migration 000058 — إضافة summary + summary_turn_count لجدول conversation_state، (2) ConversationSummaryService جديدة — تولّد ملخص كل 4 أدوار عبر Gemini، (3) تحديث AIContext لإضافة ConversationSummary field، (4) تحديث ai_context_builder لتمرير summary من conversation_state، (5) تحديث promptContext لإرسال conversation_summary كحقل JSON منفصل، (6) ربط MaybeSummarize في auto_reply flow (goroutine منفصل 30s timeout)، (7) ترقية CustomerSalesSystemPrompt إلى v6 بإضافة قاعدة استخدام conversation_summary | التاجر طالب بحل إنتاجي حقيقي وليس برمبتات. بحث الإنترنت أثبت إن Summary+Sliding Window هو النمط الذهبي الموصى به من Microsoft Learn + getmaxim.ai + IrisAgent + mem0.ai. هذا الحل يقلل tokens 50% للمحادثات الطويلة ويحافظ على السياق القديم. | مغلق |
| ADR-040 | إضافة comprehensive logging لـ MerchantCatalogAIAgent (B2B flow) — قبل: 0 log statements في الـ agent + 0 في HTTP handler. بعد: 16 log statement تشمل: START, REJECTED (3 أسباب), SESSION_CREATED, SESSION_CREATE_FAILED, MESSAGE_PERSIST_FAILED, STATE→RUNNING, RUN_START_FAILED, CONTEXT_BUILT, CONTEXT_BUILD_FAILED, GEMINI_CALL, GEMINI_OK (مع tokens + latency + response preview), GEMINI_FAILED, STATE→VALIDATING, VALIDATION_OK/FAILED, OPERATION (مع counts), ASSISTANT_PERSIST_FAILED, COMPLETED + LIFECYCLE_MARK_FAILED في كل مرحلة. + 4 logs في MerchantAIHandler: REQUEST/RESPONSE/ERROR/REJECTED. | التاجر اشتكى إن B2B (MerchantCatalogAIAgent) ما عنده logs واضحة مثل B2C (AutoReply). تماثل مع نمط الـ logging اللي طبّقناه في B2C flow. كل خطأ وكل مرحلة الآن موثّقة. | مغلق |
| ADR-041 | Catalog Selection الحتمي (deterministic) لـ B2B MerchantCatalogAIAgent — يفصل اختيار الكتالوج عن الـ AI تمامًا. الأولوية: (1) HTTP param target_catalog_id من الـ dashboard dropdown، (2) session sticky target_catalog_id المخزن في merchant_ai_sessions، (3) auto-select لو فيه كتالوج واحد فقط، (4) لما فشلت كل الطبقات الـ 3 → الكود يحوّل العملية لـ ask_merchant + يعرض قائمة الكتالوجات من catalog_evidence. ممنوع: AI inference للكتالوج من رسالة التاجر، fuzzy matching، اختراع catalog_id. الـ AI دوره فقط: (أ) معرفة كل الكتالوجات عبر catalog_evidence، (ب) صياغة سؤال التاجر بالعربي بناءً على أوامر الكود، (ج) الإجابة على أسئلة معلوماتية مثل "كم عندي كتالوج؟". | التاجر عنده أكثر من كتالوج والـ AI الحالي ما يعرف كيف يختار. الـ proposal يطلع بـ catalog_id فاضي فيفشل INSERT. الحل الإنتاجي: code deterministic selection — الـ AI ما يختار أبدًا، الكود يختار أو يطلب من التاجر التحديد صراحةً. هذا يمنع الـ hallucination ويضمن production-grade + scalability + maintainability. | مغلق |
| ADR-042 | B2B/B2C System Prompt Isolation — إنشاء MerchantCatalogSystemPrompt v1 المستقل (مشترك مع CustomerSalesSystemPrompt ممنوع). العقد 11 §2 يفرض إن B2B و B2C ما يشاركون system prompt / agent role / tool permissions / conversation purpose / proposal contract / execution workflow. قبل: bootstrap يستعمل نفس gemini.Client (اللي يستعمل CustomerSalesSystemPrompt by default) لكلا الـ flow. بعد: bootstrap يخلق gemini.Client ثاني مع SystemPrompt=MerchantCatalogSystemPrompt ويلفه بـ ContractClient ثاني. إضافة [CREATE]/[UPDATE]/[DELETE] prefix protocol في response_text عشان mapGeminiProposalToOperation يفصل informational answer عن mutation proposal (إصلاح bug في ADR-040). إضافة target_catalog_id في frontend DTO (MerchantAIChatRequest) + propagation عبر use-merchant-ai hook + merchant-ai-sheet activeCatalogId prop + catalog page. | اختبار حي كشف إن الـ B2B agent يرد على التاجر بـ "أهلاً بك في متجرنا" كأنه عميل نهائي بدل ما يتعامل معه كتاجر يدير كتالوجاته. هذا لأنه يستعمل CustomerSalesSystemPrompt. العقد 11 §2 يفرض الاستقلالية التامة. | مغلق |
| ADR-043 | إصلاح Idempotency Key Collision في B2B MerchantAI flow — قبل: handler يولّد idempotency_key قبل إنشاء الـ session، فيصير "merchant-ai::سلام" لكل دور أول في الـ message نفسه. لما التاجر يرسل "سلام" مرتين في session مختلفة (أو بعد restart)، المفتاح يتعارض مع run قديم مكتمل، فيرجّعه الـ StartRun (status=completed)، ثم HandleTurn يحاول MarkContextBuilt على run مكتمل → "illegal transition: completed → context_built" per contract ⑨ §26. بعد: (1) handler ما يولّد fallback key — يمرّر IdempotencyKey فقط من header؛ (2) HandleTurn يولّد المفتاح بعد إنشاء الـ session بـ "merchant-ai:<resolved_session_id>:<message>" — sessions مختلفة = مفاتيح مختلفة؛ (3) StartRun يفحص status الـ run الموجود: لو terminal (completed/failed/cancelled) ما يرجّعه، ينشئ واحد جديد؛ (4) migration 000060 يحوّل القيد من full UNIQUE إلى partial UNIQUE (status NOT IN terminal) — عشان INSERTs بعد terminal runs ما تتعارض. | اختبار حي كشف إن التاجر ما يقدر يرسل نفس الكلمة في session جديدة (أو بعد restart) دون خطأ. هذا كسر لإنتاجية الـ B2B flow. | مغلق |
| ADR-044 | B2B Merchant Catalog Proposal Pipeline — 3 طبقات إنتاجية: (1) prefix strip: شيل [CREATE]/[UPDATE]/[DELETE] من response_text قبل عرضه للتاجر في الشات (البريفكس داخلي للتشخيص فقط)؛ (2) structured proposal: إضافة `proposal` field لـ AIGeminiProposal + response_schema يخلي Gemini يرجّع بنية كاملة (item + variants + offers) مش مجرد نص — الـ frontend يعرضها كـ card مع زر موافقة/رفض، لما التاجر يوافق ينفّذ الـ INSERT فعليًا؛ (3) multi-turn data gathering: Gemini يفحص الحقول الناقصة في الـ Entity Contract (item_type, pricing_mode, availability_mode, fulfillment_mode) قبل ما يقول "المسودة جاهزة"، يرجّع status=needs_more_data + MissingFields[] بالحقول الناقصة — الـ frontend يعرضها كـ form بدل ما يطلب من التاجر يكتب كل شي نصياً. | اختبار حي كشف 3 ثغرات: (أ) البريفكس [CREATE] ظاهر للتاجر في الشات؛ (ب) الـ AI قال "تم تجهيز مسودة" بس ما رجّع بنية البيانات (Item/Variants/Offers) للتاجر يوافق عليها؛ (ج) الـ AI ما سأل عن الحقول الناقصة في الـ Entity Contract قبل تجهيز المسودة. | مغلق |

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
