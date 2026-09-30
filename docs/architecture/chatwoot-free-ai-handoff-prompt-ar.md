# مرجع تسليم Mujeeb 24 إلى أي AI/Agent

> **هذه وثيقة تشغيلية إلزامية وليست بديلًا عن قراءة الكود والاختبارات.** الغرض منها منع سوء الفهم وإعادة القرارات الملغاة. عند تعارض هذه الوثيقة مع الكود الحالي، لا تخمّن: أوقف التعديل، افحص `git status` و`git log` والملفات المصدرية والاختبارات، ثم سجّل التعارض بوضوح قبل اقتراح الحل.

## تعليمات البداية للوكيل

أنت تعمل على Backend مشروع **Mujeeb 24** المكتوب بلغة Go. قبل أي تحليل أو تعديل يجب عليك قراءة `README` و`todo.md` وهذه الوثيقة ووثائق `docs/architecture` ذات الصلة، ثم فحص الفرع الحالي وحالة العمل:

```text
git status --short
git branch --show-current
git log -5 --oneline --decorate
```

اعمل فقط داخل مستودع Backend المطلوب. لا تلمس Dashboard أو أي مستودع آخر ما لم يطلب صاحب المشروع ذلك صراحة. لا تعرض أسرارًا في المخرجات، ولا تطلب كلمة مرور أو token أو API key داخل المحادثة.

## القرار المعماري غير القابل للتفاوض

لقد **تخلّينا نهائيًا عن Chatwoot**. Chatwoot ليس dependency ولا runtime service ولا adapter ولا webhook ولا mirror ولا جزءًا من Docker Compose الحالي أو المستقبلي لهذا التصميم.

لا تفعل أيًا من الآتي:

- لا تعِد إضافة Chatwoot أو تقترحه كحل مؤقت أو دائم.
- لا تنشئ Chatwoot account أو inbox أو conversation من Mujeeb.
- لا تضف Chatwoot DTOs أو routes أو ports أو workers أو migrations جديدة.
- لا تستنتج من migrations أو الوثائق التاريخية `000001–000045` أن Chatwoot ما زال جزءًا من runtime.
- لا تعدّل migrations `000001–000045` ولا تحذف بيانات أو volumes أو containers.

أي تغيير في المخطط يجب أن يكون **forward-only migration** جديدًا، مع مراجعة tenant scope وidempotency وrollback/reconciliation. لا تنفذ حذفًا أو migration destructive بلا موافقة صريحة وخطة backup مستقلة.

## مصدر الحقيقة وحدود المكونات

| المكوّن | مسؤوليته | ما لا يملكه |
| --- | --- | --- |
| **Mujeeb/PostgreSQL** | مصدر الحقيقة للأنشطة والتجار والعملاء والمحادثات والرسائل والحالة والإسناد والكتالوج والسياسات والتدقيق. | لا يعتمد على Chatwoot لإثبات الحالة. |
| **SocialAPI** | نقل أحداث ورسائل القنوات الاجتماعية إلى Mujeeb وإيصال الرسائل الصادرة إلى القناة الخارجية. | لا يملك نموذج Mujeeb للمحادثة أو العميل أو الفريق أو الصلاحيات. |
| **Event Ledger/Idempotency** | تسجيل الأحداث ومنع معالجة callback المكرر بطريقة آمنة. | لا يرسل رسالة ولا يقرر صلاحيات الأعمال. |
| **Outbox/Worker** | فصل الكتابة المحلية عن الإرسال الخارجي، وإدارة leases والحالات وإعادة المحاولة الآمنة. | لا ينفذ blind retry عند نتيجة مزود غير معروفة. |
| **Inbox** | واجهة ومنطق محادثات مملوكان لـMujeeb: القراءة، الإسناد، الأولوية، labels، الملاحظات، الردود الجاهزة والأتمتة المقيدة. | لا يعتمد على Chatwoot. |
| **AIRuntime** | اقتراح structured decision اعتمادًا على context يجهزه Mujeeb. | لا يقرأ SQL، ولا يكتب PostgreSQL، ولا يملك Outbox أو SocialAPI أو صلاحية إرسال. |
| **Platform Super Admin** | أساس bootstrap وحوكمة مستقبلية محددة ومدققة. | ليس business owner تلقائيًا، ولا يحصل على cross-tenant access صامت. |

