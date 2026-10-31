package handlers

import (
	"bytes"
	"crypto/sha256"
	"strings"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

func TestCatalogPreviewValidatesBytesAndBounds(t *testing.T) {
	jpeg := append([]byte{0xff, 0xd8, 0xff, 0xe0, 0, 0x10, 'J', 'F', 'I', 'F', 0}, bytes.Repeat([]byte{1}, 80)...)
	data, err := verifiedPreviewBytes(bytes.NewReader(jpeg), "image/jpeg")
	if err != nil || !bytes.Equal(data, jpeg) {
		t.Fatalf("valid JPEG was rejected: %v", err)
	}
	if _, err := verifiedPreviewBytes(bytes.NewReader(jpeg), "image/png"); err == nil {
		t.Fatal("image content mismatch accepted")
	}
	if _, err := verifiedPreviewBytes(bytes.NewReader(jpeg), "text/html"); err == nil {
		t.Fatal("unsupported MIME accepted")
	}
	if _, err := verifiedPreviewBytes(strings.NewReader(""), "image/jpeg"); err == nil {
		t.Fatal("empty image accepted")
	}
	if _, err := verifiedPreviewBytes(bytes.NewReader(make([]byte,maxPrivatePreviewBytes+1)), "image/jpeg"); err == nil {
		t.Fatal("oversized image accepted")
	}
}

func TestCatalogPreviewVerifiesStoredImageIntegrity(t *testing.T) {
	jpeg := append([]byte{0xff, 0xd8, 0xff, 0xe0, 0, 0x10, 'J', 'F', 'I', 'F', 0}, bytes.Repeat([]byte{0x13}, 80)...)
	digest := sha256.Sum256(jpeg)
	asset := ports.MediaAsset{MimeType: "image/jpeg", SizeBytes:int64(len(jpeg)), SHA256:digest[:]}
	result, err := verifyCatalogPreview(jpeg, asset)
	if err != nil || !bytes.Equal(result,jpeg) {
		t.Fatalf("verified private image rejected: %v",err)
	}
	asset.SizeBytes++
	if _, err := verifyCatalogPreview(jpeg, asset); err == nil {
		t.Fatal("mismatched persisted image size accepted")
	}
	asset.SizeBytes--
	asset.SHA256=bytes.Repeat([]byte{1},sha256.Size)
	if _, err := verifyCatalogPreview(jpeg,asset); err == nil {
		t.Fatal("corrupted private object accepted")
	}
}
