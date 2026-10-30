package handlers

import (
	"context"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/contract"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

func optionalGalleryVariant(value string) (*string, error) {
	v := strings.TrimSpace(value)
	if v == "" {
		return nil, nil
	}
	if _, err := uuid.Parse(v); err != nil {
		return nil, huma.Error422UnprocessableEntity("invalid variant_id")
	}
	return &v, nil
}
func mediaLinkView(m ports.CatalogMediaLink) contract.CatalogMediaView {
	return contract.CatalogMediaView{ID: m.ID, MediaID: m.MediaID, CatalogItemID: m.CatalogItemID,
		VariantID: m.VariantID, IsPrimary: m.IsPrimary, SortOrder: m.SortOrder, AltText: m.AltText,
		ResourceVersion: m.ResourceVersion}
}
func (s *Server) dispatchCatalogMedia(ctx context.Context, operationID string, input any) (any, bool) {
	if operationID != "getCatalogMediaReadURL" && operationID != "uploadCatalogImage" && operationID != "listCatalogMedia" &&
		operationID != "attachCatalogMedia" && operationID != "setCatalogMediaPrimary" && operationID != "removeCatalogMedia" {
		return nil, false
	}
	var businessID contract.UUID
	switch in := input.(type) {
	case *contract.CatalogImageUploadInput:
		businessID = in.BusinessID
	case *contract.CatalogMediaReadInput:
		businessID = in.BusinessID
	case *contract.CatalogGalleryInput:
		businessID = in.BusinessID
	case *contract.CatalogMediaAttachInput:
		businessID = in.BusinessID
	case *contract.CatalogMediaLinkInput:
		businessID = in.BusinessID
	default:
		return huma.Error500InternalServerError("invalid media operation input"), true
	}
	actor, err := s.requireScope(ctx, businessID)
	if err != nil {
		return mapApplicationError(err), true
	}
	if s.deps.CatalogMediaService.Media == nil {
		return huma.Error503ServiceUnavailable("catalog media is not configured"), true
	}
	business := string(actor.BusinessID)
	var actorID *string
	if actor.PrincipalID != "" {
		value := string(actor.PrincipalID)
		actorID = &value
	}
	switch in := input.(type) {
	case *contract.CatalogMediaReadInput:
		if s.deps.CatalogImageUpload.Objects == nil {
			return huma.Error503ServiceUnavailable("private image storage is not configured"), true
		}
		asset, err := s.deps.CatalogMediaService.Media.GetAsset(ctx, business, string(in.MediaID))
		if err != nil || asset.Status != "ready" || asset.DeletedAt != nil {
			return huma.Error404NotFound("media not found"), true
		}
		u, err := s.deps.CatalogImageUpload.Objects.PresignGet(ctx, asset.StorageKey, 10*time.Minute)
		if err != nil {
			return huma.Error503ServiceUnavailable("cannot access private image"), true
		}
		out := &contract.Single[contract.CatalogMediaReadView]{}
		out.Body.Data = contract.CatalogMediaReadView{URL: u, ExpiresInSeconds: 600, MimeType: asset.MimeType, Width: asset.Width, Height: asset.Height}
		return out, true
	case *contract.CatalogImageUploadInput:
		if s.deps.CatalogImageUpload.Objects == nil {
			return huma.Error503ServiceUnavailable("private image storage is not configured"), true
		}
		asset, err := s.deps.CatalogImageUpload.UploadBase64(ctx, business, in.Body.DataBase64, actorID)
		if err != nil {
			return huma.Error422UnprocessableEntity("image upload could not be verified or stored"), true
		}
		out := &contract.Single[contract.CatalogMediaAssetView]{}
		out.Body.Data = contract.CatalogMediaAssetView{
			ID: asset.ID, MimeType: asset.MimeType, SizeBytes: asset.SizeBytes, Width: asset.Width,
			Height: asset.Height, Status: asset.Status,
		}
		return out, true
	case *contract.CatalogGalleryInput:
		variant, err := optionalGalleryVariant(in.VariantID)
		if err != nil {
			return err, true
		}
		list, err := s.deps.CatalogMediaService.Media.ListGallery(ctx, business, string(in.ItemID), variant)
		if err != nil {
			return huma.Error500InternalServerError("cannot retrieve media gallery"), true
		}
		out := &contract.CatalogMediaListOutput{}
		out.Body.Data = make([]contract.CatalogMediaView, 0, len(list))
		for _, m := range list {
			out.Body.Data = append(out.Body.Data, mediaLinkView(m))
		}
		return out, true
	case *contract.CatalogMediaAttachInput:
		variant, err := optionalGalleryVariant(in.VariantID)
		if err != nil {
			return err, true
		}
		link, err := s.deps.CatalogMediaService.Attach(ctx, business, string(in.ItemID), variant,
			string(in.Body.MediaID), in.Body.IsPrimary, in.Body.SortOrder, in.Body.AltText, actorID)
		if err != nil {
			return huma.Error422UnprocessableEntity("cannot attach image to this product gallery"), true
		}
		out := &contract.Single[contract.CatalogMediaView]{}
		out.Body.Data = mediaLinkView(link)
		return out, true
	case *contract.CatalogMediaLinkInput:
		variant, err := optionalGalleryVariant(in.VariantID)
		if err != nil {
			return err, true
		}
		itemID, linkID := string(in.ItemID), string(in.LinkID)
		if operationID == "setCatalogMediaPrimary" {
			if err := s.deps.CatalogMediaService.SetPrimary(ctx, business, itemID, variant, linkID); err != nil {
				return huma.Error422UnprocessableEntity("cannot set primary image"), true
			}
			gallery, err := s.deps.CatalogMediaService.Media.ListGallery(ctx, business, itemID, variant)
			if err != nil {
				return huma.Error500InternalServerError("cannot retrieve media gallery"), true
			}
			for _, link := range gallery {
				if link.ID == linkID {
					out := &contract.Single[contract.CatalogMediaView]{}
					out.Body.Data = mediaLinkView(link)
					return out, true
				}
			}
			return huma.Error500InternalServerError("updated media link missing"), true
		}
		if err := s.deps.CatalogMediaService.Remove(ctx, business, itemID, variant, linkID); err != nil {
			return huma.Error422UnprocessableEntity("cannot remove image link"), true
		}
		out := &contract.Single[contract.CatalogMediaView]{}
		out.Body.Data = contract.CatalogMediaView{ID: linkID, CatalogItemID: itemID, VariantID: variant}
		return out, true
	}
	return huma.Error500InternalServerError("unsupported media operation"), true
}
