package services

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

const (
	maxConversationImageBytes        = 10 * 1024 * 1024
	maxConversationImagePixels       = 16_000_000
	maxConversationImageEdge         = 12_000
	maxConversationIngestionAttempts = 8
)

// ConversationMediaIngestionProcessor downloads and verifies inbound WhatsApp
// images, stores them privately, then commits metadata in short local SQL
// operations. It deliberately does not process other channels or media kinds:
// those remain queued until their source and verifier contracts are implemented.
type ConversationMediaIngestionProcessor struct {
	Attachments ports.MessageAttachmentRepository
	Assets      ports.ConversationMediaIngestionRepository
	Resolver    ports.InboundMediaSourceResolver
	Downloader  ports.InboundMediaDownloader
	Objects     ports.MediaObjectStorage
	Now         func() time.Time
}

func (p *ConversationMediaIngestionProcessor) now() time.Time {
	if p != nil && p.Now != nil {
		return p.Now().UTC()
	}
	return time.Now().UTC()
}

func (p *ConversationMediaIngestionProcessor) configured() error {
	if p == nil || p.Attachments == nil || p.Assets == nil ||
		p.Resolver == nil || p.Downloader == nil || p.Objects == nil {
		return errors.New("conversation media ingestion dependencies are incomplete")
	}
	if _, ok := p.Objects.(ports.MediaObjectProviderWriter); !ok {
		return errors.New("conversation media storage must support provider-pinned writes")
	}
	return nil
}

// Process claims only work supported by the current first ingestion slice:
// WhatsApp image attachments. Other channels and kinds remain pending rather
// than being incorrectly downloaded, mislabelled, or marked failed.
func (p *ConversationMediaIngestionProcessor) Process(ctx context.Context, limit int) (int, error) {
	if err := p.configured(); err != nil {
		return 0, err
	}
	if limit < 1 || limit > 100 {
		return 0, errors.New("conversation media batch limit must be between 1 and 100")
	}
	claimed, err := p.Attachments.ClaimPendingIngestions(ctx, limit, 2*time.Minute)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, item := range claimed {
		if err := ctx.Err(); err != nil {
			return processed, err
		}
		if err := p.processOne(ctx, item); err != nil {
			return processed, err
		}
		processed++
	}
	return processed, nil
}

func (p *ConversationMediaIngestionProcessor) processOne(ctx context.Context, item ports.ClaimedMessageAttachmentIngestion) error {
	if item.Channel != "whatsapp" || item.Kind != "image" {
		return fmt.Errorf("claimed conversation media is outside the supported ingestion slice")
	}
	asset, err := p.Assets.GetAsset(ctx, item.BusinessID, item.MediaAssetID)
	if err != nil {
		return err
	}
	// A crash after marking the asset ready but before marking the relation ready
	// is recovered without downloading or uploading the same bytes again.
	if asset.Status == "ready" {
		_, err := p.Attachments.MarkReady(ctx, item.BusinessID, item.AttachmentID)
		return err
	}
	if asset.Status != "pending_ingestion" {
		_, err := p.Attachments.MarkFailed(ctx, item.BusinessID, item.AttachmentID)
		return err
	}

	source, err := p.Resolver.ResolveInboundMediaSource(ctx, item.ProviderConversationID, item.ProviderMediaReference, item.Kind)
	if err != nil {
		return p.handleProviderFailure(ctx, item, asset.ID, err)
	}
	// The current worker slice supports SocialAPI's authenticated media proxy.
	// Never fetch an arbitrary webhook/history URL: that would introduce an
	// SSRF primitive and bypass platform-specific URL trust rules.
	if source.ProviderMediaID == "" || source.URL != "" || source.Kind != item.Kind {
		return p.reject(ctx, item, asset.ID)
	}
	body, _, err := p.Downloader.DownloadInboundMedia(ctx, source.ProviderMediaID, maxConversationImageBytes)
	if err != nil {
		return p.handleProviderFailure(ctx, item, asset.ID, err)
	}
	verified, mime, err := verifyConversationImage(body)
	if err != nil {
		return p.reject(ctx, item, asset.ID)
	}
	writer := p.Objects.(ports.MediaObjectProviderWriter)
	if err := writer.PutToProvider(ctx, asset.StorageProvider, asset.StorageKey, mime, body); err != nil {
		return p.scheduleRetry(ctx, item, "storage_write_failed", 0)
	}
	if _, err := p.Assets.MarkImageReady(ctx, item.BusinessID, asset.ID, verified); err != nil {
		// The deterministic object key makes a retry idempotent. Keep the
		// placeholder and let the lease expire rather than deleting bytes that
		// may already be referenced by a concurrent recovery attempt.
		return err
	}
	_, err = p.Attachments.MarkReady(ctx, item.BusinessID, item.AttachmentID)
	return err
}

