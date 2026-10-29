# P1B-B2 — عقود السياسات والمعرفة والعملاء والمحادثات والمبيعات

**المستودع:** `Ammar777782439/mujeeb24-backend-go` · **الفرع:** `feat/merchant-ai-assist` فقط · **المصدر:** `main` · **الحالة:** عقود قراءة أولية منضبطة + اختبارات؛ لا توجد أدوات AI مرتبطة بها أو API أو مزود نموذج متصل.

## 1. المراجع المعتمدة

- PostgreSQL 16 بعد تطبيق ترحيلات `000001..000081` في `internal/platform/database/testdata/merchant_assist_schema_pg16.json`: أسماء الأعمدة، الأنواع، CHECK/PK/FK/UNIQUE والفهارس المستقلة.
- عقود Go الحالية: `ports/knowledge_policy_repository.go`, `core_repositories.go`, `message_repository.go`, `conversation_state.go`, `inbox_repositories.go`, `sales_repositories.go`, `sales_workflow.go`.
- صلاحيات العمليات: `internal/application/authorization/permissions.go`. **لا يكفي مجرد التحقق من وجود الصلاحية**؛ يجب إثبات عضوية المنشأة وملكية السجل وجواز ظهوره للتاجر، ثم تطبيق التنقيح قبل مزود AI.
- عقد `CustomerSalesContext` مستقل عن B2B Assist، ولا يتغير في هذه الدفعة.

## 2. العقود التي أضيفت أو روجعت

| المجال | إسقاط AI محدد | الجدول/الأعمدة المؤهلة (بعد مراجعة هوية السجل والتنقيح) | الأعمدة المستبعدة والقيود |
|---|---|---|---|
| سياسة منشورة | `PublishedPolicyAssistV1` | `business_policy_versions.policy_key, category, title, summary, authority, status, version, valid_from, valid_until` | `rules` JSONB **غير مرخص v1**، `business_id/id/created_at/updated_at` داخل الخادم فقط، `status=published` وسريان التواريخ، `authority=merchant` |
| معرفة منشورة | `PublishedKnowledgeAssistV1` | `knowledge_documents.knowledge_key, title, content, content_type, authority, status, version, valid_from, valid_until` | `source_reference` غير مرخص، لا draft/archived/expired؛ محتوى المستند نص غير موثوق ولا يتجاوز تعليمات النظام |
| العملاء | `CustomerAssistV1` | مرجع `customers.id` صادر من السيرفر، `locale_preference, status` | `profile`, `contact_points`, `merged_into_customer_id` والبيانات المعرِّفة الحساسة غير مرخصة؛ الحالة `active/merged/archived` |
| المحادثات | `ConversationAssistV1` | `conversations.id, customer_id, state, ownership, priority, ai_mode_override, last_activity_at` | `assignment_reference`, `last_gemini_interaction_id`, `business_id`, بيانات التشغيل والتوقيت الداخلي مستبعدة |
| رسائل المحادثة | `PublicTextMessageAssistV1` | `communication_messages.id, direction, origin, visibility, content_type, text_content, occurred_at` | **فقط** `visibility=public`, `content_type=text`، لا private note/media/raw provider_message_id/outbound/inbound reference IDs. النص قد يحوي PII ويحتاج تنقيحًا قبل الإرسال |
| تلخيص الحالة | `ConversationSummaryAssistV1` | `conversation_state.summary, summary_last_message_id, summary_turn_count` | لا يمر ملخص دون نقطة توقف؛ يجب قراءة الرسائل اللاحقة أيضًا والتحقق من التغطية والنسخة؛ JSONB `focus/previous/preferences/constraints/pending` ليس مرخصًا بعد |
| ردود جاهزة | `CannedReplyAssistV1` | `canned_replies.title, shortcut, body, status` | لا `business_id/id/resource_version` في جسم الإسقاط؛ لا حفظ أو إرسال تلقائي؛ shortcut normalized |
| فرص | `LeadAssistV1` | `leads.id, customer_id, status, qualification_state, current_score_band, current_score_value, next_action_at` | `qualification_context`, `qualification_evidence`, `created_by`, `intent_reference`, `assigned_ownership_reference`, `lost_reason` وغيرها خارج نسخة الإسقاط؛ درجة التقييم لا تثبت نية الشراء |
| معاملـة | `TransactionAssistV1` | `commercial_transactions.id, customer_id, transaction_type, state, requires_human_review, currency, total_amount` | `transaction_data` JSONB و`agreement_version` والسياسات وعلاقات المصدر تحتاج عقدًا أكثر تخصيصًا؛ المال decimal string، العملة مشروطة |
| بند معاملة | `TransactionLineAssistV1` | `order_lines.catalog_item_id, item_name_snapshot, quantity, unit_price_snapshot, line_total_snapshot, currency` | `selected_attributes_snapshot`, `pricing_snapshot`, `availability_snapshot`, `fulfillment_snapshot` تحتاج مراجعة مفاتيحها؛ snapshot تاريخي وليس السعر الحالي |
| مراجعة معاملة | `TransactionReviewAssistV1` | `transaction_reviews.required, status` | `reviewer_reference, reason_codes, decision_reason` غير مرخصة بدون تنقيح خاص؛ حالات `pending/approved/rejected/bypassed` مع invariant required |
| تأكيد معاملة | `TransactionConfirmationAssistV1` | `transaction_confirmations.status, confirmed_at` | `confirmed_by, evidence_reference, policy_version` غير مرخصة v1؛ lifecycle مطابق للقيود |
| مسار بيع | `SalesWorkflowAssistV1` | `sales_workflows.id, customer_id, state, transaction_id, lead_id` | `proposal` JSONB و`source_message_id`, `pending_action` خارج v1 إلى حين مراجعة الحقول والمعاني؛ لا تحكم في القبول/الرفض |
| متطلبات بيع | `SalesRequirementHeaderAssistV1` | `sales_transaction_requirements.transaction_type, schema_version` | `definitions` JSONB خارج v1 حتى تدقيق `SalesRequirementDefinition` مع تحقق الأنواع والقيود |

