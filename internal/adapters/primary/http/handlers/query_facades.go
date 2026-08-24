package handlers

import (
	"context"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/contract"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/queries"
)

func (s *Server) dispatchAdditionalQuery(ctx context.Context, operationID string, input any) (any, bool) {
	switch operationID {
	case "getBusinessPolicy":
		in := input.(*contract.BusinessPath)
		if s.deps.GetBusinessPolicy == nil {
			return mapApplicationError(appErrors.NotImplemented()), true
		}
		actor, err := s.requireScope(ctx, in.BusinessID)
		if err != nil {
			return mapApplicationError(err), true
		}
		view, err := s.deps.GetBusinessPolicy.Handle(ctx, queries.GetBusinessPolicyQuery{Meta: queryMeta(actor, "", "")})
		if err != nil {
			return mapApplicationError(err), true
		}
		out := &contract.Single[contract.BusinessPolicy]{}
		out.Body.Data = businessPolicyProjection(view)
		return out, true
	case "getDashboardOverview":
		in := input.(*contract.BusinessPath)
		if s.deps.GetDashboardOverview == nil {
			return mapApplicationError(appErrors.NotImplemented()), true
		}
		actor, err := s.requireScope(ctx, in.BusinessID)
		if err != nil {
			return mapApplicationError(err), true
		}
		view, err := s.deps.GetDashboardOverview.Handle(ctx, queries.GetDashboardOverviewQuery{Meta: queryMeta(actor, "", "")})
		if err != nil {
			return mapApplicationError(err), true
		}
		out := &contract.Single[contract.DashboardOverview]{}
		out.Body.Data = contract.DashboardOverview{OpenConversations: view.OpenConversations, WaitingHuman: view.WaitingHuman, NewCustomers: view.NewCustomers, NewLeads: view.NewLeads, TransactionsNeedingReview: view.TransactionsNeedingReview}
		return out, true
	case "listChannelConnections":
		in := input.(*contract.ConnectionListInput)
		if s.deps.ListChannelConnections == nil {
			return mapApplicationError(appErrors.NotImplemented()), true
		}
		actor, err := s.requireScope(ctx, in.BusinessID)
		if err != nil {
			return mapApplicationError(err), true
		}
		view, err := s.deps.ListChannelConnections.Handle(ctx, queries.ListChannelConnectionsQuery{Meta: queryMeta(actor, "", ""), Limit: in.Limit, Cursor: in.Cursor, Status: in.Status, Channel: in.Channel})
		if err != nil {
			return mapApplicationError(err), true
		}
		return channelConnectionList(view), true
	case "getChannelConnection":
		in := input.(*contract.ConnectionPath)
		if s.deps.GetChannelConnection == nil {
			return mapApplicationError(appErrors.NotImplemented()), true
		}
		actor, err := s.requireScope(ctx, in.BusinessID)
		if err != nil {
			return mapApplicationError(err), true
		}
		view, err := s.deps.GetChannelConnection.Handle(ctx, queries.GetChannelConnectionQuery{Meta: queryMeta(actor, "", ""), ConnectionID: commands.ConnectionID(in.ConnectionID)})
		if err != nil {
			return mapApplicationError(err), true
		}
		out := &contract.Single[contract.ChannelConnection]{}
		out.Body.Data = channelConnectionProjection(view)
		return out, true
	case "getConnectionCapabilities":
		in := input.(*contract.ConnectionPath)
		if s.deps.GetConnectionCapabilities == nil {
			return mapApplicationError(appErrors.NotImplemented()), true
		}
		actor, err := s.requireScope(ctx, in.BusinessID)
		if err != nil {
			return mapApplicationError(err), true
		}
		view, err := s.deps.GetConnectionCapabilities.Handle(ctx, queries.GetConnectionCapabilitiesQuery{Meta: queryMeta(actor, "", ""), ConnectionID: commands.ConnectionID(in.ConnectionID)})
		if err != nil {
			return mapApplicationError(err), true
		}
		return capabilityList(view), true
	case "listConversationMessages":
		in := input.(*contract.ConversationMessageListInput)
		if s.deps.ListConversationMessages == nil {
			return mapApplicationError(appErrors.NotImplemented()), true
		}
		actor, err := s.requireScope(ctx, in.BusinessID)
		if err != nil {
			return mapApplicationError(err), true
		}
		view, err := s.deps.ListConversationMessages.Handle(ctx, queries.ListConversationMessagesQuery{Meta: queryMeta(actor, "", ""), ConversationID: commands.ConversationID(in.ConversationID), Limit: in.Limit, Cursor: in.Cursor})
		if err != nil {
			return mapApplicationError(err), true
		}
		return messageList(view), true
	case "getCustomer":
		in := input.(*contract.CustomerInput)
		if s.deps.GetCustomer == nil {
			return mapApplicationError(appErrors.NotImplemented()), true
		}
		actor, err := s.requireScope(ctx, in.BusinessID)
		if err != nil {
			return mapApplicationError(err), true
		}
		view, err := s.deps.GetCustomer.Handle(ctx, queries.GetCustomerQuery{Meta: queryMeta(actor, "", ""), CustomerID: commands.CustomerID(in.CustomerID)})
		if err != nil {
			return mapApplicationError(err), true
		}
		out := &contract.Single[contract.Customer]{}
		out.Body.Data = customerProjection(view)
		return out, true
	case "listCustomerConversations":
		in := input.(*contract.CustomerConversationsInput)
		if s.deps.ListCustomerConversations == nil {
			return mapApplicationError(appErrors.NotImplemented()), true
		}
		actor, err := s.requireScope(ctx, in.BusinessID)
		if err != nil {
			return mapApplicationError(err), true
		}
		view, err := s.deps.ListCustomerConversations.Handle(ctx, queries.ListCustomerConversationsQuery{Meta: queryMeta(actor, "", ""), CustomerID: commands.CustomerID(in.CustomerID), Limit: in.Limit, Cursor: in.Cursor})
		if err != nil {
			return mapApplicationError(err), true
		}
		return conversationList(view), true
	case "listCustomerTransactions":
		in := input.(*contract.CustomerTransactionsInput)
		if s.deps.ListCustomerTransactions == nil {
			return mapApplicationError(appErrors.NotImplemented()), true
		}
		actor, err := s.requireScope(ctx, in.BusinessID)
		if err != nil {
			return mapApplicationError(err), true
		}
		view, err := s.deps.ListCustomerTransactions.Handle(ctx, queries.ListCustomerTransactionsQuery{Meta: queryMeta(actor, "", ""), CustomerID: commands.CustomerID(in.CustomerID), Limit: in.Limit, Cursor: in.Cursor})
		if err != nil {
			return mapApplicationError(err), true
		}
		return transactionList(view), true
	case "listCatalogs":
		in := input.(*contract.BusinessListInput)
		if s.deps.ListCatalogs == nil {
			return mapApplicationError(appErrors.NotImplemented()), true
		}
		actor, err := s.requireScope(ctx, in.BusinessID)
		if err != nil {
			return mapApplicationError(err), true
		}
		view, err := s.deps.ListCatalogs.Handle(ctx, queries.ListCatalogsQuery{Meta: queryMeta(actor, "", ""), Limit: in.Limit, Cursor: in.Cursor})
		if err != nil {
			return mapApplicationError(err), true
		}
		return catalogList(view), true
	case "getCatalog":
		in := input.(*contract.CatalogPath)
		if s.deps.GetCatalog == nil {
			return mapApplicationError(appErrors.NotImplemented()), true
		}
		actor, err := s.requireScope(ctx, in.BusinessID)
		if err != nil {
			return mapApplicationError(err), true
		}
		view, err := s.deps.GetCatalog.Handle(ctx, queries.GetCatalogQuery{Meta: queryMeta(actor, "", ""), CatalogID: commands.CatalogID(in.CatalogID)})
		if err != nil {
			return mapApplicationError(err), true
		}
		out := &contract.Single[contract.Catalog]{}
		out.Body.Data = catalogProjection(view)
		return out, true
	case "listCatalogItems":
		in := input.(*contract.CatalogItemsInput)
		if s.deps.ListCatalogItems == nil {
			return mapApplicationError(appErrors.NotImplemented()), true
		}
		actor, err := s.requireScope(ctx, in.BusinessID)
		if err != nil {
			return mapApplicationError(err), true
		}
		view, err := s.deps.ListCatalogItems.Handle(ctx, queries.ListCatalogItemsQuery{Meta: queryMeta(actor, "", ""), CatalogID: commands.CatalogID(in.CatalogID), Limit: in.Limit, Cursor: in.Cursor, Search: in.Search, Status: in.Status})
		if err != nil {
			return mapApplicationError(err), true
		}
		return catalogItemList(view), true
	case "getCatalogItem":
		in := input.(*contract.CatalogItemPath)
		if s.deps.GetCatalogItem == nil {
			return mapApplicationError(appErrors.NotImplemented()), true
		}
		actor, err := s.requireScope(ctx, in.BusinessID)
		if err != nil {
			return mapApplicationError(err), true
		}
		view, err := s.deps.GetCatalogItem.Handle(ctx, queries.GetCatalogItemQuery{Meta: queryMeta(actor, "", ""), CatalogID: commands.CatalogID(in.CatalogID), ItemID: commands.CatalogItemID(in.ItemID)})
		if err != nil {
			return mapApplicationError(err), true
		}
		out := &contract.Single[contract.CatalogItem]{}
		out.Body.Data = catalogItemProjection(view)
		return out, true
	case "listOffers":
		in := input.(*contract.ItemOffersInput)
		if s.deps.ListOffers == nil {
			return mapApplicationError(appErrors.NotImplemented()), true
		}
		actor, err := s.requireScope(ctx, in.BusinessID)
		if err != nil {
			return mapApplicationError(err), true
		}
		view, err := s.deps.ListOffers.Handle(ctx, queries.ListOffersQuery{Meta: queryMeta(actor, "", ""), ItemID: commands.CatalogItemID(in.ItemID), Limit: in.Limit, Cursor: in.Cursor, Status: in.Status})
		if err != nil {
			return mapApplicationError(err), true
		}
		return offerList(view), true
	case "listVariants":
		in := input.(*contract.ItemVariantsInput)
		if s.deps.ListVariants == nil {
			return mapApplicationError(appErrors.NotImplemented()), true
		}
		actor, err := s.requireScope(ctx, in.BusinessID)
		if err != nil {
			return mapApplicationError(err), true
		}
		view, err := s.deps.ListVariants.Handle(ctx, queries.ListVariantsQuery{Meta: queryMeta(actor, "", ""), ItemID: commands.CatalogItemID(in.ItemID), Limit: in.Limit, Cursor: in.Cursor, Status: in.Status})
		if err != nil {
			return mapApplicationError(err), true
		}
		return variantList(view), true
	case "listAttributeSchemas":
		in := input.(*contract.AttributeSchemasInput)
		if s.deps.ListAttributeSchemas == nil {
			return mapApplicationError(appErrors.NotImplemented()), true
		}
		actor, err := s.requireScope(ctx, in.BusinessID)
		if err != nil {
			return mapApplicationError(err), true
		}
		version := in.Version
		view, err := s.deps.ListAttributeSchemas.Handle(ctx, queries.ListAttributeSchemasQuery{Meta: queryMeta(actor, "", ""), Limit: in.Limit, Cursor: in.Cursor, Name: in.Name, Version: &version})
		if err != nil {
			return mapApplicationError(err), true
		}
		return attributeSchemaList(view), true
	case "getAttributeSchema":
		in := input.(*contract.SchemaPath)
		if s.deps.GetAttributeSchema == nil {
			return mapApplicationError(appErrors.NotImplemented()), true
		}
		actor, err := s.requireScope(ctx, in.BusinessID)
		if err != nil {
			return mapApplicationError(err), true
		}
		view, err := s.deps.GetAttributeSchema.Handle(ctx, queries.GetAttributeSchemaQuery{Meta: queryMeta(actor, "", ""), SchemaID: commands.AttributeSchemaID(in.SchemaID)})
		if err != nil {
			return mapApplicationError(err), true
		}
		out := &contract.Single[contract.AttributeSchema]{}
		out.Body.Data = attributeSchemaProjection(view)
		return out, true
	default:
		return nil, false
	}
}

