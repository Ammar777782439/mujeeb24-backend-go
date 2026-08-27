package socialapi

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

var ErrAuthorizationCallback = errors.New("socialapi authorization callback is invalid")

type ProvisioningAdapter struct {
	Client *Client
}

func NewProvisioningAdapter(client *Client) *ProvisioningAdapter {
	return &ProvisioningAdapter{Client: client}
}

func (a *ProvisioningAdapter) BeginAuthorization(ctx context.Context, request ports.SocialAuthorizationRequest) (ports.SocialAuthorization, error) {
	if a == nil || a.Client == nil {
		return ports.SocialAuthorization{}, ErrNotConfigured
	}
	response, err := a.Client.BeginConnection(ctx, ConnectRequest{Platform: request.Channel, RedirectURI: request.RedirectURI, State: request.State})
	if err != nil {
		return ports.SocialAuthorization{}, err
	}
	if strings.TrimSpace(response.AuthURL) == "" && strings.TrimSpace(response.AccountID) == "" {
		return ports.SocialAuthorization{}, fmt.Errorf("%w: connect response has neither auth_url nor account_id", ErrInvalidResponse)
	}
	return ports.SocialAuthorization{ProviderAccountRef: response.AccountID, ProviderConnectionRef: response.AccountID, AuthorizationURL: response.AuthURL, State: response.State}, nil
}

func (a *ProvisioningAdapter) ResolveAuthorization(ctx context.Context, callback ports.SocialAuthorizationCallback) (ports.SocialAuthorization, error) {
	if a == nil || a.Client == nil {
		return ports.SocialAuthorization{}, ErrNotConfigured
	}
	if callback.Status == "error" {
		return ports.SocialAuthorization{}, ErrAuthorizationCallback
	}
	if callback.Status == "success" {
		if strings.TrimSpace(callback.AccountID) == "" {
			return ports.SocialAuthorization{}, fmt.Errorf("%w: success callback has no account_id", ErrAuthorizationCallback)
		}
		return ports.SocialAuthorization{ProviderAccountRef: callback.AccountID, ProviderConnectionRef: callback.AccountID, State: callback.State}, nil
	}
	if callback.Status != "selection_required" || strings.TrimSpace(callback.ConnectionID) == "" {
		return ports.SocialAuthorization{}, fmt.Errorf("%w: unsupported callback status", ErrAuthorizationCallback)
	}
	if len(callback.PageIDs) == 0 && callback.PlatformAccountID == "" {
		raw, err := a.Client.GetPendingConnectionRaw(ctx, callback.ConnectionID)
		if err == nil {
			if pages, ok := raw["pages"].([]any); ok {
				for _, page := range pages {
					if pageMap, ok := page.(map[string]any); ok && pageMap["platform_page_id"] != nil {
						callback.PageIDs = append(callback.PageIDs, fmt.Sprintf("%v", pageMap["platform_page_id"]))
					}
				}
			}
			if profiles, ok := raw["profiles"].([]any); ok {
				for _, profile := range profiles {
					if profileMap, ok := profile.(map[string]any); ok && profileMap["platform_account_id"] != nil {
						callback.PlatformAccountID = fmt.Sprintf("%v", profileMap["platform_account_id"])
						break
					}
				}
			}
		}
	}
	selection, err := a.Client.SelectPendingConnection(ctx, callback.ConnectionID, PendingSelectionRequest{PageIDs: callback.PageIDs, PlatformAccountID: callback.PlatformAccountID})
	if err != nil {
		return ports.SocialAuthorization{}, err
	}
	if strings.TrimSpace(selection.AccountID) == "" {
		return ports.SocialAuthorization{}, fmt.Errorf("%w: selection response has no account_id", ErrAuthorizationCallback)
	}
	return ports.SocialAuthorization{ProviderAccountRef: selection.AccountID, ProviderConnectionRef: selection.AccountID, State: callback.State}, nil
}

var _ ports.SocialChannelProvisioner = (*ProvisioningAdapter)(nil)