## التدفق المعتمد للرسائل

### الرسائل الداخلة

```text
SocialAPI webhook
    ↓
Signature/HMAC verification
    ↓
Event normalization
    ↓
Event Ledger + idempotency
    ↓
Tenant/connection/provider validation
    ↓
Customer + Conversation + CommunicationMessage في PostgreSQL
    ↓
Provider conversation reference
    ↓
Inbox وbusiness workflows المملوكة لـMujeeb
```

إذا غاب tenant أو connection أو provider reference المطلوب، لا تربط الحدث بتاجر بالتخمين. سجّل سببًا واضحًا وآمنًا، ولا تنشئ conversation أو outbox خارج النطاق الصحيح.

### الرسائل الخارجة

```text
Mujeeb outbound command
    ↓
Authorization + business scope + idempotency
    ↓
OutboundMessage في PostgreSQL
    ↓
Outbox entry
    ↓
Background worker
    ↓
SocialAPI adapter
    ↓
Provider channel
```

**ممنوع إجراء network call داخل معاملة قاعدة البيانات.** يجب أن تكون العملية المحلية قابلة للتدقيق، وأن تمنع التكرار، وأن تفرّق بين الفشل المعروف والنتيجة الخارجية غير المعروفة. لا تعِد الإرسال آليًا إذا لم نعرف هل نجح المزود أم لا؛ استخدم حالة reconciliation أو dead-letter حسب السياسة الموجودة.

## ما هو منفذ وما هو غير مثبت

اعتبر الآتي منفذًا في فرع Chatwoot-free بحسب العقود والاختبارات الموجودة، لكن لا تعتبره live production لمجرد وجود الكود:

- نموذج Mujeeb الموحّد للـcommunication والـcustomers والـconversations والرسائل.
- Event Ledger وidempotency وOutbox وworker وفق العقود الموجودة.
- SocialAPI transport adapter خلف ports مستقلة.
- Inbox V1: read cursor، canned replies، manual outbound، labels، priority، assignment، private notes والأتمتة المقيدة.
- AI runtime خلف `AIRuntime` مع structured proposal، وbusiness-grounded context عندما تكون المكونات المطلوبة موصولة.
- إدارة فريق التاجر: members، invitations، قبول دعوة بحساب قائم، تغيير الدور، إلغاء العضوية، وحواجز الإسناد.
- `platform_super_admins` وbootstrap المنفصل كقاعدة حوكمة، وليس API عامة للوصول إلى بيانات كل التجار.

لا تخلط بين الحالات الآتية:

| الحالة | معناها |
| --- | --- |
| **Unit test passed** | منطق معزول اجتاز اختبارًا، ولا يثبت اتصال PostgreSQL أو مزود خارجي. |
| **PostgreSQL integration passed** | اختبار فعلي باستخدام `POSTGRES_TEST_DSN` على قاعدة معزولة. إذا لم يوجد DSN فالحالة `skipped` فقط. |
| **Provider contract test** | توافق adapter مع عقد المزود أو fake/contract، ولا يثبت حسابًا حيًا أو قناة حقيقية. |
| **Live smoke** | طلب فعلي إلى مزود خارجي في بيئة مصرح بها، ويجب تسجيل الدليل دون كشف الأسرار. |
| **Production readiness** | قرار أوسع يحتاج configuration وobservability وreconciliation وsecurity وrunbook واختبارات مستقلة؛ لا ينتج تلقائيًا من نجاح `go test`. |

