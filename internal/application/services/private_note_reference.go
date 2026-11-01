package services

import (
	"encoding/json"
	"strings"

	"github.com/google/uuid"
)

// ContentReference already stores versioned internal message metadata. Legacy
// private-note:<id> records remain readable without a schema/data migration.
const privateNoteReferencePrefix = "private-note:v1:"

type privateNoteReference struct {
	NoteID           string `json:"note_id"`
	RelatedMessageID string `json:"related_message_id"`
	Quote            string `json:"quote"`
}

func encodePrivateNoteReference(noteID, messageID, text string) string {
	quote := []rune(text)
	if len(quote) > 280 {
		quote = append(quote[:280], '…')
	}
	data, _ := json.Marshal(privateNoteReference{NoteID: noteID, RelatedMessageID: messageID, Quote: string(quote)})
	return privateNoteReferencePrefix + string(data)
}

func decodePrivateNoteReference(reference string) (privateNoteReference, bool) {
	var result privateNoteReference
	if !strings.HasPrefix(reference, privateNoteReferencePrefix) {
		return result, false
	}
	err := json.Unmarshal([]byte(strings.TrimPrefix(reference, privateNoteReferencePrefix)), &result)
	return result, err == nil && uuid.Validate(result.RelatedMessageID) == nil
}
