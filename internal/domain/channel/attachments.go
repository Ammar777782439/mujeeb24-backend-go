package channel

import (
	"errors"
	"fmt"
	"strings"
)

// AttachmentKind describes the logical media type independently of a
// provider-specific transport representation or the stored MIME type.
type AttachmentKind string

const (
	AttachmentImage    AttachmentKind = "image"
	AttachmentVideo    AttachmentKind = "video"
	AttachmentAudio    AttachmentKind = "audio"
	AttachmentDocument AttachmentKind = "document"
	AttachmentFile     AttachmentKind = "file"
)

var (
	ErrInvalidAttachmentKind         = errors.New("invalid attachment kind")
	ErrMissingProviderMediaReference = errors.New("provider media reference is required")
	ErrMissingMediaAssetID           = errors.New("media asset id is required")
	ErrInvalidAttachmentSize         = errors.New("attachment size cannot be negative")
)

func (kind AttachmentKind) Valid() bool {
	switch kind {
	case AttachmentImage, AttachmentVideo, AttachmentAudio, AttachmentDocument, AttachmentFile:
		return true
	default:
		return false
	}
}

// InboundAttachmentReference is an untrusted, provider-neutral reference
// extracted from an inbound webhook. ProviderMediaReference must be an
// opaque identifier/token that the provider adapter can resolve; it must not
// be a public URL or a Mujeeb storage key.
//
// MIMEType, FileName and SizeBytes are hints only. The media ingestion
// pipeline must verify the actual bytes before making the asset available.
type InboundAttachmentReference struct {
	ProviderMediaReference string
	Kind                   AttachmentKind
	MIMEType               string
	FileName               string
	Caption                string
	SizeBytes              int64
}

func (reference InboundAttachmentReference) Validate() error {
	if !reference.Kind.Valid() {
		return ErrInvalidAttachmentKind
	}
	if strings.TrimSpace(reference.ProviderMediaReference) == "" {
		return ErrMissingProviderMediaReference
	}
	if reference.SizeBytes < 0 {
		return ErrInvalidAttachmentSize
	}
	return nil
}

// OutboundAttachmentReference identifies a Mujeeb-owned MediaAsset selected
// for an outbound message. It deliberately carries no storage URL, object key,
// or provider media ID; the application and provider adapter resolve those
// details after authorization and channel-capability checks.
type OutboundAttachmentReference struct {
	MediaAssetID string
	Kind         AttachmentKind
	Caption      string
}

func (reference OutboundAttachmentReference) Validate() error {
	if !reference.Kind.Valid() {
		return ErrInvalidAttachmentKind
	}
	if strings.TrimSpace(reference.MediaAssetID) == "" {
		return ErrMissingMediaAssetID
	}
	return nil
}

func ValidateInboundAttachmentReferences(references []InboundAttachmentReference) error {
	for index, reference := range references {
		if err := reference.Validate(); err != nil {
			return fmt.Errorf("inbound attachment %d: %w", index, err)
		}
	}
	return nil
}

func ValidateOutboundAttachmentReferences(references []OutboundAttachmentReference) error {
	for index, reference := range references {
		if err := reference.Validate(); err != nil {
			return fmt.Errorf("outbound attachment %d: %w", index, err)
		}
	}
	return nil
}
