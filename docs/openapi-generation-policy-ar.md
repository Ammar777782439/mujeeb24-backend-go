# سياسة توليد OpenAPI من Go DTOs

## القرار

في Mujeeb 24 V1 تكون **Go HTTP DTOs وتعريفات العمليات المسجلة** هي مصدر الحقيقة لعقد Dashboard API. لا يُكتب OpenAPI يدويًا كمصدر مستقل، ولا تُعدّل النسخة المولدة مباشرة.

يستخدم المشروع Huma v2.32.0 مع Go 1.22.2. تحتوي حزمة `internal/adapters/primary/http/dto` أنواع Input وOutput وحقول path/query/header/body، بينما تسجل حزمة `internal/adapters/primary/http/contract` العمليات وmetadata باستخدام DTOs تلك. يولد الأمر `cmd/openapi-gen` ملف OpenAPI 3.0.3 من هذا المصدر البرمجي المشترك.

## دورة التوليد

```text
http/dto DTOs + http/contract operation registration
        ↓
contract.BuildAPI()
        ↓
Huma OpenAPI model
        ↓
DowngradeYAML()
        ↓
api/openapi/mujeeb24-dashboard-v1.generated.yaml
```

التوليد الرسمي:

```bash
./scripts/generate-openapi.sh
```

أو:

```bash
go generate ./internal/adapters/primary/http/contract
```

## Drift Check

يفشل الفحص إذا اختلف الملف المولد المحفوظ عن الناتج الحالي:

```bash
./scripts/check-openapi-generated.sh
```

ويُشغل هذا الفحص مع `go test ./...` في مسار CI المختار للمشروع. لا يعتمد التوليد على وجود GitHub Actions بعينه؛ الأمر والـdrift check هما المصدر القابل لإعادة الاستخدام.

## الحدود

توليد OpenAPI لا ينفذ Application Commands ولا يتصل بـPostgreSQL أو SocialAPI أو Chatwoot. الـDTOs منفصلة عن contract registration، والـHandlers مسؤولة عن تحويل DTO إلى Application Command أو Query. لا تظهر في DTOs أي Provider secrets أو raw payloads أو LLM Chain-of-Thought.

Webhooks مسجلة كحدود HTTP مستقلة بدون JWT Dashboard، وتحتاج لاحقًا signature verification وdurable Event Ledger داخل التنفيذ الفعلي.

## قاعدة التغيير

عند تغيير Endpoint أو Request/Response DTO أو status أو security:

1. عدّل Go DTO أو operation registration.
2. شغّل generator.
3. شغّل drift check و`go test ./...`.
4. راجع generated artifact.
5. ارفع المصدر والتوليد معًا في commit واحد.

ملف OpenAPI المولد Artifact قابل للقراءة والتوزيع، لكنه ليس مكان التصميم الأول.
