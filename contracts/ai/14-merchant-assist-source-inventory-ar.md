# MUJEEB24 — مراجعة مصادر البيانات لمرحلة P1B
**المستودع:** `Ammar777782439/mujeeb24-backend-go` · **فرع التنفيذ:** `feat/merchant-ai-assist` · **المصدر:** `main` الذي تفرّعنا منه · **آخر ترحيل مفحوص:** `000081`.

> **مرحلة ثانية — مسح الجداول من الكود فقط.** هذا جرد لأسماء الجداول وتصنيفاتها ولا يمنح أي نموذج إذن قراءة. لا يثبت الحالة الفعلية لقاعدة إنتاج منشورة دون تطبيق الترحيلات والاستعلام من `information_schema`. لم تتم مراجعة جميع الأعمدة والقيود والـForeign Keys عبر كل ترحيل حتى الآن.

## نتائج جرد الترحيلات
- 81 ملفًا من `000001` إلى `000081` باسم `*.up.sql`.
- 80 تعريف جدول `CREATE TABLE` مسجل في `source_inventory.go`، منها 3 جداول قديمة أزيلت صراحة بالترحيل `000053`.
- مرشح للبحث عن إسقاطات AI آمنة: **33**؛ لا يوجد حقل مسموح فعليًا في هذه المرحلة.
- مخصص للباك إند/التشغيل: **33**؛ ليس للـAI الخام.
- مصدر حساس ممنوع من الإرسال الخام: **11**.
- جداول محذوفة: **3**.
- `schema_migrations` قد يديره مُشغّل الترحيلات، ويظل خارج أي إسقاط AI ولا يصنف على أنه جدول تجاري منشأ بأحد ملفات `*.up.sql`.

## سجل مصادر قاعدة البيانات
العمود «مصدر الإنشاء» هو ملف `000NNN_*.up.sql`؛ الترحيلات الأحدث ربما عدّلت الأعمدة والقيود. التصنيف أدناه لا يجيز إرسال الجدول إلى النموذج.

