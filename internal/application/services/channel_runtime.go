package services

import (
	"context"
	"log"
	"strconv"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/queries"
)

type ChannelRuntimeService struct {
	Reader        ports.ChannelConnectionRepository
	Runtime       ports.ChannelConnectionRuntimeRepository
	Transactions  ports.TransactionManager
	Disconnector  ports.ChannelAccountDisconnector
	Provisioning  ports.ChannelProvisioningStore
	BrandCleanup  ports.ProviderBrandCleanupStore
	BrandProvider ports.ProviderBrandProvisioner
	HistoryStore  ports.ChannelHistoryStore
}
type ListChannelConnectionsQueryService struct{ ChannelRuntimeService }
type GetChannelConnectionQueryService struct{ ChannelRuntimeService }
type ReconnectChannelCommandService struct{ ChannelRuntimeService }
type DisconnectChannelCommandService struct{ ChannelRuntimeService }
type RemoveChannelCommandService struct{ ChannelRuntimeService }

func (s ListChannelConnectionsQueryService) Handle(ctx context.Context, query queries.ListChannelConnectionsQuery) (commands.ListResult[commands.ChannelConnectionView], error) {
	if s.Runtime == nil {
		return commands.ListResult[commands.ChannelConnectionView]{}, appErrors.NotImplemented()
	}
	page, err := s.Runtime.List(ctx, string(query.Meta.Actor.BusinessID), query.Status, query.Channel, query.Limit, query.Cursor)
	if err != nil {
		return commands.ListResult[commands.ChannelConnectionView]{}, err
	}
	items := make([]commands.ChannelConnectionView, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, channelConnectionView(item))
	}
	return commands.ListResult[commands.ChannelConnectionView]{Items: items, NextCursor: page.NextCursor, HasMore: page.HasMore}, nil
}

func (s GetChannelConnectionQueryService) Handle(ctx context.Context, query queries.GetChannelConnectionQuery) (commands.ChannelConnectionView, error) {
	if s.Reader == nil {
		return commands.ChannelConnectionView{}, appErrors.NotImplemented()
	}
	record, err := s.Reader.GetByID(ctx, string(query.Meta.Actor.BusinessID), string(query.ConnectionID))
	if err != nil {
		return commands.ChannelConnectionView{}, err
	}
	return channelConnectionView(record), nil
}

