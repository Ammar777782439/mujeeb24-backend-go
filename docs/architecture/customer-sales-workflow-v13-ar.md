# مسار المبيعات التفاعلي — v13

الفرع `feat/customer-sales-workflow-interactive` متفرع من `feat/universal-catalog-ai-v3` عند `32de9744da41747db7f752187db62622f8c35bfd` بناءً على تصريح المستخدم. لا دمج إلى main.

## النطاق المعتمد

Gemini يفهم الرسالة ويختار قدرة واحدة. الردود والاستفسارات تستخدم منفذ الرسائل الموجود. `record_lead` و`create_transaction_draft` و`update_transaction_draft` تستخدم منفذ مبيعات مستقلًا، مع خدمات المبيعات المشتركة. لا Intent Engine أو keywords أو بحث في الكتالوج. مسار `evaluate_full_catalog` يبقى كامل التغطية.

التأكيد المالي ينتج عن أزرار مولدة من طلب محفوظ، لا عن تفسير نص نعم/لا. الضغط يحمل مرجعًا غير سلطوي؛ توقيع المزود وهوية العميل والنطاق والمعاملة ونسخة الاتفاق هي حواجز التنفيذ. لا يستطيع Gemini إنشاء هوية أو سعر أو صلاحية أو إيصال نجاح.

## التشغيل

- تطبيق migration الأمامية `000074` قبل تشغيل النسخة.
- `CUSTOMER_SALES_ENABLED=true` يركب منفذ المبيعات في API ومعالج الأزرار في Worker؛ يجب ضبطه في العمليتين، وهو مستقل عن `AUTOREPLY_ENABLED`.
- `sales_workflow_enabled` و`allow_sales_interactions` في سياسة التاجر افتراضيًا false.
- أعلام `allow_auto_lead_creation` و`allow_auto_transaction_draft` و`allow_auto_confirmation` تحكم آثارها منفصلة. `allow_auto_reply` ليس تفويضًا تجاريًا.
- assist/approval يحفظان اقتراحًا بانتظار اعتماد الموظف؛ الاعتماد يعيد فحص البيانات الحالية.
- الأزرار تمر من ContentReference إلى Outbox/Worker ثم SocialAPI. أحداثها تُحفظ كمهام دائمة داخل معاملة materialization؛ لا تتجه إلى Gemini.

## API

`GET /businesses/{business_id}/sales/workflows` مع cursor/limit، وGET للمورد. `POST .../sales/workflows/{workflow_id}/review` يحتاج If-Match وصلاحية sales:approve. `GET/POST .../sales/requirements` يقرأ متطلبات الأنواع ويحفظ إصدارات غير قابلة للاستبدال؛ الإنشاء يحتاج business:settings.

الأسماء والمواصفات التجارية تأتي من الكتالوج. متطلبات المعاملة مستقلة ومؤرخة؛ لا نفترض أن schema عنصر الكتالوج يصف بيانات حجز العميل.

## حواجز التأكيد

اتفاق العميل له agreement_version مستقل عن resource_version. اعتماد المراجعة لا يغير تفاصيل الاتفاق. تعديل المسودة يغير agreement_version ويلغي الأزرار والمراجعة القديمة. تعديل الموظف يحدّث أيضًا نسخة وحالة وبيانات المسار؛ إلغاء المعاملة يغلق المسار. أوامر الموظف وGemini والأزرار تتبع ترتيب الأقفال نفسه: المحادثة ثم المسار ثم المعاملة. تأكيد الصفقة يحتاج عرضًا وسعرًا وتوفرًا صالحين ومراجعة معتمدة عند طلبها.

السعر starting_from/quote_required/dynamic لا يصبح مبلغًا نهائيًا بافتراض. مسودات الأنواع السبعة محفوظة دون ادعاء تشغيل تكامل خارجي. التأكيد الآلي الحالي محصور بعمليات order ذات always_available وfulfillment_mode من pickup/digital، مع سماح سياسة المحادثة؛ stock/schedule/supplier_check تحتاج مراجعة ولا ندّعي وجود منفذ حجز مورد. تفعيل اشتراك منصة مجيب مسار مستقل عن صفقة subscription في الكتالوج.

لا يظهر زر التأكيد عند غياب سعر نهائي موثق أو عدم اكتمال البيانات الإلزامية. الأنواع غير order تحتاج إصدار متطلبات صريحًا قبل عرض التأكيد. التأكيد يعيد فحص إصدار المتطلبات المثبت في المسودة، وهوية صاحب الفرصة وحالتها، ويحجز صفوف الكتالوج والنسخة والعرض خلال المعاملة. عرض quote مؤكد لا يحوّل الفرصة إلى won.

## التحقق والحالة

تم اجتياز Quality gate مع PostgreSQL 16 عند `127b83436c664ecb2408881b1c4c4fbd5429f622`؛ تُعاد البوابة عند كل تحديث. هذا ليس تصريحًا بجاهزية live. الاختبارات المحلية لا تثبت PostgreSQL أو القنوات. Quality gate على الفرع يشغل PostgreSQL 16. فحوصات Gemini وSocialAPI الحية لم تُشغّل؛ لا استدعاءات نموذج أو رسائل عملاء أثناء التطوير.

مصادر المزود: https://docs.social-api.ai/guides/inbox وhttps://docs.social-api.ai/guides/webhooks وhttps://docs.social-api.ai/api-reference/inbox/send-a-message-in-a-conversation. الأزرار/quick replies لإنستجرام وفيسبوك، وmetadata.payload للأحداث. إرسال WhatsApp بالأزرار مدعوم حسب عقد endpoint؛ يلزم اختبار القناة الفعلي قبل إعلان التشغيل.
