# Mujeeb Inbox V1: قدرات مملوكة لـMujeeb

**الحالة:** منفذ على الفرع المحلي `feat/chatwoot-free` بانتظار التحقق على PostgreSQL حقيقية قبل commit أو رفع جديد. هذه الوثيقة لا تنسخ خدمة أو كودًا خارجيًا؛ تستبقي فقط قدرات يحتاجها التاجر ضمن نموذج Mujeeb وPostgreSQL.

## الجرد الحالي

Mujeeb يملك حاليًا قائمة وتفاصيل المحادثات، حالة المحادثة وملكيتها وأولويتها، الإسناد إلى موظف، labels، الملاحظات الخاصة، timeline للرسائل، والرد اليدوي الذي ينشئ `OutboundMessage` و`Outbox`. كما يملك AutoReply مقيدًا بالسياسات في مسار SocialAPI inbound.

الفجوات العملية هي: علامة قراءة شخصية لكل مستخدم، ردود جاهزة قابلة للإدارة، طريقة آمنة لتطبيق رد جاهز كرسالة outbound، وقواعد أتمتة محدودة وواضحة لا تؤدي إلى حلقات أو إرسال غير مضبوط.

## نطاق الإصدار الأول

| القدرة | السلوك المقصود | مصدر الحقيقة | أثر خارجي |
| --- | --- | --- | --- |
| قراءة المحادثة | يسجل كل principal آخر رسالة/وقت قرأه في محادثة ضمن نفس business | `conversation_read_cursors` | لا يوجد |
| الردود الجاهزة | ينشئ التاجر ردًا بعنوان واختصار ونص وحالة تفعيل | `canned_replies` | لا يوجد عند الحفظ |
| تطبيق رد جاهز | يتحقق من ownership والـversion ثم ينشئ رسالة outbound عبر المسار الحالي | `outbound_messages` + `outbox_entries` | worker فقط بعد commit |
| قواعد الأتمتة | trigger محدود لرسالة واردة جديدة، وشروط/action محددة schema-first | `automation_rules` + `automation_executions` | لا network داخل transaction |
| أتمتة الرسالة الواردة | تقرأ القواعد الفعالة بالترتيب؛ تنفذ تغييرًا داخليًا معلومًا أو تسجل الفشل | Mujeeb transaction | لا يوجد |

## قواعد الأمان

الأتمتة لا تنفذ على رسائل outbound أو private أو duplicate أو حدث delivery status. تستعمل `inbound_event_id` في قيد dedupe الفريد لكل قاعدة وحدث، ولا تعيد محاولة نتيجة provider غير معروفة عميانيًا. لا تتصل بـSocialAPI ولا بأي نموذج لغوي داخل transaction.

الـactions المنفذة في هذه الدفعة هي فقط: `add_label` و`set_priority` و`assign_human`. لا توجد قاعدة `send_message` أو `send_canned_reply`؛ فالردود الجاهزة يرسلها مستخدم مصادق عليه فقط عبر `OutboundMessage` و`Outbox` الموجودين. تفصل هذه الحماية بين أتمتة تصنيف/توجيه آمنة وبين إرسال آلي يحتاج مواصفة Policy مستقلة. فشل action يُسجَّل كـ`failed` وسبب معلوم ولا يمنع حفظ الرسالة الواردة.

## نموذج البيانات المقترح

تضاف migrations forward-only بعد `000045` فقط. لا يُعدّل schema تاريخي ولا يُسقط أي جدول أو عمود قائم.

| الجدول | المفاتيح المهمة | قيد tenant والاتساق |
| --- | --- | --- |
| `conversation_read_cursors` | `business_id`, `conversation_id`, `principal_id`, `last_read_message_id`, `read_at` | مفتاح مركب لكل principal ومحادثة؛ FK مركب للمحادثة ونطاق business |
| `canned_replies` | `id`, `business_id`, `title`, `shortcut`, `body`, `status`, `resource_version` | اختصار فريد لكل business، نص غير فارغ، status مضبوط |
| `automation_rules` | `id`, `business_id`, `name`, `trigger_kind`, `conditions`, `action_kind`, `action_payload`, `status`, `position`, `resource_version` | JSONB محدد في application، status/trigger/action مقيدة، ترتيب فريد لكل business |
| `automation_executions` | `id`, `business_id`, `rule_id`, `inbound_event_id`, `result`, `reason_code`, `created_at` | dedupe فريد للقاعدة والحدث؛ audit واضح بلا retry أعمى |

## العقود العامة

تضاف عمليات HTTP عامة لمستخدمين مصادق عليهم وضمن business scope فقط:

```text
POST  /businesses/{business_id}/conversations/{conversation_id}/read
GET   /businesses/{business_id}/canned-replies
POST  /businesses/{business_id}/canned-replies
PATCH /businesses/{business_id}/canned-replies/{canned_reply_id}
POST  /businesses/{business_id}/conversations/{conversation_id}/canned-replies/{canned_reply_id}/send
GET   /businesses/{business_id}/automation-rules
POST  /businesses/{business_id}/automation-rules
PATCH /businesses/{business_id}/automation-rules/{automation_rule_id}
```

تتطلب عملية إرسال رد جاهز `Idempotency-Key` لأنها تنشئ `OutboundMessage` و`Outbox`. تظل executions idempotent بقيد `(business_id, rule_id, inbound_event_id)`؛ وتستخدم تحديثات الردود والقواعد `If-Match`/`resource_version` بالأسلوب الحالي في المشروع.

## ما لا يدخل هذه الدفعة

لا تدخل في هذا الإصدار: widget ويب، بريد أو صوت، حملات تسويقية، triggers مجدولة، expression engine حر، أو semantic search. تلك قدرات مستقلة تحتاج مواصفات تشغيلية واختبارات ومعالجة مخاطرة منفصلة.
