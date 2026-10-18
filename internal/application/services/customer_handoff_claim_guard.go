package services

import (
	"regexp"
	"strings"
	"unicode"
)

// This is an outbound-claim safety check, NOT an intent engine. The model
// remains solely responsible for selecting the offered capability. The check
// cannot select, upgrade, or execute handoff_to_human.
//
// Explicit promises of an employee handoff in an informational reply have no
// matching authorized operation, so they must not be sent to customers. Only
// high-confidence positive commitments are checked: questions and statements
// explaining the handoff process are not themselves commitments.
var unsupportedHandoffCommitment = []*regexp.Regexp{
	regexp.MustCompile(`(?:^|[\s،؛:!؟.])(?:ساحول|سنحول|بحول|بنحول|سارفع|سنرفع|ساصعد|سنصعد|سارسل|سنرسل|ساقوم\s+ب(?:تحويل|احاله|تصعيد|رفع)|سنقوم\s+ب(?:تحويل|احاله|تصعيد|رفع)|(?:سيتم|سوف\s+يتم|تم|جرى)\s+(?:تحويل|احاله|تصعيد|رفع)|سوف\s+(?:احول|نحول|ارفع|نرفع|اصعد|نصعد)|(?:سيتواصل|سيتصل|سنتواصل|سنتصل)\s+(?:معك|معكم)|(?:فريقنا|موظفونا|احد\s+(?:موظفينا|الفريق))\s+(?:سيتواصل|سيتصل)|سيقوم\s+(?:الفريق|الموظف)\s+ب(?:التواصل|الاتصال)|ساتواصل\s+مع\s+(?:الفريق|الموظف|الاداره))`),
	regexp.MustCompile(`(?:^|[\s,;:.!?])(?:i(?:'ll|\s+will)\s+(?:transfer|escalate|forward)|we(?:'ll|\s+will)\s+(?:transfer|escalate|forward)|(?:has\s+been|have\s+been|will\s+be)\s+(?:transferred|escalated|forwarded)|(?:someone|our\s+team)\s+will\s+(?:contact|call|reach))\b`),
}

func containsUnexecutedHandoffCommitment(text string) bool {
	normalized := strings.ToLower(strings.Map(func(r rune) rune {
		switch r {
		case 'أ', 'إ', 'آ', 'ٱ':
			return 'ا'
		case 'ى':
			return 'ي'
		case 'ة':
			return 'ه'
		case 'ـ':
			return -1
		}
		if unicode.Is(unicode.Mn, r) {
			return -1
		}
		return r
	}, text))
	for _, pattern := range unsupportedHandoffCommitment {
		if pattern.MatchString(normalized) {
			return true
		}
	}
	return false
}
