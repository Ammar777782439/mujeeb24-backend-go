# P1B-B2 — مراجعة حقول المنشأة وسياسة التشغيل للذكاء الاصطناعي

**فرع التنفيذ:** `feat/merchant-ai-assist`. **المصدر:** PostgreSQL 16 اختبارية بعد ترحيلات `000001..000081`، وعقود Go الفعلية `ports.BusinessRecord` و`ports.BusinessRuntimePolicyRecord`. **الحالة:** أول مراجعة حقول على مستوى المجال؛ لا يوجد API أو أداة قراءة تعرض هذه البيانات للنموذج حتى الآن.

## 1. قاعدة السماح

- تصنيف الجدولين `businesses` و`business_policies` في `source_inventory.go` بأنه `projection_candidate` لا يتيح تصدير أي عمود.
- تُرسل بيانات `BusinessContextV1` و`BusinessRuntimePolicyContextV1` فقط إذا تأكدت طبقة application لاحقًا من **الهوية والمنشأة والصلاحية وحالة AI Guard والحاجة الفعلية للمهمة**.
- لا تنقل IDs أو خام جدول أو SQL. مرجع المصدر ونسخته يستخدمان `EvidenceEnvelopeV1`، وليس عبر نموذج الأعمال كنص.
- سياسة تشغيل AI للتوضيح فقط: حتى إذا كانت `allow_auto_confirmation=true` فلا يصح استخدامها لتجاوز التنفيذ والصلاحيات. لا توجَد Tool Writing في AI Assist v1.

## 2. جدول businesses

النوع والقيود هنا من `testdata/merchant_assist_schema_pg16.json` بعد تنفيذ كل الترحيلات، وليس من استنتاج الواجهة.

| العمود | النوع في PostgreSQL | قرار الإسقاط | السبب |
|---|---|---|---|
| `id` | uuid | **محظور في جسم الإسقاط** | يستخدم داخليًا لربط الدليل والتحقق من نطاق المنشأة |
| `name` | text NOT NULL | **مسموح** | اسم المنشأة، غير فارغ وفق `businesses_name_not_blank_chk` |
| `slug` | text NOT NULL | **محظور** | معرّف تنقل داخلي غير لازم لمساعدة التاجر |
| `status` | text NOT NULL | **مسموح بشروط** | enum: `pending_setup|active|suspended|archived` |
| `vertical_type` | text NOT NULL | **مسموح** | enum: `retail|travel|services|restaurant|clinic|hospitality|education|real_estate|other` |
| `timezone` | text NOT NULL | **مسموح** | المنطقة الزمنية من سجل المنشأة، لا تفسَّر كمعلومة عن التوقيت المحلي لجهاز المستخدم |
| `default_currency` | char(3) NOT NULL | **مسموح** | قيد SQL: ثلاث أحرف ASCII كبيرة، وليس إثباتًا بأن رمز العملة مدعوم تجاريًا دون فحص آخر |
| `locale` | text NOT NULL | **مسموح** | `businesses_locale_not_blank_chk` |
| `created_at`, `updated_at` | timestamptz NOT NULL | **محظور في جسم الإسقاط** | تشغيل وتدقيق؛ الدليل الزمني له عقده الخاص |
| `resource_version` | bigint NOT NULL | **خارج جسم الإسقاط** | تُستخدم النسخة لمراجعة التزامن داخل Evidence عند الحاجة؛ قيد `>0` |

**عقد Go:** `internal/domain/ai/contracts/business.go:BusinessContextV1`. لا يربط طبقة domain مباشرة بـPostgreSQL أو HTTP.

## 3. جدول business_policies

`business_id` مفتاح أساسي وخارجي إلى `businesses.id`. كل الحقول المنطقية المذكورة أدناه `boolean NOT NULL`، و`ai_mode` له `CHECK` يلزم إحدى القيم الفعلية.

| العمود | قرار الإسقاط | السبب |
|---|---|---|
| `business_id` | **محظور في جسم الإسقاط** | نطاق المنشأة من طبقة المصادقة لا من النموذج |
| `ai_mode` | **مسموح بشروط** | enum: `disabled|assist|approval|restricted_auto`. `disabled` يجب أن يوقف الاتصال بالنموذج وفق BusinessAIGuard |
| `default_human_review` | **مسموح بشروط** | يشرح سياسة المراجعة البشرية ولا يمنح صلاحية اعتماد |
| `allow_auto_reply` | **مسموح بشروط** | يشرح سياسة الرد، لا يرسل رسالة |
| `allow_auto_lead_creation` | **مسموح بشروط** | يشرح سياسة إنشاء الفرص، لا ينشئ فرصة |
| `allow_auto_transaction_draft` | **مسموح بشروط** | يشرح سياسة المسودات، لا ينفذها |
| `allow_auto_confirmation` | **مسموح بشروط** | علم سياسة فقط وليس تفويضًا لعمليات مالية |
| `sales_workflow_enabled` | **مسموح بشروط** | أضيف في 000074 |
| `allow_sales_interactions` | **مسموح بشروط** | أضيف في 000074 |
| `business_hours_reference`, `escalation_policy_reference` | **محظور v1** | مراجع قد تتطلب فحص سياسة منشورة منفصلًا، لا تُرسل تلقائيًا |
| `created_at`, `updated_at` | **محظور** | تشغيل وتدقيق |
| `resource_version` | **خارج جسم الإسقاط** | إدارة التعارض والتزامن ضمن بيانات الدليل |

**عقد Go:** `BusinessRuntimePolicyContextV1`. مجرد عرض لمحتوى سجل سياسة التشغيل؛ الموافقة والتحكم به يخضعان لتطبيق قواعد العمل الحالية.

## 4. بوابة الصلاحيات

في `authorization/permissions.go`، عمليات قراءة المنشأة `getBusiness`, `getBusinessPolicy` تتطلب `business:read`. عملية AI Assist المقترحة ستتطلب أيضًا `merchant_ai:use`. التحقق من ملكية/عضوية المنشأة يتم قبل قراءة كل سجل.

**مهم:** عقد Go المُضاف لا يستدعي `authorization.Has` أو `BusinessAIGuard` مباشرة لأنهما في application؛ سيتم استخدامهما عند إنشاء reader في المرحلة اللاحقة.

## 5. قواعد المراجعة واختبارات الانحراف

- لا يمر تغيير `BusinessContextV1` أو `BusinessRuntimePolicyContextV1` إذا غيّر أسماء حقول JSON دون تحديث اختبارات المراجعة.
- تقرأ الاختبارات snapshot PostgreSQL الفعلي وتتحقق من وجود نوع كل حقل في جدول المصدر، وصحة قيم enum المعتمدة.
- يُرفض إدخال `slug`، `business_id`، `secret`، metadata التشغيلية ومفاتيح المزوّد في JSON.
- لا تُستنتج قيمة السعر أو توفر المنتج أو الإذن بالتصرف من حقول سياسة المنشأة.
- لا يوجد `SELECT *` أو Runtime Reader أو اتصال نموذج في هذه الدفعة.
- مصدر `business_policy_versions` وعقود الوثائق المنشورة يحتاجان مراجعة مستقلة تراعي `status=published` و`valid_from/valid_until` والفهرس الفريد الجزئي قبل أي استعمال للـAI.

**الحالة:** حقول `businesses` و`business_policies` المنقحة مُعرّفة ومختبرة؛ **لم تُربط بخدمة قراءة مؤهلة أو Prompt أو API**. تستمر مراجعة باقي المجالات تدريجيًا.
