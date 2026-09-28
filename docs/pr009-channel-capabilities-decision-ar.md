# PR-009 — قرار Channel Capabilities

## حالة التنفيذ

**منفذ ومرفوع إلى private GitHub بعد اجتياز اختبارات PostgreSQL 16 والتحقق النهائي.** Commit SHA هو `d05fa920132e5aeae40b267aee2dccf1f519355d`. هذه الدفعة لا تفتح قرارًا جديدًا في CommunicationMessage أو EventStore؛ هي تكمل repository surface الخاص بـChannel Capabilities فقط.

## Contract evidence

| المصدر | الدليل |
|---|---|
| Domain Channel contract | كل Capability typed تحمل `name`, `enabled`, `checked_at`, و`source`، و`enabled=false` تعني Unsupported أو غير مكتشفة، لا failure مؤقتًا تلقائيًا |
| PostgreSQL migration `000004` | الجدول يملك `connection_id`, `capability`, `enabled`, `checked_at`, `evidence_source` مع PK `(connection_id, capability)` وenum constraint |
| Application query contract | `GetConnectionCapabilitiesQuery` و`GetConnectionCapabilitiesHandler` موجودان، ويعيدان `ListResult[ConnectionCapabilityView]` |
| HTTP Dashboard contract | `GET /businesses/{business_id}/channel-connections/{id}/capabilities` هو use case قراءة للـmembers، success `200` |
| Existing HTTP façade | route يفرض business scope ثم يستدعي typed dependency؛ لا SQL أو Provider call داخل handler |

## القرار

السطح المطلوب الآن هو **قراءة capabilities فقط**. لا نضيف Create/Update/Delete repository methods لأن capabilities نتيجة provider contract/health checks وليست إعدادًا يحرره التاجر من Dashboard. تحديثها يأتي لاحقًا من integration/provider health path ويجب أن يمر عبر Application policy المناسبة.

بسبب أن جدول capabilities لا يحمل `business_id` مستقلًا، يثبت repository ownership عبر `channel_connections(business_id,id)`. أي lookup يستخدم business وconnection معًا، والـconnection المفقود أو الموجود في Business آخر يعيد `RepositoryNotFound` typed ولا يكشف rows.

## Typed Application contract

`ChannelCapabilityRecord` يحتوي:

| الحقل | المعنى |
|---|---|
| `ConnectionID` | Connection المالك |
| `Name` | اسم capability من enum المهاجر، بما فيه `media_inbound` و`media_outbound` |
| `Enabled` | capability متاحة أو غير مكتشفة وفق contract |
| `CheckedAt` | وقت آخر check |
| `EvidenceSource` | مرجع الدليل الاختياري، وليس raw provider payload |

أضيف `CheckedAt` إلى `queries.ConnectionCapabilityView` حتى لا يسقط Application حقلًا موجودًا في schema وHTTP DTO. `ConnectionCapabilitiesQueryService` يحول record إلى view typed، والـhandler يخرجه إلى DTO/Huma مع `Name`, `Enabled`, `CheckedAt`, و`EvidenceSource`.

## قرار أسماء media

كان Domain النصي يستخدم `media` كاسم تجميعي، بينما migration الفعلية تعتمد `media_inbound` و`media_outbound`. تم توحيد Domain contract على الاسمين الدقيقين لأن اتجاه النقل جزء من قرار capability ولا يجوز فقده في repository أو Dashboard projection. لم تُعدّل migration `000004`.

## Repository implementation

`internal/adapters/secondary/persistence/postgres/channel_capability_repository.go` ينفذ `ListByConnection` فوق `SQLExecutor`. يبدأ بفحص tenant-scoped لوجود connection، ثم يقرأ rows بترتيب `capability ASC`. إذا استُدعي داخل `TransactionManager.Within` يستخدم transaction context نفسه؛ لا يبدأ transaction مخفية ولا ينفذ network calls.

## اختبارات القبول

Integration test على PostgreSQL 16 يثبت:

- insert/read لثلاث capabilities؛
- الترتيب deterministic؛
- `checked_at` و`evidence_source` و`enabled` mapping؛
- cross-tenant lookup كـtyped `RepositoryNotFound`؛
- Application `ConnectionCapabilitiesQueryService` mapping؛
- التعديل داخل transaction ورؤيته قبل commit؛
- ظهور commit بعد نجاح transaction؛
- rollback وعدم تسرب التعديل بعد فشل callback.

الاختبارات العامة تثبت أيضًا migration runner `applied=28` ثم `applied=0` وschema validation الكامل، لأن هذه الدفعة لا تضيف migration جديدة.

## الحدود غير المنفذة

لا تشمل هذه الدفعة provider capability discovery أو SocialAPI أو Chatwoot calls، ولا تحديث capabilities من Dashboard، ولا Catalog/Sales repositories، ولا EventStore أو Outbox. تظل هذه العناصر ضمن المراحل اللاحقة بعد إكمال repository surfaces المطلوبة.
