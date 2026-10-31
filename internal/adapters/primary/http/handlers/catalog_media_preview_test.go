package handlers

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
)

func TestCatalogPreviewAllowsOnlyTrustedSignedPrivateHosts(t *testing.T) {
	for _, u := range []string{
		"https://s3.us-east-005.backblazeb2.com/mujeeb24-images/catalog/a.jpg?X-Amz-Signature=abc",
		"https://0123456789abcdef.r2.cloudflarestorage.com/catalog/a.jpg?X-Amz-Signature=abc",
	} {
		if !isTrustedCatalogObjectURL(u) { t.Errorf("valid presigned host denied: %s", u) }
	}
	for _, u := range []string{
		"http://s3.us-east-005.backblazeb2.com/catalog/a.jpg?X-Amz-Signature=abc",
		"https://s3.us-east-005.backblazeb2.com.attacker.test/a?X-Amz-Signature=abc",
		"https://user:password@s3.us-east-005.backblazeb2.com/a?X-Amz-Signature=abc",
		"https://127.0.0.1/a?X-Amz-Signature=abc",
		"https://s3.us-east-005.backblazeb2.com/a",
		"https://s3.us-east-005.backblazeb2.com:8443/a?X-Amz-Signature=abc",
		"https://0123456789abcdef.r2.cloudflarestorage.com.attacker.test/a?X-Amz-Signature=abc",
	} {
		if isTrustedCatalogObjectURL(u) { t.Errorf("untrusted signed host accepted: %s", u) }
	}
}

func TestCatalogPreviewValidatesBytesAndBounds(t *testing.T) {
	jpeg := append([]byte{0xff,0xd8,0xff,0xe0,0,0x10,'J','F','I','F',0}, bytes.Repeat([]byte{1}, 80)...)
	data, err := verifiedPreviewBytes(bytes.NewReader(jpeg), "image/jpeg")
	if err != nil || !bytes.Equal(data,jpeg) { t.Fatalf("valid JPEG was rejected: %v",err) }
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