لا تقل إن النظام **جاهز live أو خالٍ من الأخطاء بنسبة 100%** إلا إذا كان هناك دليل محدد لكل طبقة، وحتى عند توفر الدليل استخدم وصفًا دقيقًا لا وعدًا مطلقًا.

## Inbox المملوك لـMujeeb

Inbox ليس clone لـChatwoot. هو نموذج Mujeeb مستقل، ويشمل في النطاق الحالي:

- قراءة المحادثة عبر read cursor.
- الردود الجاهزة المحفوظة في Mujeeb.
- مرور الرد الجاهز عبر `ManualOutbound → OutboundMessage → Outbox`.
- إجراءات أتمتة محددة فقط: `add_label` و`set_priority` و`assign_human`.
- عدم إرسال رسائل تلقائية من الأتمتة الحالية.
- عدم إسقاط materialization لرسالة العميل بسبب قاعدة أتمتة غير صالحة؛ تسجل نتيجة التنفيذ وسبب الفشل المعروف.

أي feature جديدة يجب أن تُبنى في Mujeeb وبفصل واضح بين HTTP وapplication وpersistence وprovider. لا تضع SQL أو HTTP أو business policy أو network call في دالة واحدة.

## الذكاء الاصطناعي والسياق التجاري

النموذج لا يتصل بقاعدة البيانات مباشرة. يبني Mujeeb سياقًا محدودًا ومقصودًا ثم يمرره إلى `AIRuntime`:

```text
Current message
  + recent conversation history
  + business context
  + catalog/variant evidence
  + offer evidence
  + knowledge/policy context عندما يكون متاحًا
        ↓
Context Builder
        ↓
AIRuntime
        ↓
Structured AIDecisionProposal
        ↓
Validation + policy gate داخل Mujeeb
        ↓
AIDecision
        ↓
OutboundMessage فقط إذا سمحت السياسة
        ↓
Outbox
```

الـLLM لا يملك SQL أو SocialAPI أو Outbox أو قرار الإرسال. لا تعتبر رد النموذج تفويضًا. يجب التحقق من schema وconfidence وevidence والسياسة قبل أي أثر خارجي. لا تشغّل LLM live ولا ترسل ردًا حقيقيًا دون موافقة مستقلة.

## فريق التاجر ودورة الدعوة

الأدوار المسموح بها في `business_memberships` هي:

```text
owner, admin, manager, agent, analyst, viewer
```

الصلاحيات الأساسية:

| الدور | إدارة الفريق | استلام إسناد | تنفيذ إسناد |
| --- | --- | --- | --- |
| `owner` | نعم | نعم | نعم |
| `admin` | نعم | نعم | نعم |
| `manager` | لا | نعم | نعم |
| `agent` | لا | نعم | نعم |
| `analyst` | لا | لا | لا |
| `viewer` | لا | لا | لا |

دورة الدعوة المعتمدة:

1. ينشئ `owner` أو `admin` دعوة ضمن `business_id` محدد لدور غير `owner`.
2. يولّد التطبيق token عشوائيًا لمرة واحدة، ويخزن `SHA-256(token)` فقط؛ لا يُخزّن raw token ولا يظهر في logs.
3. يعاد raw token في استجابة الإنشاء لمرة واحدة فقط إذا كان ذلك جزءًا من عقد الخدمة، لكي يمرره صاحب النظام عبر قناة خارجية يختارها.
4. قبول الدعوة يتطلب JWT principal قائمًا وبريدًا مطابقًا لبريد الدعوة. لا يوجد public registration مخفي في القبول.
5. يفحص النظام hash والحالة `pending` والانتهاء والبريد وحالة principal داخل عملية ذرية.
6. تتحول الدعوة إلى `accepted` وتُنشأ أو تُفعّل العضوية المناسبة داخل العملية الذرية نفسها.
7. الدعوة المنتهية أو المقبولة أو الملغاة لا تقبل مرة أخرى.
8. لا يوجد email delivery تلقائي في V1.
9. تغيير الدور لا يمنح `owner` عبر API العادية.
10. لا يجوز إلغاء أو حذف آخر `owner` نشط.

