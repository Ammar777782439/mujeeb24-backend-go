# عقود Mujeeb 24 للذكاء الاصطناعي — فهرس المصادر المعتمدة

> **المصادر الحالية لمساعد كتالوج التاجر على فرع PR #18:** [عقد Authoring](11-merchant-catalog-ai-authoring-closed.md)، [Proposal V3](12-merchant-catalog-ai-proposal-contract-closed.md)، [Key/Label/Value والقوالب وقواعدها](../../docs/architecture/merchant-catalog-attribute-schema-ai-contract-ar.md)، و[دليل مزود AI](../../docs/architecture/adding-ai-provider-runbook-ar.md). 
>
> احتفظ بالعقود المغلقة الخاصة بالاستخدامات الأخرى كما هي، ولا تستخدم أمثلة Gemini القديمة بوصفها تعليمات تشغيل حالية لـMerchant Catalog. أي تعارض في هذا المسار يحسمه عقد Merchant الحالي والكود المتوافق معه بعد مراجعة القرار وتوثيقه في ADR.

## الترتيب المنطقي للعقود

كل عقد يبني على الذي قبله، ويجب قراءتها بالترتيب التالي:

| # | العقد | الغرض | الحالة |
|---|------|------|------|
| ① | [Catalog AI Projection v1](./01-catalog-ai-projection-v1-closed.md) | شكل البيانات الذي يراه Gemini من الكتالوج | مغلق |
| ② | [Catalog Evaluation + Batching](./02-catalog-evaluation-batching-closed.md) | كيف نضمن أن Gemini يفحص كل العناصر دون semantic search | مغلق |
| ③ | [Conversation Context Contract](./03-conversation-context-contract-closed.md) | ماذا يحتاج Gemini لفهم سياق العميل | مغلق |
| ④ | [Gemini System Contract + AI I/O](./04-gemini-system-contract-ai-input-output-closed.md) | ماذا نقول لـ Gemini وماذا يرجع لنا | مغلق |
| ⑤ | [Catalog Entity Contract + Data Boundary](./05-catalog-entity-contract-catalog-data-boundary-closed.md) | الفصل بين تعريف الكيانات وبيانات التاجر | مغلق |
| ⑥ | [AI Validation + Authorization Boundary](./06-ai-validation-authorization-boundary-closed.md) | ماذا يحدث بعد أن يقترح Gemini | مغلق |
| ⑧ | [Observability + Audit + AI Trace](./08-observability-audit-ai-trace-closed.md) | كيف نتتبع كل خطوة | مغلق |
| ⑨ | [AI Runtime Lifecycle + Failure/Retry/Timeout](./09-ai-runtime-lifecycle-failure-retry-timeout-closed.md) | دورة تشغيل AI Run واحدة | مغلق |
| 11 | [Merchant Catalog AI Authoring — الحالي](./11-merchant-catalog-ai-authoring-closed.md) | النموذج يصوغ المواصفات بالعربية ويختار القوالب؛ Go يتحقق ويحفظ بعد اعتماد التاجر | **مرجع Merchant الحالي** |
| 12 | [Merchant Catalog AI Proposal V3](./12-merchant-catalog-ai-proposal-contract-closed.md) | Canonical Proposal بعد النقل المنظم وعلاقات Variant/Offer وتعريفات المواصفات | **مرجع Merchant الحالي** |

| 13 | [قوالب المواصفات العربية: Key / Label / Value](../../docs/architecture/merchant-catalog-attribute-schema-ai-contract-ar.md) | AttributeSchema/AttributeDefinition/validation_rules/Variant JSONB وStrict Wire | **مرجع Merchant الحالي** |

> **ملاحظة:** العقود ⑦ و ⑩ و ⒛ خارج نطاق هذه المجموعة أو غير معتمدة بعد.

## المبدأ الحاكم

```
نموذج AI يفهم ويقترح ضمن الأدلة والعقود.
Go يتحقق ويثبت الهوية ويؤذن وينفذ بعد موافقة التاجر.
React تعرض label من تعريف المواصفة ولا تترجم.
```

- لا يوجد semantic search داخل Mujeeb
- لا يوجد `search_catalog(query)` نهائيًا
- لا يوجد `max 6 rounds` أو عدد ثابت للـ batches/retries في Domain
- لا يقرر Gemini: `requires_approval` ولا `business_id` ولا `tenant_id`
- الـ Business يأتي من Authenticated Context فقط
- AI Run ≠ Conversation State ≠ Business Truth
- لا توجد قيم `requires_approval`/`authorized`/`executed` في مخرج Gemini
- Lead وOrder يبقيان draft حتى Authorization

## العلاقة بين الكود والعقود

| طبقة الكود | العقد المرتبط |
|------|------|
| `internal/application/ports/ai_runtime.go` | ③ ④ ⑨ |
| `internal/application/ports/ai_capability.go` | ① ⑤ ⑧ |
| `internal/application/ports/ai_audit_repositories.go` | ⑧ ⑨ |
| `internal/application/ports/conversation_state.go` | ③ |
| `internal/application/services/ai_run_lifecycle.go` | ⑨ |
| `internal/application/services/ai_runtime_retry.go` | ⑨ |
| `internal/application/services/ai_validation_pipeline.go` | ⑥ |
| `internal/application/services/ai_context_builder.go` | ③ |
| `internal/application/services/auto_reply.go` | ④ ⑥ |
| `internal/application/services/policy_engine.go` | ⑥ |
| `internal/application/merchantcatalogai/agent.go` | 11 12 |
| `internal/application/services/catalog_batch_controller.go` | ② ⑨ |
| `internal/application/services/catalog_entity_contract.go` | ⑤ |
| `internal/adapters/secondary/ai/gemini/client.go` | ③ ④ ⑧ |
| `internal/adapters/secondary/persistence/postgres/ai_*_repositories.go` | ⑧ ⑨ |
| `migrations/000055_ai_runs.up.sql` وما بعدها | ⑧ ⑨ |

## قاعدة تغيير العقود

لا يُعاد فتح عقد مغلق إلا عبر:
1. اختبار فاشل مثبت.
2. قيد Provider موثق من توثيق Google الرسمي.
3. مشكلة تشغيل حقيقية.
4. متطلب تجاري جديد.
5. تكلفة/مخاطر مثبتة.

أي تغيير يجب أن يُسجل كقرار ADR جديد في `docs/decision-log-ar.md`.


### قاعدة عقد Proposal الوسيط

العقد 12 يملك العلاقات المؤقتة داخل Proposal (`variant_ref`) ولا يغيّر Domain Catalog Contract. أي تغيير في هذه العلاقة يجب أن يمر عبر ADR جديد.
