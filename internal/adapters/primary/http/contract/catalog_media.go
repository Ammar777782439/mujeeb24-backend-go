package contract

import (
	"github.com/danielgtaylor/huma/v2"
	"net/http"
	"time"
)

type CatalogImageUploadRequest struct {
	DataBase64 string `json:"data_base64" minLength:"1" doc:"JPEG or PNG binary, base64-encoded. Up to 10 MiB decoded."`
}
type CatalogImageUploadInput struct {
	BusinessID UUID `path:"business_id" format:"uuid"`
	Body       CatalogImageUploadRequest
}
type CatalogMediaReadInput struct {
	BusinessID UUID `path:"business_id" format:"uuid"`
	MediaID    UUID `path:"media_id" format:"uuid"`
}
type CatalogMediaPreviewView struct {
	DataBase64 string `json:"data_base64" doc:"Base64-encoded JPEG/PNG image from merchant-authorized private storage. No provider links returned."`
	MimeType string `json:"mime_type"`
	Width int `json:"width"`
	Height int `json:"height"`
}
type CatalogMediaPreviewOutput struct {
	CacheControl string `header:"Cache-Control"`
	Body struct { Data CatalogMediaPreviewView `json:"data"` }
}
type CatalogMediaReadView struct {
	URL              string `json:"url"`
	ExpiresInSeconds int    `json:"expires_in_seconds"`
	MimeType         string `json:"mime_type"`
	Width            int    `json:"width"`
	Height           int    `json:"height"`
}
type CatalogMediaAssetView struct {
	ID        string `json:"id"`
	MimeType  string `json:"mime_type"`
	SizeBytes int64  `json:"size_bytes"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Status    string `json:"status"`
}
type CatalogGalleryInput struct {
	BusinessID UUID   `path:"business_id" format:"uuid"`
	ItemID     UUID   `path:"item_id" format:"uuid"`
	VariantID  string `query:"variant_id" format:"uuid" doc:"Optional exact variant; omitted for product gallery"`
}
type CatalogMediaAttachRequest struct {
	MediaID   UUID   `json:"media_id" format:"uuid"`
	IsPrimary bool   `json:"is_primary"`
	SortOrder int    `json:"sort_order" minimum:"0"`
	AltText   string `json:"alt_text" maxLength:"500"`
}
type CatalogMediaAttachInput struct {
	BusinessID UUID   `path:"business_id" format:"uuid"`
	ItemID     UUID   `path:"item_id" format:"uuid"`
	VariantID  string `query:"variant_id" format:"uuid"`
	Body       CatalogMediaAttachRequest
}
type CatalogMediaLinkInput struct {
	BusinessID UUID   `path:"business_id" format:"uuid"`
	ItemID     UUID   `path:"item_id" format:"uuid"`
	LinkID     UUID   `path:"link_id" format:"uuid"`
	VariantID  string `query:"variant_id" format:"uuid"`
}
type CatalogMediaView struct {
	ID              string  `json:"id"`
	MediaID         string  `json:"media_id"`
	CatalogItemID   string  `json:"catalog_item_id"`
	VariantID       *string `json:"variant_id,omitempty"`
	IsPrimary       bool    `json:"is_primary"`
	SortOrder       int     `json:"sort_order"`
	AltText         string  `json:"alt_text"`
	ResourceVersion int64   `json:"resource_version"`
}
type CatalogMediaListOutput struct {
	Body struct {
		Data []CatalogMediaView `json:"data"`
	}
}

func registerCatalogMediaOperations(api huma.API, dispatcher DashboardOperationHandler) {
	register(api, dispatcher, huma.Operation{OperationID: "getCatalogMediaPreview", Method: http.MethodGet,
		Path: "/businesses/{business_id}/media/{media_id}/preview", Tags: []string{"CatalogMedia"},
		Summary: "Fetch an authorized private catalog photo through Mujeeb, without exposing storage URLs",
		Security: dashboardSecurity}, CatalogMediaReadInput{}, CatalogMediaPreviewOutput{})
	register(api, dispatcher, huma.Operation{OperationID: "getCatalogMediaReadURL", Method: http.MethodGet,
		Path: "/businesses/{business_id}/media/{media_id}/read-url", Tags: []string{"CatalogMedia"},
		Summary: "Get short-lived private catalog image access URL", Security: dashboardSecurity},
		CatalogMediaReadInput{}, Single[CatalogMediaReadView]{})
	register(api, dispatcher, huma.Operation{OperationID: "uploadCatalogImage", Method: http.MethodPost,
		Path: "/businesses/{business_id}/media/uploads", Tags: []string{"CatalogMedia"},
		Summary: "Upload merchant catalog image to private storage", Security: dashboardSecurity, DefaultStatus: http.StatusCreated, MaxBodyBytes: 14 * 1024 * 1024, BodyReadTimeout: 120 * time.Second},
		CatalogImageUploadInput{}, Single[CatalogMediaAssetView]{})
	register(api, dispatcher, huma.Operation{OperationID: "listCatalogMedia", Method: http.MethodGet,
		Path: "/businesses/{business_id}/catalog-items/{item_id}/media", Tags: []string{"CatalogMedia"},
		Summary: "List product or variant image gallery", Security: dashboardSecurity},
		CatalogGalleryInput{}, CatalogMediaListOutput{})
	register(api, dispatcher, huma.Operation{OperationID: "attachCatalogMedia", Method: http.MethodPost,
		Path: "/businesses/{business_id}/catalog-items/{item_id}/media", Tags: []string{"CatalogMedia"},
		Summary: "Attach approved image to product or variant", Security: dashboardSecurity, DefaultStatus: http.StatusCreated},
		CatalogMediaAttachInput{}, Single[CatalogMediaView]{})
	register(api, dispatcher, huma.Operation{OperationID: "setCatalogMediaPrimary", Method: http.MethodPost,
		Path: "/businesses/{business_id}/catalog-items/{item_id}/media/{link_id}/primary", Tags: []string{"CatalogMedia"},
		Summary: "Select product or variant gallery primary image", Security: dashboardSecurity},
		CatalogMediaLinkInput{}, Single[CatalogMediaView]{})
	register(api, dispatcher, huma.Operation{OperationID: "removeCatalogMedia", Method: http.MethodDelete,
		Path: "/businesses/{business_id}/catalog-items/{item_id}/media/{link_id}", Tags: []string{"CatalogMedia"},
		Summary: "Remove image link from product or variant", Security: dashboardSecurity},
		CatalogMediaLinkInput{}, Single[CatalogMediaView]{})
}