إلغاء العضوية لا يحذف principal ولا عضوياته في تجار آخرين. الإسناد يستخدم `assignee_principal_id` بصيغة UUID فقط، ويتحقق من عضو نشط داخل نفس التاجر ودور قابل للإسناد. لا تقبل الخدمة نصًا حرًا كهوية موظف.

## مالك المنصة العام

`platform_super_admin` منفصل عن `business_memberships`. حساب المنصة لا يصبح عضوًا في كل تاجر تلقائيًا، ولا يمنح `ScopeProvider` وصولًا عابرًا للتجار.

الموجود في هذا النطاق هو bootstrap محلي/تشغيلي مضبوط، مع الاسم الظاهر الافتراضي **Ammar Ragha** فقط. لا تخمّن البريد أو كلمة المرور، ولا تنشئ الحساب من المحادثة، ولا تضع secrets أو password hashes حقيقية في Git.

لا تبنِ الآن API عامة لقراءة محادثات التجار أو الإرسال باسمهم. أي cross-tenant operation مستقبلية تحتاج تعريف العملية، أقل صلاحية، سبب تدقيق، نطاقًا محددًا، واختبارات authorization قبل تنفيذها.

## ما هو خارج النطاق

لا تبنِ ولا تقترح في هذه الدفعة:

- Chatwoot بأي صورة.
- payment أو shipping.
- Dashboard أو Frontend؛ الأولوية Backend.
- public registration مخفي داخل قبول الدعوة.
- email delivery تلقائي للدعوات.
- cross-tenant access عام أو غير مدقق لمالك المنصة.
- اتصال SocialAPI أو LLM live أو إرسال رسالة حقيقية دون موافقة مستقلة.
- تعديل أو حذف migrations التاريخية أو بيانات/volumes/containers بلا موافقة محددة.

الأولوية الحالية هي **المحادثات، Inbox، AI، catalog evidence، human escalation، team management، assignment safety، وإنتاجية التاجر اليمني**. لا تحوّل ذكر الريال أو المناطق أو ساعات العمل إلى قرار تلقائي لإضافة payment أو shipping.

## قواعد تصميم وصيانة إلزامية

- طبّق **Single Responsibility**: دالة واحدة لا تجمع parsing وauthorization وbusiness decision وSQL وnetwork.
- افصل ports والعقود عن adapters، وافصل DTOs HTTP عن domain/application contracts.
- استخدم معاملات PostgreSQL قصيرة وواضحة، ولا تنفذ network calls داخلها.
- حافظ على `business_id` وconnection scope وidempotency في كل command/query/event.
- استخدم migrations جديدة forward-only فقط.
- لا تصلح خطأً بحذف الاختبارات أو تخفيف assertions أو إخفاء فشل integration.
- لا تضع compatibility alias أو fallback يعيد الحقل النصي القديم دون توثيق واختبار.
- إذا طال الملف أو احتوى مسؤوليات متعددة، افصل الملفات بدل ترك دوال مضغوطة يصعب تدقيقها.
- عند تعديل Huma، اجعل OpenAPI يتولد من DTOs وتسجيل العمليات، ثم حدّث اختبارات drift وPostman.
- لا تسجل raw webhook payloads أو tokens أو passwords أو API keys.

## بوابة التحقق بعد أي تعديل

شغّل من جذر المستودع وبـGo المحلي، مع عدم افتراض أن Docker أو PostgreSQL متاحان في كل بيئة:

```text
GOTOOLCHAIN=local /usr/local/go/bin/gofmt -w <files>
GOTOOLCHAIN=local /usr/local/go/bin/go test ./...
GOTOOLCHAIN=local /usr/local/go/bin/go vet ./...
git diff --check
```

