# ⑥ التحقق والتفويض للقدرات

الحالة: CURRENT — يحل محل وصف status/action السابق.

المرجع التفصيلي: [عقد القدرات v11](../../docs/architecture/customer-sales-capabilities-v11-ar.md).

يمر `CapabilityDecision` دون تغيير هويته عبر: Structural → Reference → Tenant/Ownership → Policy → Authorization → Execution → Audit.

- Structural يقبل فقط قدرة نهائية مسجلة ومنفذة، ويشترط نص العميل والمراجع الصحيحة. القدرة غير المعروفة أو retrieval في مرحلة نهائية تُرفض.
- Reference يتحقق من الأدلة الفعلية وعلاقات item/variant/offer التي عُرضت للنموذج، دون بحث أو إعادة مطابقة دلالية.
- Tenant/Ownership يتحقق من نطاق التاجر الموثوق؛ النموذج لا يزود tenant.
- Policy يتلقى CapabilityDecision نفسه ويطبق خصائص القدرة المسجلة وسياسة التاجر. جميع القدرات النهائية الحالية تستخدم منفذ رسالة العميل، لذا تخضع allow_auto_reply وdefault_human_review وai_mode.
- Authorization يسمح أو يمنع أو يطلب مراجعة، ولا يستبدل القدرة المختارة ولا يتجاوز policy.
- Execution يستخدم تعريف القدرة في Registry لتحديد المنفذ وحالة المحادثة. reply_customer/ask_customer يؤديان waiting_customer وownership=ai. handoff_to_human يرسل نص Gemini ويضع waiting_human دون ادعاء إسناد ownership لموظف.
- VALIDATING → AUTHORIZED → EXECUTING إلزامية قبل Outbox؛ الانتقالان الأخيران مع القرار وOutbox داخل معاملة محلية دون شبكة.
- denied/requires_approval لا ينفذان الرسالة. انتظار الموظف يوقف الدور التالي قبل النموذج.

Audit يحفظ requested_capability وeffective_capability مع policy_decision والتوقيت، ويسجل اللوج capability. IntentBase فارغ لقرارات القدرات؛ لا Intent taxonomy. effective_capability هي القدرة التي اجتازت التحقق، وليست دليل تنفيذ؛ policy_decision وauthorized_at وai_run status يثبتان الإذن والنتيجة. RequestedAction/EffectiveAction للتوافق فقط عند الحفظ وAPI.

[النص التاريخي المستبدل](archive/06-ai-validation-authorization-boundary-closed.md) محفوظ للمراجعة فقط ولا يوجه الكود.
