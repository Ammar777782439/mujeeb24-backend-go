package services

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type conversationMediaAttachmentsFake struct {
	claimed []ports.ClaimedMessageAttachmentIngestion
	retried int
	retryAt time.Time
	retryCode string
	ready int
	failed int
}

func (f *conversationMediaAttachmentsFake) Create(context.Context, ports.MessageAttachmentDraft) (ports.MessageAttachmentRecord, error) {
	return ports.MessageAttachmentRecord{}, errors.New("not used")
}
func (f *conversationMediaAttachmentsFake) ListByMessage(context.Context, string, string) ([]ports.MessageAttachmentRecord, error) {
	return nil, errors.New("not used")
}
func (f *conversationMediaAttachmentsFake) ClaimPendingIngestions(context.Context, int, time.Duration) ([]ports.ClaimedMessageAttachmentIngestion, error) {
	return f.claimed, nil
}
func (f *conversationMediaAttachmentsFake) ReleaseForRetry(_ context.Context, _, _ string, at time.Time, code string) error {
	f.retried++
	f.retryAt, f.retryCode = at, code
	return nil
}
func (f *conversationMediaAttachmentsFake) MarkReady(context.Context, string, string) (ports.MessageAttachmentRecord, error) {
	f.ready++
	return ports.MessageAttachmentRecord{Status: "ready"}, nil
}
func (f *conversationMediaAttachmentsFake) MarkFailed(context.Context, string, string) (ports.MessageAttachmentRecord, error) {
	f.failed++
	return ports.MessageAttachmentRecord{Status: "failed"}, nil
}

type conversationMediaAssetsFake struct {
	asset ports.MediaAsset
	rejected int
	marked ports.VerifiedImage
}

func (f *conversationMediaAssetsFake) CreatePendingIngestion(context.Context, ports.PendingConversationMediaAsset) (ports.MediaAsset, error) {
	return ports.MediaAsset{}, errors.New("not used")
}
func (f *conversationMediaAssetsFake) GetAsset(context.Context, string, string) (ports.MediaAsset, error) {
	return f.asset, nil
}
func (f *conversationMediaAssetsFake) MarkImageReady(_ context.Context, _, _ string, verified ports.VerifiedImage) (ports.MediaAsset, error) {
	f.marked = verified
	f.asset.Status = "ready"
	f.asset.MimeType = verified.MimeType
	return f.asset, nil
}
func (f *conversationMediaAssetsFake) MarkIngestionRejected(context.Context, string, string) (ports.MediaAsset, error) {
	f.rejected++
	f.asset.Status = "rejected"
	return f.asset, nil
}

type conversationMediaResolverFake struct {
	source ports.InboundMediaSource
	err error
	calls int
}
func (f *conversationMediaResolverFake) ResolveInboundMediaSource(context.Context, string, string, string) (ports.InboundMediaSource, error) {
	f.calls++
	return f.source, f.err
}

type conversationMediaDownloaderFake struct {
	body []byte
	err error
	calls int
}
func (f *conversationMediaDownloaderFake) DownloadInboundMedia(context.Context, string, int64) ([]byte, string, error) {
	f.calls++
	return f.body, "image/png", f.err
}

type conversationMediaObjectsFake struct {
	provider string
	key string
	mime string
	body []byte
	puts int
}
func (f *conversationMediaObjectsFake) Put(context.Context, string, string, []byte) error {
	return errors.New("provider-pinned Put must be used")
}
func (f *conversationMediaObjectsFake) PutToProvider(_ context.Context, provider, key, mime string, body []byte) error {
	f.provider, f.key, f.mime, f.body = provider, key, mime, append([]byte(nil), body...)
	f.puts++
	return nil
}
func (f *conversationMediaObjectsFake) DeleteFromProvider(context.Context, string, string) error { return nil }
func (f *conversationMediaObjectsFake) Delete(context.Context, string) error { return nil }
func (f *conversationMediaObjectsFake) PresignGet(context.Context, string, time.Duration) (string, error) {
	return "", errors.New("not used")
}

type conversationMediaRetryError struct { delay time.Duration }
func (e conversationMediaRetryError) Error() string { return "provider temporarily unavailable" }
func (e conversationMediaRetryError) IsRetryable() bool { return true }
func (e conversationMediaRetryError) RetryAfterDuration() time.Duration { return e.delay }

func validConversationPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 20, G: 40, B: 60, A: 255})
	var data bytes.Buffer
	if err := png.Encode(&data, img); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func conversationMediaFixture(t *testing.T) (*ConversationMediaIngestionProcessor, *conversationMediaAttachmentsFake, *conversationMediaAssetsFake, *conversationMediaResolverFake, *conversationMediaDownloaderFake, *conversationMediaObjectsFake) {
	t.Helper()
	attachments := &conversationMediaAttachmentsFake{claimed: []ports.ClaimedMessageAttachmentIngestion{{
		AttachmentID: "attachment-1", BusinessID: "business-1", CommunicationMessageID: "message-1",
		MediaAssetID: "asset-1", ProviderConversationID: "provider-conversation-1",
		Channel: "whatsapp", ProviderMediaReference: "opaque-ref", Kind: "image",
		StorageProvider: "backblaze", StorageKey: "conversation/business-1/asset-1.png", AttemptCount: 1,
	}}}
	assets := &conversationMediaAssetsFake{asset: ports.MediaAsset{
		ID: "asset-1", BusinessID: "business-1", Purpose: "conversation_attachment",
		Kind: "image", StorageProvider: "backblaze", StorageKey: "conversation/business-1/asset-1.png",
		Status: "pending_ingestion",
	}}
	resolver := &conversationMediaResolverFake{source: ports.InboundMediaSource{ProviderMediaID: "provider-media-1", Kind: "image"}}
	downloader := &conversationMediaDownloaderFake{body: validConversationPNG(t)}
	objects := &conversationMediaObjectsFake{}
	processor := &ConversationMediaIngestionProcessor{
		Attachments: attachments, Assets: assets, Resolver: resolver, Downloader: downloader, Objects: objects,
		Now: func() time.Time { return time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC) },
	}
	return processor, attachments, assets, resolver, downloader, objects
}

func TestConversationMediaIngestionStoresVerifiedImageOnRecordedProvider(t *testing.T) {
	processor, attachments, assets, resolver, downloader, objects := conversationMediaFixture(t)
	processed, err := processor.Process(context.Background(), 10)
	if err != nil || processed != 1 {
		t.Fatalf("processed=%d err=%v", processed, err)
	}
	if resolver.calls != 1 || downloader.calls != 1 || objects.puts != 1 ||
		objects.provider != "backblaze" || objects.key != assets.asset.StorageKey ||
		objects.mime != "image/png" || assets.asset.Status != "ready" ||
		assets.marked.Width != 2 || assets.marked.Height != 2 || len(assets.marked.SHA256) != 32 ||
		attachments.ready != 1 || attachments.failed != 0 || attachments.retried != 0 {
		t.Fatalf("unexpected ingestion state: resolver=%d downloader=%d objects=%#v asset=%#v verified=%#v attachments=%#v",
			resolver.calls, downloader.calls, objects, assets.asset, assets.marked, attachments)
	}
}

func TestConversationMediaIngestionRetriesProvider504WithoutLoggingProviderDetails(t *testing.T) {
	processor, attachments, assets, resolver, downloader, objects := conversationMediaFixture(t)
	resolver.err = conversationMediaRetryError{delay: 17 * time.Second}
	processor.Now = func() time.Time { return time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC) }
	processed, err := processor.Process(context.Background(), 10)
	if err != nil || processed != 1 {
		t.Fatalf("processed=%d err=%v", processed, err)
	}
	if attachments.retried != 1 || attachments.retryCode != "provider_transient" ||
		!attachments.retryAt.Equal(processor.Now().Add(17*time.Second)) ||
		assets.rejected != 0 || downloader.calls != 0 || objects.puts != 0 {
		t.Fatalf("provider retry was not scheduled correctly: attachments=%#v assets=%#v downloader=%d objects=%d",
			attachments, assets, downloader.calls, objects.puts)
	}
}

func TestConversationMediaIngestionRejectsArbitraryExternalURL(t *testing.T) {
	processor, attachments, assets, resolver, downloader, objects := conversationMediaFixture(t)
	resolver.source = ports.InboundMediaSource{URL: "https://attacker.example.test/image.png", Kind: "image"}
	processed, err := processor.Process(context.Background(), 10)
	if err != nil || processed != 1 {
		t.Fatalf("processed=%d err=%v", processed, err)
	}
	if assets.rejected != 1 || attachments.failed != 1 || downloader.calls != 0 || objects.puts != 0 {
		t.Fatalf("arbitrary URL was not rejected safely: assets=%#v attachments=%#v downloader=%d objects=%d",
			assets, attachments, downloader.calls, objects.puts)
	}
}

func TestConversationMediaIngestionRecoversReadyAssetWithoutRedownload(t *testing.T) {
	processor, attachments, _, resolver, downloader, objects := conversationMediaFixture(t)
	processor.Assets.(*conversationMediaAssetsFake).asset.Status = "ready"
	processed, err := processor.Process(context.Background(), 10)
	if err != nil || processed != 1 || attachments.ready != 1 ||
		resolver.calls != 0 || downloader.calls != 0 || objects.puts != 0 {
		t.Fatalf("ready-asset recovery failed: processed=%d err=%v attachments=%#v resolver=%d downloader=%d puts=%d",
			processed, err, attachments, resolver.calls, downloader.calls, objects.puts)
	}
}