func (s ReconnectChannelCommandService) Handle(ctx context.Context, command commands.ReconnectChannelCommand) (commands.ChannelConnectionResult, error) {
	if s.Reader == nil || s.Runtime == nil || s.Transactions == nil || s.Disconnector == nil {
		return commands.ChannelConnectionResult{}, appErrors.NotImplemented()
	}
	reason := strings.TrimSpace(command.Reason)
	if reason == "" {
		return commands.ChannelConnectionResult{}, appErrors.New(appErrors.CodeValidation, "connection action reason is required")
	}
	expected, err := parseBusinessExpectedVersion(command.Meta.ExpectedVersion)
	if err != nil {
		return commands.ChannelConnectionResult{}, err
	}

	record, err := s.Reader.GetByID(ctx, string(command.Meta.Actor.BusinessID), string(command.ConnectionID))
	if err != nil {
		return commands.ChannelConnectionResult{}, err
	}
	if record.ResourceVersion != expected {
		return commands.ChannelConnectionResult{}, appErrors.New(appErrors.CodeConflict, "stale channel connection resource version")
	}
	if record.Status == "archived" {
		return commands.ChannelConnectionResult{}, appErrors.New(appErrors.CodeConflict, "archived channel connection cannot be reconnected")
	}
	if record.Status == "reconnect_required" {
		view := channelConnectionView(record)
		result := commands.ChannelConnectionResult{Connection: view}
		result.ResourceVersion = view.ResourceVersion
		return result, nil
	}

	if s.Provisioning != nil {
		if err := s.Provisioning.SupersedeConnectedByChannelConnection(ctx, string(command.Meta.Actor.BusinessID), string(command.ConnectionID)); err != nil {
			return commands.ChannelConnectionResult{}, err
		}
	}

	var result commands.ChannelConnectionResult
	err = s.Transactions.Within(ctx, func(txCtx context.Context) error {
		updated, transitionErr := s.Runtime.Transition(txCtx, ports.ChannelConnectionTransition{
			BusinessID:      string(command.Meta.Actor.BusinessID),
			ConnectionID:    string(command.ConnectionID),
			ExpectedVersion: expected,
			TargetStatus:    "reconnect_required",
			Action:          "reconnect_requested",
			Reason:          reason,
			ActorReference:  string(command.Meta.Actor.PrincipalID),
		})
		if transitionErr != nil {
			return transitionErr
		}
		result.Connection = channelConnectionView(updated)
		result.ResourceVersion = result.Connection.ResourceVersion
		return nil
	})
	return result, err
}
func (s DisconnectChannelCommandService) Handle(ctx context.Context, command commands.DisconnectChannelCommand) (commands.ChannelConnectionResult, error) {
	if s.Reader == nil || s.Runtime == nil || s.Transactions == nil || s.Disconnector == nil {
		return commands.ChannelConnectionResult{}, appErrors.NotImplemented()
	}
	reason := strings.TrimSpace(command.Reason)
	if reason == "" {
		return commands.ChannelConnectionResult{}, appErrors.New(appErrors.CodeValidation, "connection action reason is required")
	}

	record, err := s.Reader.GetByID(ctx, string(command.Meta.Actor.BusinessID), string(command.ConnectionID))
	if err != nil {
		return commands.ChannelConnectionResult{}, err
	}
	if record.Status == "disconnected" {
		if err := s.deleteProviderBrandIfUnused(ctx, record.BusinessID, record.ProviderReference); err != nil {
			return commands.ChannelConnectionResult{}, appErrors.New(appErrors.CodeExternalDependency, "channel was disconnected but provider Brand cleanup remains incomplete; retry disconnect")
		}
		result := commands.ChannelConnectionResult{Connection: channelConnectionView(record)}
		result.ResourceVersion = result.Connection.ResourceVersion
		return result, nil
	}
	if record.Status == "archived" {
		return commands.ChannelConnectionResult{}, appErrors.New(appErrors.CodeConflict, "archived channel connection cannot be disconnected")
	}
	if record.ProviderAccountReference == nil || strings.TrimSpace(*record.ProviderAccountReference) == "" {
		return commands.ChannelConnectionResult{}, appErrors.New(appErrors.CodeConflict, "connected channel has no provider account reference")
	}

	// Provider side is authoritative for the external connection. Disconnect it
	// first, then commit the local lifecycle transition using the exact version
	// observed before the provider mutation. The local transition is deliberately
	// optimistic: concurrent reconnect/status mutations are rejected as stale
	// instead of silently overwriting them.
	if err := s.Disconnector.DisconnectAccount(ctx, strings.TrimSpace(*record.ProviderAccountReference)); err != nil {
		return commands.ChannelConnectionResult{}, err
	}

	expected := record.ResourceVersion
	if expected <= 0 {
		return commands.ChannelConnectionResult{}, appErrors.New(appErrors.CodeConflict, "channel connection has invalid resource version")
	}
	var result commands.ChannelConnectionResult
	err = s.Transactions.Within(ctx, func(txCtx context.Context) error {
		updated, transitionErr := s.Runtime.Transition(txCtx, ports.ChannelConnectionTransition{
			BusinessID:      string(command.Meta.Actor.BusinessID),
			ConnectionID:    string(command.ConnectionID),
			ExpectedVersion: expected,
			TargetStatus:    "disconnected",
			Action:          "disconnect_requested",
			Reason:          reason,
			ActorReference:  string(command.Meta.Actor.PrincipalID),
		})
		if transitionErr != nil {
			return transitionErr
		}
		result.Connection = channelConnectionView(updated)
		result.ResourceVersion = result.Connection.ResourceVersion
		return nil
	})
	if err != nil {
		return result, err
	}
	if cleanupErr := s.deleteProviderBrandIfUnused(ctx, record.BusinessID, record.ProviderReference); cleanupErr != nil {
		return result, appErrors.New(appErrors.CodeExternalDependency, "channel disconnected, provider Brand cleanup incomplete; retry disconnect")
	}
	return result, nil
}