func businessPolicyProjection(v commands.BusinessPolicyView) contract.BusinessPolicy {
	return contract.BusinessPolicy{BusinessID: contract.UUID(v.BusinessID), AIMode: v.AIMode, DefaultHumanReview: v.DefaultHumanReview, AllowAutoReply: v.AllowAutoReply, AllowAutoLeadCreation: v.AllowAutoLeadCreation, AllowAutoTransactionDraft: v.AllowAutoTransactionDraft, AllowAutoConfirmation: v.AllowAutoConfirmation, ResourceVersion: string(v.ResourceVersion)}
}
func listPage[T any](items []T, cursor string, more bool) *contract.List[T] {
	out := &contract.List[T]{}
	out.Body.Data = items
	out.Body.Pagination = contract.Page{NextCursor: optionalString(cursor), HasMore: more}
	return out
}
func conversationList(v commands.ListResult[commands.ConversationView]) *contract.List[contract.Conversation] {
	items := make([]contract.Conversation, 0, len(v.Items))
	for _, item := range v.Items {
		items = append(items, conversationProjection(item))
	}
	return listPage(items, v.NextCursor, v.HasMore)
}
func messageList(v commands.ListResult[commands.MessageView]) *contract.List[contract.Message] {
	items := make([]contract.Message, 0, len(v.Items))
	for _, item := range v.Items {
		items = append(items, contract.Message{ID: contract.UUID(item.ID), ConversationID: contract.UUID(item.ConversationID), Direction: item.Direction, Origin: item.Origin, Status: item.Status, Text: item.Text, CreatedAt: item.CreatedAt})
	}
	return listPage(items, v.NextCursor, v.HasMore)
}
func transactionList(v commands.ListResult[commands.TransactionView]) *contract.List[contract.CommercialTransaction] {
	items := make([]contract.CommercialTransaction, 0, len(v.Items))
	for _, item := range v.Items {
		items = append(items, contract.CommercialTransaction{ID: contract.UUID(item.ID), BusinessID: contract.UUID(item.BusinessID), CustomerID: contract.UUID(item.CustomerID), TransactionType: item.TransactionType, State: item.State, ResourceVersion: string(item.ResourceVersion)})
	}
	return listPage(items, v.NextCursor, v.HasMore)
}
func channelConnectionProjection(v commands.ChannelConnectionView) contract.ChannelConnection {
	return contract.ChannelConnection{ID: contract.UUID(v.ID), BusinessID: contract.UUID(v.BusinessID), Provider: v.Provider, Channel: v.Channel, Status: v.Status, ExternalAccountReference: optionalString(v.ExternalAccountReference), ResourceVersion: string(v.ResourceVersion)}
}
func channelConnectionList(v commands.ListResult[commands.ChannelConnectionView]) *contract.List[contract.ChannelConnection] {
	items := make([]contract.ChannelConnection, 0, len(v.Items))
	for _, item := range v.Items {
		items = append(items, channelConnectionProjection(item))
	}
	return listPage(items, v.NextCursor, v.HasMore)
}
func capabilityList(v commands.ListResult[queries.ConnectionCapabilityView]) *contract.List[contract.Capability] {
	items := make([]contract.Capability, 0, len(v.Items))
	for _, item := range v.Items {
		evidence := optionalString(item.EvidenceSource)
		items = append(items, contract.Capability{Name: item.Name, Enabled: item.Enabled, EvidenceSource: evidence})
	}
	return listPage(items, v.NextCursor, v.HasMore)
}
func customerList(v commands.ListResult[commands.CustomerView]) *contract.List[contract.Customer] {
	items := make([]contract.Customer, 0, len(v.Items))
	for _, item := range v.Items {
		items = append(items, customerProjection(item))
	}
	return listPage(items, v.NextCursor, v.HasMore)
}
func catalogProjection(v commands.CatalogView) contract.Catalog {
	return contract.Catalog{ID: contract.UUID(v.ID), BusinessID: contract.UUID(v.BusinessID), Name: v.Name, Status: v.Status, ResourceVersion: string(v.ResourceVersion)}
}
func catalogList(v commands.ListResult[commands.CatalogView]) *contract.List[contract.Catalog] {
	items := make([]contract.Catalog, 0, len(v.Items))
	for _, item := range v.Items {
		items = append(items, catalogProjection(item))
	}
	return listPage(items, v.NextCursor, v.HasMore)
}
func catalogItemProjection(v commands.CatalogItemView) contract.CatalogItem {
	return contract.CatalogItem{ID: contract.UUID(v.ID), BusinessID: contract.UUID(v.BusinessID), CatalogID: contract.UUID(v.CatalogID), ItemType: v.ItemType, Name: v.Name, Status: v.Status, ResourceVersion: string(v.ResourceVersion)}
}
func catalogItemList(v commands.ListResult[commands.CatalogItemView]) *contract.List[contract.CatalogItem] {
	items := make([]contract.CatalogItem, 0, len(v.Items))
	for _, item := range v.Items {
		items = append(items, catalogItemProjection(item))
	}
	return listPage(items, v.NextCursor, v.HasMore)
}
func offerProjection(v commands.OfferView) contract.Offer {
	return contract.Offer{ID: contract.UUID(v.ID), BusinessID: contract.UUID(v.BusinessID), CatalogItemID: contract.UUID(v.CatalogItemID), VariantID: optionalUUID(v.VariantID), Name: v.Name, Status: v.Status, ResourceVersion: string(v.ResourceVersion)}
}
func offerList(v commands.ListResult[commands.OfferView]) *contract.List[contract.Offer] {
	items := make([]contract.Offer, 0, len(v.Items))
	for _, item := range v.Items {
		items = append(items, offerProjection(item))
	}
	return listPage(items, v.NextCursor, v.HasMore)
}
func variantProjection(v commands.VariantView) contract.Variant {
	return contract.Variant{ID: contract.UUID(v.ID), BusinessID: contract.UUID(v.BusinessID), CatalogItemID: contract.UUID(v.CatalogItemID), Name: v.Name, Status: v.Status, ResourceVersion: string(v.ResourceVersion)}
}
func variantList(v commands.ListResult[commands.VariantView]) *contract.List[contract.Variant] {
	items := make([]contract.Variant, 0, len(v.Items))
	for _, item := range v.Items {
		items = append(items, variantProjection(item))
	}
	return listPage(items, v.NextCursor, v.HasMore)
}
func attributeSchemaProjection(v commands.AttributeSchemaView) contract.AttributeSchema {
	return contract.AttributeSchema{ID: contract.UUID(v.ID), BusinessID: contract.UUID(v.BusinessID), Name: v.Name, Version: v.Version}
}
func attributeSchemaList(v commands.ListResult[commands.AttributeSchemaView]) *contract.List[contract.AttributeSchema] {
	items := make([]contract.AttributeSchema, 0, len(v.Items))
	for _, item := range v.Items {
		items = append(items, attributeSchemaProjection(item))
	}
	return listPage(items, v.NextCursor, v.HasMore)
}
func optionalUUID(v commands.VariantID) *contract.UUID {
	if v == "" {
		return nil
	}
	id := contract.UUID(v)
	return &id
}
