package handlers

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"io"
	"net/http"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

const maxPrivatePreviewBytes = 10 * 1024 * 1024

// verifyCatalogPreview validates the recorded image without allowing the
// API to become a generic object reader. SHA256/size are verified when present.
func verifyCatalogPreview(data []byte, asset ports.MediaAsset) ([]byte, error) {
	imageBytes, err := verifiedPreviewBytes(bytes.NewReader(data), asset.MimeType)
	if err != nil {
		return nil, err
	}
	if asset.SizeBytes > 0 && int64(len(imageBytes)) != asset.SizeBytes {
		return nil, errors.New("image byte count differs from stored metadata")
	}
	if len(asset.SHA256) > 0 {
		if len(asset.SHA256) != sha256.Size {
			return nil, errors.New("invalid stored image checksum")
		}
		digest := sha256.Sum256(imageBytes)
		if !bytes.Equal(digest[:], asset.SHA256) {
			return nil, errors.New("image checksum mismatch")
		}
	}
	return imageBytes, nil
}

func verifiedPreviewBytes(r io.Reader, mime string) ([]byte, error) {
	if mime != "image/jpeg" && mime != "image/png" {
		return nil, errors.New("unsupported media preview type")
	}
	data, err := io.ReadAll(io.LimitReader(r, maxPrivatePreviewBytes+1))
	if err != nil || len(data) == 0 || len(data) > maxPrivatePreviewBytes {
		return nil, errors.New("media preview is oversized or incomplete")
	}
	if http.DetectContentType(data) != mime {
		return nil, errors.New("media preview bytes do not match recorded type")
	}
	return data, nil
}
