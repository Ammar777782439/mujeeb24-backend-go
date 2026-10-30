# Merchant Catalog AI — تتبع طلبات النموذج الكاملة في Render Logs

هذه الخاصية للاختبار وتشخيص الأخطاء، ولا تغير العقود التجارية أو الصلاحيات أو الحفظ.

في إعدادات Render Environment أضف:

```env
MUJEEB_MERCHANT_CATALOG_DEBUG=true
```

بعد إعادة نشر Backend، أرسل الرسالة الفاشلة إلى مساعد الكتالوج، ثم ابحث في Render Logs عن `[MerchantCatalogTrace]`. جميع المراحل تحمل `session` لتسهيل ربط جولات الطلب.

| stage | ما الذي يظهر؟ |
|---|---|
| `canonical_request_round_N` | تعليمات النموذج، المحادثة، أدوات القراءة والطلب قبل المزوّد |
| `deepseek_http_request` | جسم JSON الحقيقي المرسل إلى DeepSeek بعد تحويل العقد |
| `deepseek_http_raw_response` | الاستجابة الأصلية قبل أي تطبيع أو تعديل في DeepSeek Adapter |
| `canonical_response_round_N` | الاستجابة الموحدة بعد ترجمة المزوّد |
| `canonical_wire_round_N` | JSON الذي فحصه `catalogWireParse` |
| `wire_rejected`, `references_rejected`, `proposal_rejected` | السبب النصي الذي رفض Go من أجله الاستجابة |
| `provider_error`, `provider_success`, `wire_parsed`, `proposal_accepted` | حالة الجولة، المزوّد، العملية والنتيجة |
| `deepseek_http_error_response` | نص خطأ DeepSeek عند فشل HTTP |

النص الطويل ينقسم إلى `part=1/N` إلخ. اجمع قيم `payload=` بالترتيب للحصول على النص الكامل؛ لا يتم حذف النص العربي أو محتوى JSON.

المحتوى غير منقح عمدًا، ويتضمن طلبات التاجر وبيانات الكتالوج ومخرجات DeepSeek. لا يتضمن ترويسة HTTP Authorization أو مفتاح API لأنهما ليسا ضمن جسم الطلب. بعد انتهاء التشخيص غيّر المتغير إلى `false` أو احذفه وأعد النشر.
