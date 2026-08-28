# قرار التصميم: Mujeeb بلا Chatwoot

**الحالة:** مقترح منفذ على الفرع `feat/chatwoot-free` فقط. لا يغير `main` ولا يرسل أو يحذف أي بيانات خارجية.

## القرار

Mujeeb وPostgreSQL هما مصدر الحقيقة الوحيد للمحادثات والعملاء والرسائل والقنوات. لا يُشغَّل Chatwoot ولا تُنشأ له Accounts أو Inboxes ولا تُرسل إليه نسخ mirror. SocialAPI هو transport للقنوات فقط، وDashboard Mujeeb هو واجهة فريق التاجر الوحيدة.

```text
SocialAPI webhook
  -> Mujeeb verification + event ledger
  -> Mujeeb Customer + Conversation + CommunicationMessage
  -> Mujeeb Dashboard API

Dashboard Mujeeb
  -> OutboundMessage + Outbox
  -> Worker
  -> SocialAPI send
```

## ما يبقى ملكًا لـMujeeb

| قدرة منتجية | الأساس في Mujeeb | قرار الفرع |
| --- | --- | --- |
| العميل وهويته الخارجية | `customers`, `external_identities` | يحتفظ بها |
| المحادثة وحالتها وملكية التاجر | `conversations`, `conversation_references` | يحتفظ بها؛ يعتمد runtime على provider فقط |
| الرسائل وتاريخها والخصوصية | `communication_messages` مع اتجاه/visibility | يحتفظ بها |
| tags والتصنيف | `conversation_labels` | يحتفظ بها |
| ملاحظات داخلية | رسائل private داخل Mujeeb | يحتفظ بها |
| الإسناد والأولوية والحالة | Conversation runtime في Mujeeb | يحتفظ به |
| الردود الجاهزة وmacros | Dashboard/API في Mujeeb لاحقًا | لا يعاد بناء Chatwoot؛ تنفذ كميزات Mujeeb منفصلة عند الحاجة |
| التحليلات وSLA والأتمتة | Event ledger وoutbox وaudit | تبنى من بيانات Mujeeb عند الحاجة |

## ما يزال خارج النطاق عمدًا

لا تضيف هذه الدفعة واجهة Dashboard جديدة، أو ردودًا مباشرة من Chatwoot، أو AI جديدًا، أو إرسال SocialAPI حيًا. كما لا تعيد تنفيذ كل مزايا Chatwoot: widget الويب، البريد، الصوت، حملات التسويق، أو نظام help center ليست بدائل لازمة لمسار رسائل التاجر الحالي.

## خطة الإزالة

1. يحذف فرع `feat/chatwoot-free` جميع متغيرات Chatwoot وadapters والـroutes والـworkers وCompose services.
2. يتحول Merchant Channel Provisioning إلى SocialAPI-only: ينشئ ChannelConnection بعد OAuth ولا ينشئ Chatwoot Account أو Inbox أو binding.
3. يحذف مسار mirror فقط؛ لا يتغير SocialAPI webhook materialization إلى Customer/Conversation/CommunicationMessage.
4. تبقى migrations `000001`–`000045` كما هي. لا تُضاف في هذه الدفعة migration إسقاط للحقول أو الجداول التاريخية، لأن ذلك تغيير بيانات مدمر ويستلزم backup وموافقة صريحة مستقلة.
5. تحدّث Postman وOpenAPI وREADME وCompose إلى PostgreSQL + API + worker فقط.
6. لا يتم دمج الفرع أو رفعه إلا بعد نجاح `go test ./...` و`go vet ./...` وOpenAPI checks وPostgreSQL integration checks.

## حواجز صحة غير قابلة للتنازل

- لا تدخل provider payload إلى Dashboard أو Domain مباشرة.
- لا يوجد network call داخل transaction PostgreSQL.
- تظل inbound deduplication وoutbox durable.
- لا تنشأ أو تعدل موارد SocialAPI في الاختبارات.
- لا توضع secrets في source أو Compose tracked files.
- لا تحذف migrations السابقة؛ أي إزالة schema تكون في migration جديد forward-only.

## سياسة schema التاريخي وrollback

الـruntime لا يقرأ أو يكتب جداول أو أعمدة Chatwoot التاريخية بعد هذه الدفعة. تبقى تلك البنى مؤقتًا لأن المهاجرات السابقة غير قابلة للتعديل ولأن إسقاطها قد يحذف سجلات تشغيلية من بيئات قائمة. لا توجد migration رقم `000046` في هذا الفرع لهذه الغاية.

عند طلب تنظيف schema لاحقًا، يبدأ العمل بجرد البيانات ونسخة احتياطية قابلة للاستعادة وخطة تحقق، ثم migration forward-only منفصلة. لا يشغّل هذا الفرع أي أمر `docker compose down -v` أو SQL `DROP` أو حذف containers/volumes أو موارد SocialAPI تلقائيًا.

إذا ظهر خلل في تشغيل النسخة Mujeeb-only، فالرجوع الآمن هو تشغيل checkpoint/commit سابق أو العودة إلى الفرع السابق بعد قرار مراجعة؛ لا يُعالج عبر تعديل migrations التاريخية أو حذف بيانات الإنتاج.
