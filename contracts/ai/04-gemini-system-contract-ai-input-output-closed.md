# ④ عقد Gemini — اختيار القدرات

الحالة: CURRENT — يحل محل وصف status/action السابق.

المرجع التفصيلي: [عقد القدرات v11](../../docs/architecture/customer-sales-capabilities-v11-ar.md).

Gemini يفهم الرسالة وسياق المحادثة ويختار استدعاء دالة واحدة من القدرات المعروضة. مجيب لا يصنف النية ولا يستخدم كلمات أو قطاعات لاختيار إجراء.

المخرج التشغيلي هو `CapabilityDecision { capability, response_text, selected[] }`. اسم القدرة يأتي من اسم Function Call؛ arguments لا تقبل capability بديلة أو tenant أو policy أو authorization أو status/action/routing_reason. القدرات النهائية تحمل نص Gemini ومراجع موثوقة. `evaluate_full_catalog` بلا arguments وبلا response_text.

إذا طلب Gemini الكتالوج، يقرأ مجيب كامل الكتالوج النشط للتاجر وعلاقاته، ويقيّم Gemini كل دفعات التوكن. فشل دفعة أو تغير revision يمنع القرار النهائي. بعد اكتمال التغطية يختار Gemini قدرة نهائية فقط، ويحافظ `BatchClient → CatalogEvaluationResult → AutoReply` على اسم القدرة ونصها ومراجعها. لا retrieval loop ولا تحويل إلى Action داخل المهايئ.

الـManifest يشرح البنية ولا يثبت الوجود أو السعر أو التوفر. الفهم قد ينتهي مباشرة إذا كانت الأدلة الحالية الموثوقة كافية. تاريخ مجيب هو المرجع، واختيار القدرات stateless.

الحقول القديمة `answer/clarification/human_request` تُشتق في حد توافق الحفظ وAPI فقط. لا تقود السياسات أو التنفيذ. لا تُعرض create_lead_draft/create_order_draft حتى يوجد executor فعلي.

[النص التاريخي المستبدل](archive/04-gemini-system-contract-ai-input-output-closed.md) محفوظ للمراجعة فقط ولا يوجه الكود.
