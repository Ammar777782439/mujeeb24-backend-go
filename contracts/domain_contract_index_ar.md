# Mujeeb 24 Domain Contract — Index

## الحالة

تمت مراجعة وإغلاق الاتجاه الدوميني الأولي لمجيب 24 قبل كتابة Repositories أو Dashboard API أو AI Provider integration.

```text
shared + business                 ✅
channel + identity + communication ✅
catalog + sales                    ✅
ai + audit                         ✅
application/ports                  ⏳ التالي
migrations                         ⏳ بعد Ports
reliability foundation             ⏳ بعد migrations
```

## العقود المغلقة

| الجزء | القرار المركزي |
|---|---|
| `domain/shared` | IDs داخلية منفصلة عن Provider IDs، Money مضبوط، UTC، Domain Errors typed |
| `domain/business` | Business Aggregate مستقل، vertical default لا قيد، lifecycle واضح، لا يعتمد على Chatwoot/SocialAPI |
| `domain/channel` | Connection وCapabilities وInboundEvent وlifecycle، بلا secrets داخل Domain |
| `domain/identity` | Customer داخل Mujeeb وExternalIdentity للقناة، لا دمج بالاسم وحده |
| `domain/communication` | ConversationReference وMessageReference فقط، لا إعادة بناء Chatwoot |
| `domain/catalog` | CatalogItem وOffer وVariant وTyped Attributes، pricing/availability states، schema version |
| `domain/sales` | Lead وCommercialTransaction بأنواعها، details/lifecycles/snapshots |
| `domain/ai` | Structured AIDecision، evidence، policy، confidence، actions محدودة، AI يقترح ولا ينفذ |
| `domain/audit` | AuditEvent append-only، correlation/causation، بلا secrets أو raw payloads |

## Dependency direction

```text
Primary Adapter
      ↓
Application Use Case
      ↓
Domain

Application → Ports
Secondary Adapters → Ports
```

Domain لا يستورد أي Provider SDK أو Chatwoot DTO أو Fiber أو PostgreSQL أو Redis.

## القرارات التي تمنع الأخطاء الكبرى

لا نستخدم Product كجذر Universal Catalog. لا نستخدم Order كمعاملة عامة. لا نحول Unknown إلى Available أو Confirmed. لا نعيد إرسال رسالة بعد نتيجة خارجية غامضة دون reconciliation. لا نستخدم Chatwoot Authorization بدل Mujeeb Authorization. ولا نجعل AI يملك side effects مباشرة.

## ما لم نكتبه بعد

لم نكتب Repository أو Handler أو Provider API implementation أو SQL migration. ملفات Go الحالية في المستودع هي Foundation minimale فقط للتحقق من module/package boundaries. التنفيذ التالي يبدأ بـValue Objects وEntities وConstructors واختبارات invariants داخل Domain، ثم `application/ports`.

## الملفات التفصيلية

- `domain_shared_business_contract_ar.md`
- `domain_channel_identity_communication_contract_ar.md`
- `domain_catalog_sales_contract_ar.md`
- `domain_ai_audit_contract_ar.md`
