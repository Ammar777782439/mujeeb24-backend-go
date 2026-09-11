# تعليمات وكيل البرمجة (Agent Guidelines) — Mujeeb 24 Backend

هذا الملف هو **المرجع التشغيلي الإلزامي الأول** لأي Coding Agent يعمل على Backend مشروع **Mujeeb 24**.
المشروع مبني بلغة **Go 1.22** وقاعدة بيانات **PostgreSQL 16** وفق نمط **Modular Monolith (Hexagonal / Clean Architecture)**.

---

## 1. إجراءات بدء أي مهمة (Agent Startup Checklist)

قبل أي قراءة تحليلية أو تعديل أو تنفيذ لأي مهمة، التزم بالخطوات العشر الآتية بالترتيب:

1. **اقرأ هذا الملف (`AGENTS.md`) بالكامل.**
2. **اقرأ الوثيقة المرجعية الشاملة:** [docs/architecture/chatwoot-free-ai-handoff-prompt-ar.md](file:///D:/mujeeb24/mujeeb24-backend-go/docs/architecture/chatwoot-free-ai-handoff-prompt-ar.md).
3. **اقرأ وثائق البدء وحالة العمل إن وجدت:** `docs/engineering/START_HERE.md` و`docs/engineering/WORK_IN_PROGRESS.md` ومستند [todo.md](file:///D:/mujeeb24/mujeeb24-backend-go/todo.md).
4. **افحص حالة Git الحالية:** تأكد من أنك على الفرع المعتمد `feat/chatwoot-free` وأن شجرة العمل نظيفة (`git status --short`).
5. **حدد نطاق المهمة (Task Scope):** تأكد من أن المطلوب يخص الـBackend فقط، ولا تلمس الـFrontend أو أنظمة الدفع أو الشحن.
6. **حدد العقود المتأثرة:** راجع العقود المغلقة في هذا الملف وتأكد من عدم كسر أي قيد ثابت.
7. **استخدم الأدوات المخصصة فعلياً:**
   - استخدم `codebase-memory-mcp` لفهم الروابط المعمارية ومسارات الاستدعاء.
   - استخدم `gopls-mcp-server` لفحص كود Go والتشخيصات والرموز.
8. **افحص الكود الفعلي كحكم نهائي:** عند أي تعارض بين وثيقة والكود، الكود والاختبارات هما مصدر إثبات الحالة الحالية.
9. **صنّف المعطيات بدقة:** قسّم أي ملاحظة أو استنتاج إلى `FACT` أو `INFERENCE` أو `UNKNOWN`، ولا تبنِ قراراً معمارياً على غير الحقائق المثبتة.
10. **تحقق قبل وأثناء التعديل:** لا تُجرِ أي تعديل دون وجود اختبار يثبته وخطة تحقق دقيقة.

---

## 2. القواعد الثماني غير القابلة للتفاوض (Non-Negotiable Rules)

1. **حظر Chatwoot المطلق:** تم التخلي عن Chatwoot نهائياً؛ مصدر الحقيقة الوحيد هو `Mujeeb / PostgreSQL`، و`SocialAPI` ناقل خارجي فقط. يُمنع منعاً باتاً إضافة أي حزم، أو مهايئات، أو DTOs، أو مسارات، أو مهام خلفية تخص Chatwoot في الـRuntime.
2. **لا اتصالات شبكية داخل معاملات قاعدة البيانات (No Network Calls inside DB Transactions):** أي اتصال خارجي (SocialAPI, HTTP, LLM) يجب أن يتم خارج كتل معاملات SQL المحلية لحماية اتصالات المجمع ومنع التعليق.
3. **حظر إعادة المحاولة العمياء (No Blind Retry on Unknown Outcome):** عند فشل اتصال المزود الخارجي أو الحصول على استجابة مجهولة، تُنقل الرسالة فوراً إلى `dead_letter` لمراجعتها، ويُحظر إعادة المحاولة التلقائية لتفادي تكرار الرسائل للعميل الخارجي.
4. **الترحيلات للأمام فقط (Forward-Only Migrations):** ترحيلات قاعدة البيانات تصاعدية وتراكمية حصراً (`migrations/*.up.sql`). يُمنع منعاً باتاً تعديل أو حذف ملفات الترحيل التاريخية (`000001` إلى `000052`).
5. **حظر الأسرار والرموز الخام (Zero Raw Secrets/Tokens):** يُحظر وضع أو طباعة أو تسجيل كلمات المرور، أو مفاتيح API، أو رموز الدعوة الخام (`raw invite tokens`) في Git أو الـLogs أو المحادثة.
6. **إلزامية نطاق المستأجر (Mandatory Tenant/Business Scope):** كل أمر واستعلام يخص متجراً يجب أن يتحقق من `business_id` والعضوية النشطة والصلاحيات عبر `ScopeProvider`. لا يُمنح مدير المنصة العام وصولاً صامتاً لبيانات التجار.
7. **حماية المالك الأخير للتاجر (Last Owner Protection):** يُمنع إلغاء أو حذف أو خفض دور آخر مالك نشط (`owner`) للتاجر، ومفروض ذلك على مستوى المعاملة وقاعدة البيانات.
8. **عقد API كودي التوليد (OpenAPI DTO-First):** مصدر الحقيقة لعقد الـAPI هو Go DTOs في `http/dto` وتسجيل العمليات في `contract/api.go`. يُمنع تعديل ملف `mujeeb24-dashboard-v1.generated.yaml` يدوياً؛ التعديل يبدأ من الكود ثم التوليد عبر `cmd/openapi-gen`.

---

## 3. العقود السبعة المغلقة (Closed Contracts)

تم التحقق من إغلاق واستقرار العقود السبعة التالية، ويُحظر إعادة تصميمها أو نقضها:

1. **Chatwoot-free Boundary:**
   - الحذف التام لأي اعتمادية تشغيلية على Chatwoot.
   - الجداول التاريخية (`000034`, `000035`, `000039`) محفوظة في المهاجرات لسلامة تسلسل المخطط فقط، ولا يتعامل معها الـRuntime.
2. **Webhook Ingestion & Signature Verification:**
   - التحقق من توقيع HMAC-SHA256 عبر الترويسة المعتمدة ومفتاح `SOCIALAPI_WEBHOOK_SECRET`.
   - حفظ الـPayload الخام في `inbound_webhook_payloads`، ومنع التكرار (Deduplication) عبر قيد `provider_event_id` في `inbound_event_ledger`.
3. **Outbox & Background Worker:**
   - الـAPI ينشئ `outbound_messages` و`outbox_entries` محلياً داخل معاملة PostgreSQL.
   - المشغل الخلفي [cmd/worker](file:///D:/mujeeb24/mujeeb24-backend-go/cmd/worker/main.go) يسحب السجلات بشكل غير متزامن بنظام الـLease ويستدعي SocialAPI.
4. **Tenant Isolation & Scope Provider:**
   - التحقق من هوية المستخدم `PrincipalID` وعضويته في `business_id` عبر [PostgresScopeProvider](file:///D:/mujeeb24/mujeeb24-backend-go/internal/adapters/primary/http/handlers/postgres_scope.go).
   - حماية الجداول بمفاتيح أجنبية مركبة (`business_id, id`).
5. **Team Invitations & Role Governance:**
   - إنشاء الدعوة يولد رمزاً عشوائياً ويخزن `SHA-256(token)` فقط؛ ويعود الرمز لمرة واحدة للمرسل.
   - قبول الدعوة يستلزم حساب `Principal` مسجلاً ومصادقاً يطابق بريده بريد الدعوة داخل عملية ذرية.
   - الأدوار المعتمدة: `owner`, `admin`, `manager`, `agent`, `analyst`, `viewer`.
6. **Restricted AI Context & Grounded Policy Engine:**
   - الـLLM (Gemini / OpenAI Compatible) يقترح قراراً هيكلياً (`AIDecisionProposal`) بناءً على سياق مقيد ومبني من الكتالوج والسياسات.
   - [GroundedPolicyEngine](file:///D:/mujeeb24/mujeeb24-backend-go/internal/application/services/policy_engine.go) يفحص القرار؛ ولا يملك النموذج صلاحية تنفيذ SQL أو إرسال مباشر.
7. **Worker Lifecycle & Resilience:**
   - المشغل يدعم الإيقاف السلس (Graceful Shutdown) عبر `SIGINT/SIGTERM`.
   - عزل حالات الفشل غير المعروفة فوراً إلى `dead_letter` مع حفظ سبب الخطأ.

---

## 4. البنية المعمارية وتدفق الاعتماديات (Architecture)

يتبع المشروع بدقة تدفق الاعتماديات السداسي (Hexagonal Flow):

```text
Primary Adapters (HTTP / Webhooks)
        ↓
Application Layer (Commands / Queries / Services)
        ↓
Domain Layer & Application Ports (Interfaces)
        ↑
Secondary Adapters (Postgres Persistence / SocialAPI / AI / Ed25519 JWT)
```

- **مسؤولية Bootstrap:** حزمة [internal/bootstrap](file:///D:/mujeeb24/mujeeb24-backend-go/internal/bootstrap) هي المسؤولة الوحيدة عن تركيب وحقن الاعتماديات (Wiring).
- **فصل المسؤوليات (Single Responsibility):** لا تجمع دالة واحدة بين معالجة HTTP، والتحقق، والمنطق الدوميني، واستعلامات SQL، والاتصال بالشبكة.

---

## 5. معايير واجهة البرمجة (API Conventions)

- **المسار الأساسي:** `/api/v1` ومسجل عبر Huma v2 و`net/http.ServeMux`.
- **أغلفة الاستجابة القياسية (Response Envelopes):**
  - الكيان المفرد: `{"data": { ... }}`
  - القوائم: `{"data": [ ... ], "pagination": { "next_cursor": "...", "has_more": false }}`
  - الأخطاء: `{"error": { "code": "...", "message": "...", "fields": { ... }, "retryable": false }}`
- **الترقيم والتصفح (Pagination):** يعتمد كلياً على الـCursors (`limit` و`cursor`).
- **المصادقة (Authentication):** ترويسة `Authorization: Bearer <Ed25519-JWT>` تفحص في Middleware.
- **التفويض (Authorization):** `ActorContext` يحتوي الدور والصلاحيات ضمن نطاق المتجر.
- **منع التكرار (Idempotency):** ترويسة `Idempotency-Key` إلزامية في عمليات الإرسال والإنشاء الحساسة.

---

## 6. معايير قاعدة البيانات والترحيل (Database Conventions)

- **المحرك:** PostgreSQL 16 مدعوماً بـ`pgx/v5` و`pgxpool`.
- **المهاجرات (Migrations):**
  - ملفات SQL متسلسلة (`000001` إلى `000052`) ومضمنة في الباينري عبر `embed.FS`.
  - المهاجرات للأمام فقط؛ يُمنع تعديل المهاجرات السابقة، وأي تعديل جديد يتطلب ملفاً جديداً برقم متسلسل.
- **انضباط المعاملات (Transaction Discipline):**
  - معاملات محلية قصيرة وسريعة.
  - الحفاظ الصارم على قيود المفاتيح المركبة `(business_id, ...)`.
- **فصل الجداول:**
  - جداول الـRuntime النشطة: تشمل العملاء، المحادثات، الرسائل، الكتالوج، المبيعات، صندوق الوارد، سجل الأحداث، وOutbox.
  - جداول المهاجرات التاريخية: مثل `chatwoot_mirror_jobs` معزولة تماماً ولا يتعامل معها التطبيق.

---

## 7. معايير المشغل ومعالجة الرسائل (Worker & Outbox Conventions)

- **تدفق الإرسال الكامل:**
  `API Request -> SQL Transaction (outbound_messages + outbox_entries) -> DB Commit -> Background Worker Poll -> Lease Claim -> SocialAPI Send -> Status Transition`
- **إدارة الحجز (Lease Management):**
  - حجز السجل بتعيين `worker_owner` و`lease_token` ووقت انتهاء `lease_expires_at` (دقيقتان افتراضياً).
- **عزل الأخطاء (Dead-Letter):**
  - عند تعذر تحديد مصير الرسالة لدى المزود (`provider_send_outcome_unknown`) أو نقص معرّف الرسالة، يُنقل السجل فوراً إلى `dead_letter` لمراجعته أو معالجته تسووياً (Reconciliation).

---

## 8. معايير الأمان (Security Conventions)

- **الرموز:** إصدار وتحقق Ed25519 (EdDSA) JWT بمفاتيح غير متماثلة؛ وجلسات تجديد مخزنة في PostgreSQL.
- **كلمات المرور:** تجزئة بـBcrypt مع DefaultCost.
- **رموز الدعوة:** تخزين تجزئة `SHA-256(token)` في قاعدة البيانات، وإرجاع الرمز الخام لمرة واحدة فقط عند الإنشاء.
- **إدارة الأدوار:** التحقق الصارم من الدور قبل أي عملية، وقفل صفوف الملاك لمنع إلغاء آخر مالك نشط.
- **الأسرار والبيئة:** قراءة الأسرار عبر متغيرات البيئة فقط، والتحقق من صحتها وتنسيق HTTPS عند الإقلاع في بيئة الإنتاج.
- **السجلات (Logging):** تسجيل المعرفات والحالات فقط؛ يُحظر تسجيل الـPayloads الخام أو الرموز أو كلمات المرور.

---

## 9. معايير الاختبارات وصدق النتائج (Testing Discipline)

يجب على الوكيل التمييز الصارم بين حالات الاختبار:
- **Unit Tests:** اختبار منطق معزول؛ لا يثبت اتصال قاعدة بيانات أو مزود خارجي.
- **PostgreSQL Integration Tests:** اختبار حقيقي يستلزم وجود متغير البيئة `POSTGRES_TEST_DSN`. **إذا لم يتوفر المتغير، فالاختبار يُعد `SKIPPED` وليس `PASSED`.**
- **Provider Contract Tests:** اختبار يثبت توافق المهايئ مع عقد المزود ولا يثبت الاتصال الحي.
- **Live Smoke Tests:** اختبار فعلي في بيئة مصرح بها يتطلب موافقة وبيانات اعتماد معتمدة.
- **Production Readiness:** جاهزية تشغيلية وأمنية متكاملة تتطلب أدلة مستقلة.

---

## 10. سياسة Git والتسليم (Git Policy)

- **الفرع المعتمد:** العمل يجري حصراً على الفرع `feat/chatwoot-free`.
- **الحظر الصريح:** يُمنع منعاً باتاً إنشاء `commit`، أو `push`، أو `merge` إلى `main` دون إذن صريح ومباشر من المستخدم.
- **نظافة شجرة العمل:** قبل طلب الموافقة، راجع `git status --short` و`git diff --check` وتأكد من خلو العمل من أي أسرار أو تغييرات خارج نطاق المهمة.

---

> **تنبيه نهائي:** هذا الملف هو القانون الهندسي الحاكم لمشروع Mujeeb 24 Backend. عند مواجهة أي حالة غير واضحة، التزم بالتصنيف الثلاثي (`FACT`, `INFERENCE`, `UNKNOWN`) واطلب توضيحاً صريحاً قبل اتخاذ أي قرار غير مثبت.
