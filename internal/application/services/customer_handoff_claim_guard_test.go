package services

import "testing"

func TestUnexecutedHandoffCommitmentSafetyCheck(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		claim bool
	}{
		{"promised direct transfer", "سأحوّلك لأحد الموظفين الآن.", true},
		{"future passive transfer", "سيتم تحويلك إلى فريق خدمة العملاء.", true},
		{"team follow up", "فريقنا سيتواصل معك قريباً.", true},
		{"raising a request", "سأقوم برفع طلبك للفريق المختص.", true},
		{"past unverified escalation", "تم تصعيد طلبك لقسم الدعم.", true},
		{"customer messaging", "بنحولك للموظف المسؤول.", true},
		{"english future escalation", "We'll escalate this to our support team.", true},
		{"explanation of process", "التحويل للموظف متاح من لوحة التحكم.", false},
		{"question about transfer", "كيف يتم تحويل المحادثة؟", false},
		{"no ability to transfer", "لا أستطيع تحويل المحادثة دون موظف.", false},
		{"plain information", "باقة الأعمال تتيح ربط القنوات حسب اشتراكك.", false},
		{"plain greeting", "وعليكم السلام ورحمة الله وبركاته", false},
		{"request for clarification", "ما المشكلة التي ترغب في حلها؟", false},
	} {
		t.Run(tc.name,func(t *testing.T) {
			if got := containsUnexecutedHandoffCommitment(tc.text); got != tc.claim {
				t.Fatalf("claim=%t want=%t for case=%s",got,tc.claim,tc.name)
			}
		})
	}
}
