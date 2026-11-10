package handlers

import (
	"context"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/contract"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
)

func (s *Server) dispatchConversationMediaUpload(ctx context.Context, operationID string, input any) (any, bool) {
	if operationID != "uploadConversationImage" {
		return nil, false
	}
	in, ok := input.(*contract.ConversationImageUploadInput)
	if !ok {
		return appErrors.New(appErrors.CodeValidation, "invalid conversation media upload input"), true
	}
	actor, err := s.requireScope(ctx, in.BusinessID)
	if err != nil {
		return mapApplicationError(err), true
	}
	if s.deps.ConversationImageUpload.Assets == nil || s.deps.ConversationImageUpload.Objects == nil {
		return mapApplicationError(appErrors.NotImplemented()), true
	}
	var createdBy *string
	if actor.PrincipalID != "" {
		principalID := string(actor.PrincipalID)
		createdBy = &principalID
	}
	asset, err := s.deps.ConversationImageUpload.UploadBase64(ctx, string(commands.BusinessID(actor.BusinessID)), in.Body.DataBase64, createdBy)
	if err != nil {
		return mapApplicationError(err), true
	}
	out := &contract.Single[contract.ConversationMediaAssetView]{}
	out.Body.Data = contract.ConversationMediaAssetView{
		ID: asset.ID, MimeType: asset.MimeType, SizeBytes: asset.SizeBytes,
		Width: asset.Width, Height: asset.Height, Status: asset.Status,
	}
	return out, true
}