| المصدر | مصدر الإنشاء | المجال | حالة المصدر |
|---|---:|---|---|
| `businesses` | `000001` | business | مرشح لإسقاط آمن (غير معتمد) |
| `business_policies` | `000002` | policy | مرشح لإسقاط آمن (غير معتمد) |
| `channel_connections` | `000003` | channels | مرشح لإسقاط آمن (غير معتمد) |
| `channel_connection_capabilities` | `000004` | channels | مرشح لإسقاط آمن (غير معتمد) |
| `customers` | `000005` | customer | مرشح لإسقاط آمن (غير معتمد) |
| `external_identities` | `000006` | identity | ممنوع في AI Assist |
| `identity_matches` | `000007` | identity | ممنوع في AI Assist |
| `conversations` | `000008` | inbox | مرشح لإسقاط آمن (غير معتمد) |
| `conversation_references` | `000009` | inbox | داخلي فقط |
| `inbound_event_ledger` | `000010` | transport | داخلي فقط |
| `outbound_messages` | `000011` | transport | داخلي فقط |
| `outbox_entries` | `000012` | transport | داخلي فقط |
| `catalogs` | `000013` | catalog | مرشح لإسقاط آمن (غير معتمد) |
| `attribute_schemas` | `000014` | catalog | مرشح لإسقاط آمن (غير معتمد) |
| `attribute_definitions` | `000015` | catalog | مرشح لإسقاط آمن (غير معتمد) |
| `catalog_items` | `000016` | catalog | مرشح لإسقاط آمن (غير معتمد) |
| `variants` | `000017` | catalog | مرشح لإسقاط آمن (غير معتمد) |
| `offers` | `000018` | catalog | مرشح لإسقاط آمن (غير معتمد) |
| `leads` | `000019` | sales | مرشح لإسقاط آمن (غير معتمد) |
| `lead_attributions` | `000020` | sales | مرشح لإسقاط آمن (غير معتمد) |
| `lead_scores` | `000021` | sales | مرشح لإسقاط آمن (غير معتمد) |
| `commercial_transactions` | `000022` | sales | مرشح لإسقاط آمن (غير معتمد) |
| `transaction_confirmations` | `000023` | sales | مرشح لإسقاط آمن (غير معتمد) |
| `transaction_reviews` | `000024` | sales | مرشح لإسقاط آمن (غير معتمد) |
| `order_lines` | `000025` | sales | مرشح لإسقاط آمن (غير معتمد) |
| `audit_events` | `000026` | audit | داخلي فقط |
| `ai_decisions` | `000027` | ai_ops | داخلي فقط |
| `communication_messages` | `000028` | inbox | مرشح لإسقاط آمن (غير معتمد) |
| `chatwoot_contact_links` | `000034` → حذف 000053 | retired | محذوف في الترحيلات |
| `chatwoot_workspace_bindings` | `000034` → حذف 000053 | retired | محذوف في الترحيلات |
| `business_policy_versions` | `000036` | knowledge | مرشح لإسقاط آمن (غير معتمد) |
| `knowledge_documents` | `000036` | knowledge | مرشح لإسقاط آمن (غير معتمد) |
| `channel_provisioning_sessions` | `000037` | channels | ممنوع في AI Assist |
| `inbound_webhook_payloads` | `000038` | transport | ممنوع في AI Assist |
| `chatwoot_mirror_jobs` | `000039` → حذف 000053 | retired | محذوف في الترحيلات |
| `outbound_delivery_status_updates` | `000040` | transport | داخلي فقط |
| `business_memberships` | `000041` | identity | داخلي فقط |
| `principals` | `000041` | identity | ممنوع في AI Assist |
| `refresh_sessions` | `000041` | identity | ممنوع في AI Assist |
| `conversation_labels` | `000044` | inbox | مرشح لإسقاط آمن (غير معتمد) |
| `channel_connection_state_events` | `000045` | channels | داخلي فقط |
| `conversation_read_cursors` | `000046` | inbox | داخلي فقط |
| `canned_replies` | `000047` | inbox | مرشح لإسقاط آمن (غير معتمد) |
| `automation_executions` | `000048` | automation | داخلي فقط |
| `automation_rules` | `000048` | automation | مرشح لإسقاط آمن (غير معتمد) |
| `team_invitations` | `000049` | identity | ممنوع في AI Assist |
| `platform_super_admins` | `000050` | identity | ممنوع في AI Assist |
| `conversation_state` | `000052` | inbox | مرشح لإسقاط آمن (غير معتمد) |
| `merchant_ai_messages` | `000054` | ai_ops | داخلي فقط |
| `merchant_ai_sessions` | `000054` | ai_ops | داخلي فقط |
| `ai_catalog_batches` | `000055` | ai_ops | داخلي فقط |
| `ai_gemini_interactions` | `000055` | ai_ops | داخلي فقط |
| `ai_run_attempts` | `000055` | ai_ops | داخلي فقط |
| `ai_runs` | `000055` | ai_ops | داخلي فقط |
| `ai_stage_latencies` | `000055` | ai_ops | داخلي فقط |
| `ai_tool_calls` | `000055` | ai_ops | داخلي فقط |
| `ai_usage_telemetry` | `000055` | ai_ops | داخلي فقط |
| `plans` | `000061` | billing | مرشح لإسقاط آمن (غير معتمد) |
| `ai_provider_pricing_versions` | `000062` | ai_ops | داخلي فقط |
| `ai_usage_records` | `000062` | ai_ops | داخلي فقط |
| `subscription_ai_usage` | `000062` | billing | مرشح لإسقاط آمن (غير معتمد) |
| `subscription_payments` | `000062` | billing | ممنوع في AI Assist |
| `subscriptions` | `000062` | billing | مرشح لإسقاط آمن (غير معتمد) |
| `support_messages` | `000063` | support | داخلي فقط |
| `support_tickets` | `000063` | support | داخلي فقط |
| `platform_audit_events` | `000064` | audit | داخلي فقط |
| `ai_configuration_versions` | `000065` | ai_ops | داخلي فقط |
| `ai_provider_credentials` | `000065` | ai_ops | ممنوع في AI Assist |
| `ai_provider_models` | `000065` | ai_ops | داخلي فقط |
| `channel_provider_brands` | `000066` | channels | داخلي فقط |
| `merchant_ai_proposals` | `000069` | ai_ops | داخلي فقط |
| `channel_history_purges` | `000070` | channels | داخلي فقط |
| `sales_action_jobs` | `000074` | sales | داخلي فقط |
| `sales_command_results` | `000074` | sales | داخلي فقط |
| `sales_confirmation_requests` | `000074` | sales | مرشح لإسقاط آمن (غير معتمد) |
| `sales_transaction_requirements` | `000074` | sales | مرشح لإسقاط آمن (غير معتمد) |
| `sales_workflows` | `000074` | sales | مرشح لإسقاط آمن (غير معتمد) |
| `subscription_addon_payments` | `000077` | billing | ممنوع في AI Assist |
| `subscription_addons` | `000077` | billing | مرشح لإسقاط آمن (غير معتمد) |
| `subscription_channel_pass_capacity_approvals` | `000078` | billing | داخلي فقط |