func (s RemoveChannelCommandService) Handle(ctx context.Context, command commands.RemoveChannelCommand) (commands.ChannelConnectionResult, error) {
	// Log lifecycle stages without logging customer messages, tokens or provider account IDs.
	log.Printf("[ChannelRemove] stage=start business=%s connection=%s", command.Meta.Actor.BusinessID, command.ConnectionID)
	if s.Reader == nil || s.Runtime == nil || s.Transactions == nil || s.Disconnector == nil || s.HistoryStore == nil {
		log.Printf("[ChannelRemove] stage=dependencies_unavailable business=%s connection=%s", command.Meta.Actor.BusinessID, command.ConnectionID)
		return commands.ChannelConnectionResult{}, appErrors.NotImplemented()
	}
	if command.Meta.Actor.Role != "owner" && command.Meta.Actor.Role != "admin" {
		log.Printf("[ChannelRemove] stage=authorization_rejected business=%s connection=%s", command.Meta.Actor.BusinessID, command.ConnectionID)
		return commands.ChannelConnectionResult{}, appErrors.New(appErrors.CodeForbidden, "owner or admin role is required")
	}
	if command.Confirmation != "REMOVE_CHANNEL_AND_PURGE_DATA" {
		log.Printf("[ChannelRemove] stage=confirmation_rejected business=%s connection=%s", command.Meta.Actor.BusinessID, command.ConnectionID)
		return commands.ChannelConnectionResult{}, appErrors.New(appErrors.CodeValidation, "explicit REMOVE_CHANNEL_AND_PURGE_DATA confirmation is required")
	}
	reason := strings.TrimSpace(command.Reason)
	if reason == "" {
		reason = "user_channel_removal"
	}
	// Fetch the authoritative record first. An already archived connection
	// can complete a pending provider cleanup on a safe repeated request even
	// if the client no longer holds its previous resource version.
	record, err := s.Reader.GetByID(ctx, string(command.Meta.Actor.BusinessID), string(command.ConnectionID))
	if err != nil {
		log.Printf("[ChannelRemove] stage=load_connection_failed business=%s connection=%s error_type=%T err=%v", command.Meta.Actor.BusinessID, command.ConnectionID, err, err)
		return commands.ChannelConnectionResult{}, err
	}
	log.Printf("[ChannelRemove] stage=loaded business=%s connection=%s status=%s resource_version=%d", command.Meta.Actor.BusinessID, command.ConnectionID, record.Status, record.ResourceVersion)
	if record.Status == "archived" {
		// Repeated removal is idempotent and also resumes Brand cleanup if the
		// prior request committed locally but the provider call failed.
		result := commands.ChannelConnectionResult{Connection: channelConnectionView(record)}
		result.ResourceVersion = result.Connection.ResourceVersion
		log.Printf("[ChannelRemove] stage=archived_retry_brand_cleanup business=%s connection=%s", command.Meta.Actor.BusinessID, command.ConnectionID)
		if err := s.deleteProviderBrandIfUnused(ctx, record.BusinessID, record.ProviderReference); err != nil {
			log.Printf("[ChannelRemove] stage=archived_brand_cleanup_failed business=%s connection=%s error_type=%T err=%v", command.Meta.Actor.BusinessID, command.ConnectionID, err, err)
			return result, appErrors.New(appErrors.CodeExternalDependency, "channel removed locally; provider Brand cleanup is pending, retry removal")
		}
		log.Printf("[ChannelRemove] stage=already_archived_complete business=%s connection=%s", command.Meta.Actor.BusinessID, command.ConnectionID)
		return result, nil
	}
	expected, err := parseBusinessExpectedVersion(command.Meta.ExpectedVersion)
	if err != nil {
		log.Printf("[ChannelRemove] stage=invalid_expected_version business=%s connection=%s err=%v", command.Meta.Actor.BusinessID, command.ConnectionID, err)
		return commands.ChannelConnectionResult{}, err
	}
	if record.ResourceVersion != expected {
		log.Printf("[ChannelRemove] stage=stale_version business=%s connection=%s expected=%d actual=%d", command.Meta.Actor.BusinessID, command.ConnectionID, expected, record.ResourceVersion)
		return commands.ChannelConnectionResult{}, appErrors.New(appErrors.CodeConflict, "stale channel connection resource version")
	}

	// 1. Invalidate any open provisioning sessions
	if s.Provisioning != nil {
		log.Printf("[ChannelRemove] stage=supersede_provisioning_start business=%s connection=%s", command.Meta.Actor.BusinessID, command.ConnectionID)
		if err := s.Provisioning.SupersedeConnectedByChannelConnection(ctx, string(command.Meta.Actor.BusinessID), string(command.ConnectionID)); err != nil {
			log.Printf("[ChannelRemove] stage=supersede_provisioning_failed business=%s connection=%s error_type=%T err=%v", command.Meta.Actor.BusinessID, command.ConnectionID, err, err)
			return commands.ChannelConnectionResult{}, err
		}
	}

	// 2. Fail closed if the provider account could not be disconnected.
	// External calls must remain outside the database transaction.
	if record.ProviderAccountReference != nil && strings.TrimSpace(*record.ProviderAccountReference) != "" {
		log.Printf("[ChannelRemove] stage=provider_disconnect_start business=%s connection=%s", command.Meta.Actor.BusinessID, command.ConnectionID)
		if err := s.Disconnector.DisconnectAccount(ctx, strings.TrimSpace(*record.ProviderAccountReference)); err != nil {
			log.Printf("[ChannelRemove] stage=provider_disconnect_failed business=%s connection=%s error_type=%T err=%v", command.Meta.Actor.BusinessID, command.ConnectionID, err, err)
			return commands.ChannelConnectionResult{}, appErrors.New(appErrors.CodeExternalDependency, "provider disconnect failed; local channel history was not removed")
		}
		log.Printf("[ChannelRemove] stage=provider_disconnect_ok business=%s connection=%s", command.Meta.Actor.BusinessID, command.ConnectionID)
	} else {
		log.Printf("[ChannelRemove] stage=provider_disconnect_skipped business=%s connection=%s reason=no_account_ref", command.Meta.Actor.BusinessID, command.ConnectionID)
	}

	idempotencyKey := strings.TrimSpace(command.Meta.IdempotencyKey)
	if idempotencyKey == "" {
		idempotencyKey = "remove-" + string(command.ConnectionID)
	}

	// 3. Purge channel history and archive the connection in ONE transaction.
	// Any conflict or FK failure rolls back both operations. The history store
	// joins the existing tx rather than opening a nested one.
	var result commands.ChannelConnectionResult
	log.Printf("[ChannelRemove] stage=transaction_start business=%s connection=%s", command.Meta.Actor.BusinessID, command.ConnectionID)
	err = s.Transactions.Within(ctx, func(txCtx context.Context) error {
		log.Printf("[ChannelRemove] stage=history_purge_start business=%s connection=%s", command.Meta.Actor.BusinessID, command.ConnectionID)
		if _, err := s.HistoryStore.Purge(txCtx, string(command.Meta.Actor.BusinessID), string(command.ConnectionID), idempotencyKey, expected); err != nil {
			log.Printf("[ChannelRemove] stage=history_purge_failed business=%s connection=%s error_type=%T err=%v", command.Meta.Actor.BusinessID, command.ConnectionID, err, err)
			return mapChannelHistoryRepositoryError(err)
		}
		log.Printf("[ChannelRemove] stage=history_purge_ok business=%s connection=%s", command.Meta.Actor.BusinessID, command.ConnectionID)
		updated, transitionErr := s.Runtime.Transition(txCtx, ports.ChannelConnectionTransition{
			BusinessID:      string(command.Meta.Actor.BusinessID),
			ConnectionID:    string(command.ConnectionID),
			ExpectedVersion: expected,
			TargetStatus:    "archived",
			Action:          "remove_requested",
			Reason:          reason,
			ActorReference:  string(command.Meta.Actor.PrincipalID),
		})
		if transitionErr != nil {
			log.Printf("[ChannelRemove] stage=archive_failed business=%s connection=%s error_type=%T err=%v", command.Meta.Actor.BusinessID, command.ConnectionID, transitionErr, transitionErr)
			return transitionErr
		}
		log.Printf("[ChannelRemove] stage=archive_ok business=%s connection=%s", command.Meta.Actor.BusinessID, command.ConnectionID)
		result.Connection = channelConnectionView(updated)
		result.ResourceVersion = result.Connection.ResourceVersion
		return nil
	})
	if err != nil {
		log.Printf("[ChannelRemove] stage=transaction_rolled_back business=%s connection=%s error_type=%T err=%v", command.Meta.Actor.BusinessID, command.ConnectionID, err, err)
		return result, err
	}
	log.Printf("[ChannelRemove] stage=transaction_committed business=%s connection=%s", command.Meta.Actor.BusinessID, command.ConnectionID)

	// 5. If this was the last active/connected channel for this business under this provider, delete the Brand at SocialAPI to free up quota
	log.Printf("[ChannelRemove] stage=brand_cleanup_start business=%s connection=%s", command.Meta.Actor.BusinessID, command.ConnectionID)
	if cleanupErr := s.deleteProviderBrandIfUnused(ctx, record.BusinessID, record.ProviderReference); cleanupErr != nil {
		log.Printf("[ChannelRemove] stage=brand_cleanup_failed business=%s connection=%s error_type=%T err=%v", command.Meta.Actor.BusinessID, command.ConnectionID, cleanupErr, cleanupErr)
		return result, appErrors.New(appErrors.CodeExternalDependency, "channel removed locally; provider Brand cleanup is pending, retry removal")
	}

	log.Printf("[ChannelRemove] stage=completed business=%s connection=%s", command.Meta.Actor.BusinessID, command.ConnectionID)
	return result, nil
}

