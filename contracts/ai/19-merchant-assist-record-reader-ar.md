# P3-B — قراءة سجلات العملاء والمحادثات والفرص والمعاملات

**الفرع:** `feat/merchant-ai-assist` (من `main`). **النطاق:** واجهات قراءة داخلية (Go) فقط، بلا HTTP endpoint وبلا Tool Calling وبلا إرسال إلى AI. **الحالة:** تنفيذ مُجزّأ وقابل للاختبار.

## الهدف الأصلي

تجهيز منصة Mujeeb 24 ليتمكن AI Assist في لوحة التاجر مستقبلًا من فهم السجل التجاري الذي يراجعه الموظف، في ضوء بيانات النظام الفعلية وصلاحياته، بدل الاعتماد على Prompt عام أو محتويات قاعدة البيانات كاملة. تظل عقود `CustomerSalesContext` و`MerchantCatalogAI` و`CatalogAIProjection` ومسارات تنفيذ المبيعات مغلقة وغير معدّلة.

## المدخلات والعقود

`internal/application/merchantassist/record_reader.go` يعرّف `RecordReader.Read(ctx, RecordReadRequest)` ومعرفات `customer|conversation|lead|transaction`. يستخدم واجهات ضيقة تطابق **عمليات القراءة القائمة**:
- `ports.CustomerRepository.GetByID(ctx,businessID,customerID)`
- `ports.ConversationRepository.GetByID(ctx,businessID,conversationID)`
- `ports.LeadRepository.Get(ctx,businessID,leadID)`
- `ports.TransactionRepository.Get(ctx,businessID,transactionID)`

**قبل القراءة:** `ActorContext` متحقق منه في طبقة المصادقة (يجب ألا يؤخذ من body مستقبلاً)، و`Actor.BusinessID == req.Business`، ووجود principal، وصلاحية `merchant_ai:use` بالإضافة إلى الصلاحية التجارية المطلوبة، ومهمة `explain|summarize` ومعرف سجل مقيد، وفحص `BusinessAIExecutionGuard` الفعلي. لا يوجد `SELECT *` أو بحث عام عبر مستودعات دون حدود.

| نوع السجل | الصلاحية الثانية | الإسقاط | المحجوب عنه |
|---|---|---|---|
| Customer | `customers:read` | `CustomerAssistV1`: المرجع، اللغة الاختيارية، الحالة | `profile`, `contact_points`, بيانات الدمج، تفاصيل العميل |
| Conversation | `conversations:read` | `ConversationAssistV1`: المراجع والحالة والملكية والأولوية ووضع AI ووقت آخر نشاط | `CustomerDisplayName` ما لم يراجع PII، `AssignmentReference`، `LastGeminiInteractionID`، النصوص الخام |
| Lead | `sales:read` | `LeadAssistV1`: المرجع والحالة والتأهيل والتقييم ووقت الإجراء التالي | `QualificationContext/Evidence`, معلومات الموظف/نية العميل غير المعتمدة، أسباب الخسارة الخام |
| Transaction | `sales:read` | `TransactionAssistV1`: المرجع ونوع المعاملة والحالة والمبلغ/العملة الموثقان ومراجعة الإنسان | `Lines` و`TransactionData` واللقطات الخام وعلاقات المصدر الحساسة |

**بعد القراءة:** يُعاد فحص `BusinessID`, `ID`, `ResourceVersion>0` لكل سجل، والتحقق من صحة العقد باستخدام `Validate()`. إذا ظهرت بيانات لا تتوافق مع منشأة الطالب أو العقد، تُرفض الاستجابة بالكامل. لا تُعرض نتائج جزئية لحساب مختلف.

## Evidence Contract

الإسقاط مرفق بـ`EvidenceEnvelopeV1`: `resource_ref` بنوع السجل والمعرف الحقيقي، `source_type` مغلق، `resource_version` من السجل الحقيقي، وقت الحصول عليه و`valid_until` قصير (دقيقتان)، `authority=system_record`، و`completeness=partial` **عمداً**. هذه تفاصيل محددة وليست إثباتًا لاكتمال السجل بأكمله أو صحة سعر الكتالوج الآن. انتهاء الدقيقتين لا يعني صحة السعر خلال تلك المدة؛ أي عملية تجارية تحتاج إعادة تحقق مستقلًا.

حجم الاستجابة مضبوط بحد تسلسل داخلي. `RecordReader` لا يقرأ الرسائل أو البنود الموسعة ويستخدم DTOs ذات حقول صريحة بدل نسخ raw ports structs. الاستثناءات والأخطاء لا تتحول إلى «سجل غير موجود» أو «تم التنفيذ».

## الاختبارات

`record_reader_test.go` يغطي أربعة أنواع من السجلات باستخدام منافذ قراءة وهمية:
- مراجع الأدلة وإصدارات السجلات وعدم تسرب profile/contact، ومعرف إسناد داخلي، وJSON تأهيل الفرصة وJSON بنود المعاملة.
- رفض تجاوز صلاحية المجال، وغياب صلاحية AI، وعدم تطابق منشأة المستخدم، وغياب principal **قبل أي استعلام**.
- رفض سجل أعاده repository بمُعرّف مختلف أو منشأة أخرى أو نسخة غير صالحة.
- احترام تعطيل AI؛ رفض المهام والمعرفات غير الصالحة؛ منع قيم الأسعار غير الصحيحة.
- نشر ملفات الاختبار بصيغ `gofmt` الرسمية؛ التحقق بواسطة Go CI.

## قيود لا نتجاوزها

- `RecordReader` ليس موصولًا بـHTTP ولا ببوابة DeepSeek/Gemini ولا بـ`Registry` التنفيذ. هو طبقة داخلية آمنة من حيث اختيار الحقول والقراءة المصرح بها؛ لا يُعد فحص PII كليًا أو اختبارًا لإرسال المحتوى.
- `CustomerProfile` و`CommunicationMessage.Text` و`ConversationState.Preferences/Constraints` و`Lead.QualificationContext` و`TransactionData` و`SalesWorkflow.Proposal` لا تدخل ضمن الإسقاطات الجديدة. هذه تحتاج عقودًا وتصفية PII وحقول JSONB مرتبطة بالمجال قبل أي استدعاء مزود.
- صيغ `decimal string` والحالات تأتي من PostgreSQL والعقود الحالية، وليست تخمينات LLM.
- اختبار `ActorContext` يتطلب من نقطة الربط المستقبلية مصادقة حقيقية/التحقق من العضوية؛ لا يصح أن يُنشأ بمدخلات النموذج.
- الواجهة ليست بديلًا عن صلاحيات `sales:approve` أو عن `CustomerSalesContext`.

## ما يلي

P3-C: قراءة رسائل المحادثة النصية العامة وأدلة الملخص والتأهيل التجاري ضمن حدود واضحة، بعد اعتماد تصفية محتوى الرسائل ومعلومات العميل وتنقيح PII، والتحقق من الربط بالمنشأة واسم المورد. ثم إكمال سجل أدوات قراءة `Registry` وواجهة AI Assist، مع اختبارات سياسات النشر وإخفاق المزود. لا دمج مع `main` دون تصريح المستخدم.