**غير مشمول في هذه الإسقاطات:** `lead_attributions`, `lead_scores`, `sales_confirmation_requests`، أعلام مفصلة في `conversation_state`، وقواعد المعاملة JSONB الداخلية. الجرد السابق لا يجعلها مرخصة؛ تغلق بعقود منفصلة قبل الاستخدام. لا تلتف الواجهة حول هذا الاستبعاد.

## 3. قيم Enum الفعلية من PostgreSQL

تتحقق `reviewed_sql_enums_test.go` من ثبات مجموعة قيم أكثر من 20 قيدًا للسياسات والمعرفة والعملاء والمحادثات والمبيعات. لا يجوز نقل وصف حر من model output إلى status مغاير لقيم الـCHECK.

- Published: `status=draft|published|archived` على قاعدة البيانات، لكن **إسقاط AI يقبل فقط published**.
- Conversation `visibility=public|private` في SQL، ولكن **PublicTextMessageAssistV1 لا يقبل private**.
- Transactions `currency` عند وجود مبلغ؛ decimal مخزن كـ`numeric(20,4)` ويتنقل كنص لا float.
- `lead_score` `numeric(5,2)` ضمن 0..100؛ لا يجوز للنموذج حسابها أو استخدامها كتأكيد شراء.
- States والتحولات في `SalesWorkflow` و`TransactionReview` و`TransactionConfirmation` تبقى في Go وخدمات المبيعات؛ إسقاط AI لا ينفذ التحولات.

## 4. الحقوق ومتطلبات نطاق المنشأة

| العقد | صلاحية قراءة المجال الحالية | متطلبات إضافية قبل النقل إلى AI |
|---|---|---|
| السياسات والمعرفة | `business:read` أو مسار معرفة مصرح محدد؛ لا تفترض صلاحية بلا عملية مراجعة | `merchant_ai:use`، عضوية المنشأة، status/validity، فحص حدود النص |
| العملاء | `customers:read` | `merchant_ai:use`، حجب JSONB/raw contact + نطاق العميل |
| المحادثات والرسائل/الملخص | `conversations:read` | `merchant_ai:use`، فلترة `visibility=public`، عدم استخدام raw provider metadata، checkpoint + recent window |
| الردود الجاهزة | في `main` عملياتها الحالية تحت `conversations:reply` | لا تفترض `conversations:read` بديلًا؛ أعد استخدام صلاحية العملية الحالية |
| فرص البيع والصفقات | `sales:read` | `merchant_ai:use`، فحص كل سجل، عدم تجاوز `sales:approve` |
| متطلبات البيع | `sales:read` | السماح metadata فقط قبل تدقيق dynamic JSONB |

**ملاحظة تنفيذية:** `RequiredPermission` في سجل عقود AI لن يمنح الصلاحية وحده. يجب استخدام auth/tenant guard من application قبل بناء الإسقاط.

## 5. اختبارات ومعايير عدم التسريب

- `reviewed_projection_test.go`: حالات صحيحة ومرفوضة لكل مجموعة، ويقارن **مجموعة مفاتيح JSON بالضبط** مع قائمة المراجعة ويربط نوع كل حقل بعنصر PostgreSQL الفعلي (مع mapping واضح لاسم المرجع).
- `reviewed_sql_enums_test.go`: يحذر من أي تغيير في الحالة أو قيم enum الفعلية دون إعادة اعتماد العقود.
- `testdata/merchant_assist_schema_pg16.json`: صورة بعد 81 ترحيلًا، ويظل فحص schema drift مفعلًا في PostgreSQL CI.
- يجب إضافة اختبارات تكامل tenant-scope / PII sanitization وقيود النص والبيانات وإصدار الدليل عند تنفيذ reader في المرحلة التالية؛ ليست هذه الاختبارات حاليًا إثباتًا بأن بيانات الإنتاج تُرسل بأمان.

## 6. ما لم ننفذه ولم نجزه

- لا API `ai-assist` ولا provider/model call ولا Tools runtime، ولا تغيير في Frontend.
- لا صلاحيات إضافية أو روابط قراءة للنموذج؛ كل الإسقاطات prototypes وليست مُسجلة في runtime.
- لا إرسال صامت لكل محادثة أو كل عميل أو كل فرصة.
- لا إعادة تعريف عقود `CustomerSalesContext`, `CatalogEntityContract`, `CatalogAIProjection` أو تعديل منطق الموافقة/التنفيذ.
- **جميع raw JSONB التجارية الحساسة تتطلب عقودًا دقيقة مستقلة؛ لا `map[string]any` مفتوح.**
- الوثيقة توثق أسماء البيانات وتدقيقها في PostgreSQL الاختبارية، ولا تثبت تطابق المخطط مع production حتى تنفذ مقارنة بيئة الإنتاج بموافقة صاحب المشروع.

## 7. بوابة المرحلة اللاحقة

الخطوة التالية قبل أي اتصال بالنموذج: `authorized application read facades` لكل مجال + verified `EvidenceEnvelopeV1` + redaction/PII + bounded pagination + freshness والاختبارات الأمنية (cross-merchant/injection). بعد ذلك فقط يمكن تسجيل الإسقاطات المسموحة في runtime وبناء API/provider orchestration بصورة تدريجية.
