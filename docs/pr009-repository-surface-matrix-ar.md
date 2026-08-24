# PR-009 — Repository Surface Matrix

## الهدف

لا نبني عشرات methods على كل جدول لمجرد أن الجدول موجود. نضيف فقط ما تحتاجه Application Use Cases الحالية، ونثبت كل مجموعة باختبار PostgreSQL حقيقي قبل الانتقال للمجموعة التالية.

## الأولوية الحالية

| المجموعة | Use Cases المرتبطة | الجداول الأساسية | الحالة |
|---|---|---|---|
| Foundation | قراءة tenant/business scope | `businesses` | منفذ |
| Identity/Communication | عرض العملاء والمحادثات، وقراءة/إنشاء outbound message حيث يسمح schema | `customers`, `conversations`, `conversation_references`, `outbound_messages` | Business/Customer/Conversation/Connection/Reference/Outbound read-create foundation منفذ؛ لا يوجد جدول عام للرسائل الواردة/الداخلية |
| Channels | عرض connection وcapabilities | `channel_connections`, `channel_connection_capabilities` | قراءة أساسية منفذة للـconnection فقط؛ capabilities لاحقًا |
| Catalog | عرض/إنشاء/تعديل catalog وitems/offers/variants | `catalogs`, `attribute_schemas`, `attribute_definitions`, `catalog_items`, `offers`, `variants` | مؤجلة بعد Communication |
| Sales | Leads وtransactions وreviews/order lines | `leads`, `lead_attributions`, `lead_scores`, `commercial_transactions`, `transaction_reviews`, `transaction_confirmations`, `order_lines` | مؤجلة بعد Catalog |
| AI/Audit | قراءة decisions وتسجيل/عرض audit | `ai_decisions`, `audit_events` | مؤجلة بعد Sales |
| Reliability | atomic inbound dedupe وoutbox | `inbound_event_ledger`, `outbox_entries` | لا تُنفذ قبل اكتمال repository foundation؛ EventStore يملك inbound idempotency |

## Communication surface المنفذ

تم تنفيذ ports وrepositories للـConversationReference وOutboundMessage فوق SQLExecutor. `CreatePending` يفرض الحقول اللازمة من schema ويترك unique constraint `(provider_ref, connection_id, provider_idempotency_key)` مصدر conflict semantics؛ لا ينفذ network call ولا يرسل Provider.

## Methods المسموح بها في الدفعة التالية

Communication الحالية تشمل `GetCustomer` و`GetConversation` وقراءة current reference و`CreatePending/GetByID` للـoutbound. أما `ListConversationMessages` فلا يُنفذ كقراءة كاملة قبل حسم جدول الرسائل، لأن schema الحالية لا تحتوي `messages` أو `communication_messages`; `conversation_references` ليست بديلًا عن message timeline. لا نضيف update/list methods غير المطلوبة من Application contracts الحالية.

كل method يستقبل `context.Context` وbusiness scope حيث يلزم، ويستخدم `SQLExecutor` من PostgreSQL adapter. لا يرجع pgx rows أو HTTP DTOs إلى Application؛ يعيد record types مستقلة، والـJSON المرن يبقى raw bytes عند الحاجة.

## قواعد إلزامية

يجب أن يستخدم أي query يقرأ سجلًا tenant-scoped شرط `business_id` مع record ID أو cursor المناسب. يجب ألا يعتمد repository على ID وحده عندما يسمح schema بحدود composite. ويجب أن تعمل methods داخل TransactionManager عندما يستدعيها Use Case داخل transaction، دون بدء nested transaction مخفية.

كل خطأ يعبر من adapter إلى Application عبر typed repository error، مع تحويل واضح لـnot-found وconstraint/conflict وinvalid input. لا يُستخدم `map[string]any` كبديل عن record contract؛ يُسمح فقط بالـraw JSON في الحقول المرنة المعتمدة.

## معيار قبول كل مجموعة

لا تنتقل المجموعة إلى التالية قبل نجاح unit tests وintegration test على PostgreSQL حقيقي، مع تطبيق migrations، وقراءة صحيحة، وnot-found، وcross-tenant rejection، وسلوك transaction عند الحاجة. بعد تثبيت surfaces الموجودة فعليًا في Communication/Catalog/Sales/AI/Audit فقط نبدأ EventStore ثم inbound dedupe ثم Outbox. وأي حاجة إلى inbound/internal message timeline تتطلب قرار schema موثقًا قبل adapter، ولا تُحل باختراع repository فوق جدول غير موجود.
