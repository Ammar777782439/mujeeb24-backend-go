# Mujeeb 24 Backend

Backend مستقل لمنصة **مجيب 24**: نظام تشغيل للمحادثات والمبيعات متعدد التجار، مبني بـGo وPostgreSQL.

> **لا تفوّت عميلك، ولو كنت مشغولًا.**

## ابدأ من هنا

إذا فتحت المستودع لأول مرة، ابدأ بالملفات الآتية:

```text
1. docs/architecture/chatwoot-free-migration-ar.md
2. docs/project-status-ar.md
3. docs/developer-guide-ar.md
4. docs/decision-log-ar.md
5. contracts/domain_contract_index_ar.md
```

## مسؤوليات النظام

يمتلك Mujeeb دورة البيانات التجارية كاملة، من استلام حدث القناة حتى حفظ المحادثة والرد الخارجي:

| الطبقة | المسؤولية |
| --- | --- |
| `Mujeeb Go + PostgreSQL` | مصدر الحقيقة التجاري، العملاء، المحادثات، الرسائل، الكتالوج، القرارات، التدقيق، Event Ledger وOutbox |
| `SocialAPI` | ناقل القنوات وOAuth والـwebhook والإرسال الخارجي |
| `Mujeeb API` | عقود API مع عزل tenant والتحقق والمصادقة |
| `Mujeeb worker` | معالجة Outbox وإرسال الرسائل إلى SocialAPI خارج معاملات قاعدة البيانات |

التاجر يتعامل مع Mujeeb فقط. لا تُخزّن DTOs للمزود كحقيقة تجارية، ولا يملك نموذج الذكاء الاصطناعي صلاحية الوصول المباشر إلى قاعدة البيانات أو الإرسال الخارجي.

## المعمارية

```text
internal/
├── bootstrap/
├── domain/
├── application/
│   ├── ports/
│   ├── commands/
│   ├── queries/
│   └── services/
├── adapters/
│   ├── primary/http/
│   └── secondary/
│       ├── providers/socialapi/
│       ├── persistence/postgres/
│       └── ai/openaicompatible/
└── platform/
```

نستخدم **Modular Monolith**. نقاط الدخول `cmd/api` و`cmd/worker` و`cmd/migrate` رفيعة، وطبقة `bootstrap` فقط هي التي تركب الاعتماديات الخارجية.

## المبادئ غير القابلة للكسر

1. يُتحقق من webhook ثم يُسجل في Event Ledger قبل ACK.
2. المعالجة الخارجية **At-Least-Once + Idempotent Processing**؛ لا يُدَّعى Exactly-Once خارجيًا.
3. يحمي Outbox الانتقال بين database commit والإرسال الخارجي.
4. لا توجد network calls داخل DB transaction.
5. نتيجة الإرسال `unknown` لا تعاد محاولتها عميانيًا؛ تُعزل للمراجعة أو reconciliation.
6. لا يُخلط معرّف Mujeeb أو مزود القناة أو المحادثة الخارجية.
7. لا يختلق AI سعرًا أو مخزونًا أو موعدًا أو سياسة، ولا ينفذ إرسالًا أو SQL.
8. لا تُخزن tokens أو مفاتيح webhook أو أسرار في Domain أو Git.

## المسار التشغيلي

```text
SocialAPI webhook
  → verify / normalize
  → inbound_event_ledger
  → Customer + Conversation + CommunicationMessage
  → optional AutoReply policy
  → AIDecision + OutboundMessage + Outbox
  → worker
  → SocialAPI delivery
```

نجاح الاختبارات المحلية يثبت الوحدات والعقود ومسار PostgreSQL عندما تتوفر قاعدة اختبار؛ ولا يثبت اتصال SocialAPI حيًا أو إرسالًا خارجيًا أو جاهزية إنتاج دون دليل تشغيل مستقل.

## الاختبارات

```bash
GOTOOLCHAIN=local /usr/local/go/bin/go test ./...
GOTOOLCHAIN=local /usr/local/go/bin/go vet ./...
```

توجد أيضًا بوابة المستودع في `scripts/verify-repository.sh`.

## التشغيل المحلي على Windows

راجع [دليل Mujeeb-only](deploy/local/README-ar.md). لا يحتوي Compose المحلي على خدمات خارجية للمحادثات؛ يشغّل Mujeeb وPostgreSQL والـworker والبوابة المحدودة فقط. القيم الحساسة تبقى في `deploy/local/.env` المحلي ولا تُرفع إلى Git.

## المستودع

المسار المرجعي هو الفرع `feat/chatwoot-free` إلى أن تُراجع التغييرات ويُتخذ قرار منفصل برفعه أو دمجه.

<https://github.com/Ammar777782439/mujeeb24-backend-go>