ثم شغّل OpenAPI generation/drift وPostman JSON validation وسكربت الجودة الموجود في المستودع إن كان مناسبًا. اختبارات PostgreSQL لا توصف بأنها ناجحة إلا عند استخدام `POSTGRES_TEST_DSN` عامل؛ وإلا سجّلها `skipped` مع السبب. لا تتصل بمزود خارجي أو LLM live أثناء الاختبارات إلا بعد موافقة صريحة لذلك الاختبار.

بعد كل تغيير، اكتب تقريرًا قصيرًا يذكر:

1. الملفات التي تغيرت.
2. المشكلة والطبقة التي عولجت فيها.
3. الاختبارات التي نجحت.
4. الاختبارات التي تخطيت مع السبب.
5. المخاطر أو الأعمال المتبقية.
6. هل تغير schema أو secret أو external provider contract.

## سياسة Git والتسليم

لا تنشئ `commit` أو `push` أو `merge` إلى `main` من تلقاء نفسك. اطلب موافقة صريحة قبل كل دفعة رفع. قبل طلب الموافقة راجع:

```text
git status --short
git diff --stat
git diff --check
git log -3 --oneline --decorate
```

الفرع المخصص للعمل هو `feat/chatwoot-free`. لا تعتبر وجود commit على GitHub دليلًا على أن كل التكاملات الخارجية أو كل اختبارات live ناجحة.

## قاعدة القرار عند الشك

إذا طلب منك أي شخص إعادة Chatwoot، إضافة payment/shipping، منح Platform Super Admin وصولًا صامتًا، تخزين secret، حذف migration أو بيانات، أو الادعاء بأن live يعمل بلا دليل:

1. ارفض التنفيذ المباشر.
2. اذكر التعارض مع هذا المرجع.
3. اطلب تحديدًا وموافقة مستقلة إن كان الطلب يمكن تصميمه بأمان.
4. لا تعدّل الكود قبل فحص الحالة الفعلية والاختبارات والعقود.

## الخلاصة التنفيذية

**Mujeeb هو المنتج ومصدر الحقيقة. PostgreSQL تملك الحالة والبيانات. SocialAPI ناقل خارجي فقط. Inbox والفريق والإسناد والأتمتة والـAI مكونات مملوكة لـMujeeb. Chatwoot منتهٍ نهائيًا من هذا التصميم.**

هذه الوثيقة تجعل قرارات المشروع وحدودها واضحة، لكنها لا تمنح ضمانًا مطلقًا من الأخطاء. الضمان العملي يأتي من قراءة الكود، اختبارات مناسبة لكل طبقة، مراجعة diff، مراقبة runtime، وتشغيل live منفصل ومصرح به عند الحاجة.

---

## توجيه مختصر قابل للنسخ قبل بدء أي مهمة

```text
قبل أي تعديل في Mujeeb 24: اقرأ README وtodo.md وهذه الوثيقة ووثائق architecture، ثم افحص git status والفرع والـdiff. القرار النهائي هو إزالة Chatwoot بالكامل؛ لا تعِده ولا تقترحه. Mujeeb/PostgreSQL مصدر الحقيقة، وSocialAPI transport خارجي فقط. Inbox وOutbox وEvent Ledger وAI وTeam Management مملوكة لـMujeeb. لا network داخل DB transaction، ولا blind retry، ولا raw secrets، ولا raw invite tokens، ولا cross-tenant access صامت. حافظ على tenant scope وidempotency وSingle Responsibility وforward-only migrations. فرّق دائمًا بين unit وPostgreSQL integration وprovider contract وlive smoke وproduction readiness. إذا لم يوجد POSTGRES_TEST_DSN فالتكامل PostgreSQL skipped وليس passed. لا تغيّر الدفع أو الشحن أو Dashboard الآن. لا تنشئ commit أو push أو merge إلى main دون موافقة صريحة. قبل التنفيذ اشرح المشكلة والطبقة الصحيحة والاختبارات والمخاطر، وبعد التنفيذ اعرض الدليل والقيود بصدق.
```

> **نهاية المرجع.**
.