func (p *ConversationMediaIngestionProcessor) handleProviderFailure(ctx context.Context, item ports.ClaimedMessageAttachmentIngestion, assetID string, cause error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var retryable interface{ IsRetryable() bool }
	if errors.Is(cause, ports.ErrTransientMediaProvider) ||
		(errors.As(cause, &retryable) && retryable.IsRetryable()) {
		var after interface{ RetryAfterDuration() time.Duration }
		delay := time.Duration(0)
		if errors.As(cause, &after) {
			delay = after.RetryAfterDuration()
		}
		if item.AttemptCount < maxConversationIngestionAttempts {
			return p.scheduleRetry(ctx, item, "provider_transient", delay)
		}
	}
	return p.reject(ctx, item, assetID)
}

func (p *ConversationMediaIngestionProcessor) scheduleRetry(ctx context.Context, item ports.ClaimedMessageAttachmentIngestion, code string, requested time.Duration) error {
	delay := requested
	if delay < time.Second {
		shift := item.AttemptCount - 1
		if shift < 0 {
			shift = 0
		}
		if shift > 8 {
			shift = 8
		}
		delay = 5 * time.Second * time.Duration(1<<shift)
	}
	if delay > 15*time.Minute {
		delay = 15 * time.Minute
	}
	return p.Attachments.ReleaseForRetry(ctx, item.BusinessID, item.AttachmentID, p.now().Add(delay), code)
}

func (p *ConversationMediaIngestionProcessor) reject(ctx context.Context, item ports.ClaimedMessageAttachmentIngestion, assetID string) error {
	if _, err := p.Assets.MarkIngestionRejected(ctx, item.BusinessID, assetID); err != nil {
		return err
	}
	_, err := p.Attachments.MarkFailed(ctx, item.BusinessID, item.AttachmentID)
	return err
}

func verifyConversationImage(data []byte) (ports.VerifiedImage, string, error) {
	if len(data) == 0 || len(data) > maxConversationImageBytes {
		return ports.VerifiedImage{}, "", errors.New("conversation image is empty or exceeds the size limit")
	}
	mime := http.DetectContentType(data)
	if mime != "image/jpeg" && mime != "image/png" {
		return ports.VerifiedImage{}, "", errors.New("conversation attachment is not a supported image")
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width < 1 || config.Height < 1 ||
		config.Width > maxConversationImageEdge || config.Height > maxConversationImageEdge ||
		int64(config.Width)*int64(config.Height) > maxConversationImagePixels {
		return ports.VerifiedImage{}, "", errors.New("conversation image dimensions are invalid")
	}
	if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
		return ports.VerifiedImage{}, "", errors.New("conversation image body is corrupt")
	}
	digest := sha256.Sum256(data)
	return ports.VerifiedImage{
		MimeType: mime, SizeBytes: int64(len(data)), SHA256: digest[:],
		Width: config.Width, Height: config.Height,
	}, mime, nil
}
