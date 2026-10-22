# ADR — حدود الأدلة التجارية في برمبت Customer Sales

التاريخ: 2026-10-22  
النطاق: `feat/gemini-deepseek`، مسار Customer Sales فقط.  
القرار: تحديث تعليمات النموذج الأولية والنهائية دون تغيير أدوات التنفيذ أو واجهات API.

## السبب
كشفت تجربة محادثة عميل عن إجابة تنفي وجود iPhone رغم وجود عناصر iPhone نشطة في الكتالوج. استُخدم `respond_without_action` دون طلب `evaluate_full_catalog`، مع أن أدلة السياق الجزئية لا تكفي لإثبات الغياب. الملخص والرسائل السابقة لا يصلحان مصدرًا لحقيقة تجارية.

## التعديل
- `CustomerSalesSystemPromptVersion = customer-sales-v15-evidence-boundaries`: توضيح أن ملخص المحادثة للاسترشاد بنيّة العميل، وأن حقائق المنتجات والأسعار والتوفر تستند إلى أدلة موثوقة؛ وعند نقص الأدلة تطلب قراءة الكتالوج الكاملة إن كانت القدرة معروضة.
- `FinalEvaluationSystemPromptVersion = final-evaluation-v5-evidence-boundaries`: اختيار قدرة نهائية واحدة بعد اكتمال التقييم، وعدم تعميم غياب عنصر من المرشحين خارج نطاق التغطية.
- `BatchEvaluationSystemPrompt` و`CandidateReductionSystemPromptSuffix` وعقود `JSON Schema` والأدوات والـauthorization والـoutbox لا تتغير.

## حدود الضمان والتحقق
هذه تعليمات للنموذج وليست ضمانًا سلوكيًا؛ يجب لاحقًا اختبار سيناريو الانتقال بين Samsung وiPhone، وادعاءات عدم التوفر والأسعار وغياب المخزون، وتغيير هدف العميل، والمبيعات والتحويل البشري، وGemini/DeepSeek والفشل الاحتياطي. سلامة صياغة Go ونجاح CI لا يثبتان خلو نص `response_text` من ادعاءات تجارية غير مدعومة.

لا يتغير مصدر الحقيقة ولا نموذج الكتالوج ولا بنية Customer Sales أو Merchant Catalog AI.
