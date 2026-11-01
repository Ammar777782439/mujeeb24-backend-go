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

// Scope the retry identity to the authenticated author as well as the tenant
// and conversation; sharing an Idempotency-Key cannot expose another author's note.
func privateNoteID(businessID, conversationID, principalID, key string) string {
	if key == "" {
		return uuid.NewString()
	}
	scope, _ := json.Marshal([]string{"private-note-v1", businessID, conversationID, principalID, key})
	return uuid.NewSHA1(uuid.NameSpaceURL, scope).String()
}
