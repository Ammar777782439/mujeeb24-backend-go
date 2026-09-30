# مصفوفة إزالة 501 من Runtime API

## قاعدة التنفيذ

الهدف ليس تحويل `501` إلى `200` شكليًا. كل operation يجب أن يصل إلى **Application Handler حقيقي** ثم PostgreSQL أو adapter مهيأ، ويحافظ على `JWT → PostgreSQL membership → business scope`. إذا كان endpoint يعتمد على provider غير مهيأ محليًا، يعيد خطأ تشغيليًا typed مناسبًا، ولا ينفذ اتصالًا حيًا أو نجاحًا مزيفًا.

| المجموعة | العمليات الحالية | نوع العمل | شرط الإغلاق |
|---|---|---|---|
| Business | `getBusiness`، `updateBusinessProfile`، `getBusinessPolicy`، `updateBusinessPolicy`، `getDashboardOverview` | services وports للقراءة/الكتابة/overview ثم wiring | PostgreSQL + 401/403/404/If-Match HTTP tests. |
| Conversations | list/get/update/assign/labels/private-note | توسعة Conversation repository وcommand/query services | scope، ordering/keyset، mutations transaction-aware، وعدم خلط communication مع outbound. |
| Customers | list/get/create/update/merge وcustomer conversations | توسعة Customer repository والخدمات | tenant isolation وmerge semantics وtransaction tests. |
| Outbound message | `createOutboundMessage` | command service يكتب Mujeeb-owned outbound + outbox في transaction | لا provider call داخل transaction؛ test outbox atomicity. |
| Channels | list/get/reconnect/disconnect | read services الآن؛ reconnect/disconnect workflow مهيأ بالـprovider/feature gate لاحقًا | لا 501؛ local path يثبت status/error typed من دون SocialAPI live. |
| Metrics | `getMetrics` | metrics query من runtime/database counters | response contract واختبار unauthenticated/public policy صريح. |

## ترتيب التنفيذ

1. **Business** أولًا، لأنه يثبت أن principal scope يستطيع قراءة وتحديث root aggregate، ويعطي fixture أساسًا لبقية الاختبارات.
2. **Conversation + Customer** معًا، لأن conversation تملك customer scope وmessage timeline، ولا يصح بناء أحدهما بمعزل عن الآخر.
3. **Outbound + Channels + Metrics** بعد ذلك، مع بقاء network adapter خارج transaction وfeature flags مغلقة افتراضيًا.
4. بعد كل مجموعة: PostgreSQL 16 integration، HTTP runtime tests، OpenAPI drift، وتحديث Postman لإزالة 501 من folder المعروف فقط عندما يثبت success path.

## حدود لا تتغير

لا SocialAPI أو Chatwoot أو Meta/WhatsApp/Instagram أو LLM live في هذه الإزالة للـ501. لا principal ثابت ولا scope من request header ولا success mock. لا تعديلات على migrations `000001` إلى `000041`؛ أي حقل جديد يحتاج forward migration مثبتة.