## فوارق حرجة يجب الحفاظ عليها
1. `000029` يضيف إصدارات موارد الكتالوج؛ تستخدم النسخ للتحقق من تغير السجل بين القراءة واقتراح AI.
2. `000030` يعدل عقود المبيعات والقيود؛ لا نستبدل Enums الفعلية باقتراحات AI.
3. `000034` و`000039` أضافا Chatwoot، ثم **`000053` أسقط الجداول الثلاثة وأعمدتها الخاصة**. لا أدوات ولا إسقاطات لتلك المصادر القديمة.
4. `000041` يقدم بيانات هوية وجلسات وعضويات؛ يصدر الباك إند قرارات الصلاحيات دون إرسال سجلات المصادقة الخام.
5. `000042` و`000043` يضيفان إصدارات للموارد التجارية؛ لا نخلط snapshots مختلفة.
6. `000054` و`000055` يضيفان جلسات مساعد الكتالوج وتتبعات AI؛ لا نجعلها بيانات لوحة عامة.
7. `000058` و`000080` يحدثان ملخصات المحادثات/checkpoints؛ لا نعتبرها تغطية لكل الرسائل دون مقارنة نقطة التغطية.
8. `000065` يحتوي سجلات مزودي AI ومعلومات قد تشمل أسرارًا؛ لا تُعرض مباشرة.
9. **`000068` يحذف `attribute_definitions.is_searchable`**؛ ممنوع إعادة تضمينه في العقد الجديد.
10. `000074` يضيف مسارات وأوامر المبيعات؛ قدرات AI Assist الأولى اقتراحات وقراءة فقط، لا موافقات أو إرسال.
11. `000076`–`000078` تغير حدود الاشتراكات والإضافات؛ بيانات السقف تؤخذ من الخدمات الفعالة لا من افتراضات ثابتة.
12. `000081` يراجع تصنيفات أسباب الفشل الداخلي؛ تبقى سجلات الفشل التشغيلية خارج AI Assist.

## إجراءات P1B المتبقية — لا يجوز تجاوزها
- تحليل أعمدة وأنواع وConstraints وFK وCHECK وALTER/DROP/RENAME لكل مصدر مرشح من 33؛ وإثبات الشكل النهائي في قاعدة PostgreSQL اختبارية بعد تشغيل كل الترحيلات.
- مقارنة `schema.sql` مع النتيجة الفعلية، وتوثيق أي اختلاف بدل توفيقه بالتخمين.
- مراجعة كل حقل مرشح لدى مالك المجال: معنى الحقل، نوعه، حدود PII، قاعدة التنقيح، الربط، صلاحية القراءة، سبب الحاجة، freshness.
- إضافة allowlist صريحة **للحقول** إلى عقود `ProjectionDescriptor` حين تثبت صحتها؛ لا تُعتبر حالة `SourceProjectionCandidate` إعفاءً من ذلك.
- ربط الاختبار البنيوي بالأذونات الفعلية والمهاجرات على CI قبل تشغيل أي قارئ AI.

## الضمان الحالي
`TestMigrationTableInventoryFailClosed` يمسح ملفات `migrations/*.up.sql`، يرفض أي جدول `CREATE TABLE` غير مسجل أو ترحيل مفقود، ويتحقق من وجود DROP للجداول المحذوفة، كما يمنع تكرار الجداول. **الاختبار يفحص عبارات CREATE/DROP المباشرة، وليس مفسر SQL كاملًا**؛ لذلك لا يحل محل PostgreSQL integration ولا تحليل ALTER TABLE.
