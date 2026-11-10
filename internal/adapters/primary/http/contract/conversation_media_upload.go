package contract

import (
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

type ConversationImageUploadRequest struct {
	DataBase64 string `json:"data_base64" minLength:"1" doc:"JPEG or PNG image, base64-encoded. Up to 10 MiB decoded."`
}

type ConversationImageUploadInput struct {
	BusinessID UUID `path:"business_id" format:"uuid"`
	Body       ConversationImageUploadRequest
}

type ConversationMediaAssetView struct {
	ID        string `json:"id"`
	MimeType  string `json:"mime_type"`
	SizeBytes int64  `json:"size_bytes"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Status    string `json:"status"`
}

func registerConversationMediaUploadOperations(api huma.API, dispatcher DashboardOperationHandler) {
	register(api, dispatcher, huma.Operation{
		OperationID:     "uploadConversationImage",
		Method:          http.MethodPost,
		Path:            "/businesses/{business_id}/conversation-media/uploads",
		Tags:            []string{"ConversationMedia"},
		Summary:         "Upload a verified private image for a conversation attachment",
		Security:        dashboardSecurity,
		DefaultStatus:   http.StatusCreated,
		MaxBodyBytes:    14 * 1024 * 1024,
		BodyReadTimeout: 120 * time.Second,
		Errors:          []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusUnprocessableEntity, http.StatusServiceUnavailable},
	}, ConversationImageUploadInput{}, Single[ConversationMediaAssetView]{})
}