// deleteProviderBrandIfUnused never calls SocialAPI within a database transaction.
func (s ChannelRuntimeService) deleteProviderBrandIfUnused(ctx context.Context, businessID, providerRef string) error {
	if s.BrandCleanup == nil || s.BrandProvider == nil || providerRef != "socialapi" {
		return nil
	}
	if s.Provisioning != nil {
		_ = s.Provisioning.CancelPendingByBusiness(ctx, businessID, providerRef)
	}
	brandID, reserved, err := s.BrandCleanup.ReserveUnused(ctx, businessID, providerRef)
	if err != nil {
		return err
	}
	if !reserved {
		return nil
	}
	if err := s.BrandProvider.DeleteBrand(ctx, brandID); err != nil {
		// The persistent 'deleting' state prevents any new OAuth session from
		// reusing the Brand. Retrying disconnect resumes cleanup idempotently.
		return err
	}
	return s.BrandCleanup.CompleteDeletion(ctx, businessID, providerRef, brandID)
}

func channelConnectionView(record ports.ChannelConnectionRecord) commands.ChannelConnectionView {
	account := ""
	if record.ProviderAccountReference != nil {
		account = *record.ProviderAccountReference
	}
	return commands.ChannelConnectionView{ID: commands.ConnectionID(record.ID), BusinessID: commands.BusinessID(record.BusinessID), Provider: record.ProviderReference, Channel: record.Channel, Status: record.Status, ExternalAccountReference: account, ResourceVersion: commands.ResourceVersion(strconv.FormatInt(record.ResourceVersion, 10))}
}

var _ queries.ListChannelConnectionsHandler = ListChannelConnectionsQueryService{}
var _ queries.GetChannelConnectionHandler = GetChannelConnectionQueryService{}
var _ commands.ReconnectChannelHandler = ReconnectChannelCommandService{}
var _ commands.DisconnectChannelHandler = DisconnectChannelCommandService{}
var _ commands.RemoveChannelHandler = RemoveChannelCommandService{}
