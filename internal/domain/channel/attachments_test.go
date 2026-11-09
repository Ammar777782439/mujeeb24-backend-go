package channel

import (
	"errors"
	"testing"
)

func TestAttachmentKindValid(t *testing.T) {
	kinds := []AttachmentKind{
		AttachmentImage,
		AttachmentVideo,
		AttachmentAudio,
		AttachmentDocument,
		AttachmentFile,
	}
	for _, kind := range kinds {
		if !kind.Valid() {
			t.Errorf("expected %q to be a valid attachment kind", kind)
		}
	}
	if (AttachmentKind("sticker")).Valid() {
		t.Fatal("unexpected attachment kind accepted")
	}
}

func TestInboundAttachmentReferenceValidate(t *testing.T) {
	valid := InboundAttachmentReference{
		ProviderMediaReference: "opaque-provider-media-id",
		Kind:                   AttachmentImage,
		MIMEType:               "image/jpeg",
		FileName:               "photo.jpg",
		SizeBytes:              1024,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid inbound attachment rejected: %v", err)
	}

	tests := []struct {
		name      string
		reference InboundAttachmentReference
		want      error
	}{
		{
			name: "missing provider reference",
			reference: InboundAttachmentReference{
				Kind: AttachmentImage,
			},
			want: ErrMissingProviderMediaReference,
		},
		{
			name: "invalid kind",
			reference: InboundAttachmentReference{
				ProviderMediaReference: "provider-media-id",
				Kind:                   AttachmentKind("archive"),
			},
			want: ErrInvalidAttachmentKind,
		},
		{
			name: "negative size",
			reference: InboundAttachmentReference{
				ProviderMediaReference: "provider-media-id",
				Kind:                   AttachmentDocument,
				SizeBytes:              -1,
			},
			want: ErrInvalidAttachmentSize,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.reference.Validate(); !errors.Is(err, test.want) {
				t.Fatalf("expected error %v, got %v", test.want, err)
			}
		})
	}
}

func TestValidateInboundAttachmentReferencesIncludesIndex(t *testing.T) {
	err := ValidateInboundAttachmentReferences([]InboundAttachmentReference{
		{ProviderMediaReference: "provider-media-1", Kind: AttachmentImage},
		{Kind: AttachmentDocument},
	})
	if !errors.Is(err, ErrMissingProviderMediaReference) {
		t.Fatalf("expected missing provider reference, got %v", err)
	}
	if err == nil || err.Error() != "inbound attachment 1: provider media reference is required" {
		t.Fatalf("expected indexed validation error, got %v", err)
	}
}

func TestOutboundAttachmentReferenceValidate(t *testing.T) {
	valid := OutboundAttachmentReference{
		MediaAssetID: "mujeeb-media-asset-id",
		Kind:         AttachmentDocument,
		Caption:      "فاتورة",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid outbound attachment rejected: %v", err)
	}

	if err := (OutboundAttachmentReference{Kind: AttachmentImage}).Validate(); !errors.Is(err, ErrMissingMediaAssetID) {
		t.Fatalf("expected missing MediaAsset id, got %v", err)
	}
	if err := (OutboundAttachmentReference{MediaAssetID: "asset-1", Kind: AttachmentKind("unknown")}).Validate(); !errors.Is(err, ErrInvalidAttachmentKind) {
		t.Fatalf("expected invalid kind, got %v", err)
	}
}

func TestValidateOutboundAttachmentReferencesIncludesIndex(t *testing.T) {
	err := ValidateOutboundAttachmentReferences([]OutboundAttachmentReference{
		{MediaAssetID: "asset-1", Kind: AttachmentImage},
		{Kind: AttachmentFile},
	})
	if !errors.Is(err, ErrMissingMediaAssetID) {
		t.Fatalf("expected missing MediaAsset id, got %v", err)
	}
	if err == nil || err.Error() != "outbound attachment 1: media asset id is required" {
		t.Fatalf("expected indexed validation error, got %v", err)
	}
}
