package merchantassist

import (
	"context"
	"errors"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/authorization"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/queries"
)

// DiscoveryReader exposes only existing authenticated, tenant-scoped query
// handlers. The model NEVER gets SQL, unfiltered repository access or writes.
type DiscoveryReader struct {
	Guard         ports.BusinessAIExecutionGuard
	Overview      queries.GetDashboardOverviewHandler
	Customers     queries.ListCustomersHandler
	Conversations queries.ListConversationsHandler
	Leads         queries.ListLeadsHandler
	Transactions  queries.ListTransactionsHandler
	Catalogs      queries.ListCatalogsHandler
	Channels      queries.ListChannelConnectionsHandler
	Subscription  queries.GetBusinessSubscriptionHandler
}

type DiscoveryCall struct {
	Name   string `json:"name"`
	Search string `json:"search,omitempty"`
}

type DiscoveryLink struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

type DiscoveryResult struct {
	Tool     string          `json:"tool"`
	Rows     any             `json:"rows"`
	HasMore  bool            `json:"has_more"`
	Coverage string          `json:"coverage"`
	Links    []DiscoveryLink `json:"-"`
}

// Supported read tools expose business-scoped identifiers so the assistant can
// connect customers, conversations and orders, while remaining permission-scoped.
var discoveryPermissions = map[string]string{
	"dashboard_overview": authorization.PermissionDashboardRead,
	"find_customers":     authorization.PermissionCustomersRead,
	"list_conversations": authorization.PermissionConversationsRead,
	"list_leads":         authorization.PermissionSalesRead,
	"list_transactions":  authorization.PermissionSalesRead,
	"list_catalogs":      authorization.PermissionCatalogRead,
	"list_channels":      authorization.PermissionChannelsRead,
	"subscription":       authorization.PermissionBusinessRead,
}

var discoveryNames = []string{
	"dashboard_overview", "find_customers", "list_conversations",
	"list_leads", "list_transactions", "list_catalogs",
	"list_channels", "subscription",
}

func (r *DiscoveryReader) Available(actor commands.ActorContext) []string {
	if r == nil || r.Guard == nil || !authorization.Has(actor, authorization.PermissionMerchantAIUse) {
		return nil
	}
	out := make([]string, 0, len(discoveryNames))
	for _, name := range discoveryNames {
		if authorization.Has(actor, discoveryPermissions[name]) && r.hasHandler(name) {
			out = append(out, name)
		}
	}
	return out
}

func (r *DiscoveryReader) hasHandler(name string) bool {
	switch name {
	case "dashboard_overview":
		return r.Overview != nil
	case "find_customers":
		return r.Customers != nil
	case "list_conversations":
		return r.Conversations != nil
	case "list_leads":
		return r.Leads != nil
	case "list_transactions":
		return r.Transactions != nil
	case "list_catalogs":
		return r.Catalogs != nil
	case "list_channels":
		return r.Channels != nil
	case "subscription":
		return r.Subscription != nil
	default:
		return false
	}
}

func (r *DiscoveryReader) Read(ctx context.Context, actor commands.ActorContext, business string, call DiscoveryCall) (DiscoveryResult, error) {
	var out DiscoveryResult
	if r == nil || r.Guard == nil || business == "" ||
		string(actor.BusinessID) != business || strings.TrimSpace(string(actor.PrincipalID)) == "" ||
		!authorization.Has(actor, authorization.PermissionMerchantAIUse) {
		return out, ErrReadDenied
	}
	permission, known := discoveryPermissions[call.Name]
	if !known || !authorization.Has(actor, permission) {
		return out, ErrReadDenied
	}
	if !r.hasHandler(call.Name) || len(call.Search) > 80 || strings.ContainsAny(call.Search, "\n\r") {
		return out, ErrInvalidReadRequest
	}
	if err := r.Guard.Check(ctx, business); err != nil {
		return out, ErrReadDenied
	}
	meta := commands.QueryMeta{Actor: actor}
	out.Tool = call.Name
	out.Coverage = "bounded_list_not_all_records"
	out.Links = []DiscoveryLink{}
	switch call.Name {
	case "dashboard_overview":
		v, err := r.Overview.Handle(ctx, queries.GetDashboardOverviewQuery{Meta: meta})
		if err != nil {
			return out, errors.Join(ErrReadUnavailable, err)
		}
		out.Rows = map[string]any{
			"open_conversations": v.OpenConversations, "waiting_human": v.WaitingHuman,
			"new_customers": v.NewCustomers, "new_leads": v.NewLeads,
			"transactions_needing_review": v.TransactionsNeedingReview,
		}
		out.Coverage = "dashboard_aggregates"
		out.Links = append(out.Links, DiscoveryLink{Title: "لوحة التحكم", URL: "/dashboard"})
	case "find_customers":
		p, err := r.Customers.Handle(ctx, queries.ListCustomersQuery{Meta: meta, Limit: 12, Search: call.Search})
		if err != nil {
			return out, errors.Join(ErrReadUnavailable, err)
		}
		rows := make([]map[string]any, 0, len(p.Items))
		for _, x := range p.Items {
			if string(x.BusinessID) != business {
				return out, ErrInvalidSourceEvidence
			}
			rows = append(rows, map[string]any{"id": string(x.ID), "name": x.DisplayName, "status": x.Status})
		}
		out.Rows, out.HasMore = rows, p.HasMore
		out.Links = append(out.Links, DiscoveryLink{Title: "العملاء", URL: "/customers"})
	case "list_conversations":
		p, err := r.Conversations.Handle(ctx, queries.ListConversationsQuery{Meta: meta, Limit: 12})
		if err != nil {
			return out, errors.Join(ErrReadUnavailable, err)
		}
		rows := make([]map[string]any, 0, len(p.Items))
		for _, x := range p.Items {
			if string(x.BusinessID) != business {
				return out, ErrInvalidSourceEvidence
			}
			rows = append(rows, map[string]any{
				"id": string(x.ID), "customer_id": string(x.CustomerID), "customer_name": x.CustomerDisplayName, "state": x.State, "ownership": x.Ownership,
				"priority": x.Priority, "last_activity": x.LastActivityAt,
			})
			out.Links = append(out.Links, DiscoveryLink{Title: "فتح محادثة", URL: "/chat?conversation_id=" + string(x.ID)})
		}
		out.Rows, out.HasMore = rows, p.HasMore
	case "list_leads":
		p, err := r.Leads.Handle(ctx, queries.ListLeadsQuery{Meta: meta, Limit: 12})
		if err != nil {
			return out, errors.Join(ErrReadUnavailable, err)
		}
		rows := make([]map[string]any, 0, len(p.Items))
		for _, x := range p.Items {
			if string(x.BusinessID) != business {
				return out, ErrInvalidSourceEvidence
			}
			rows = append(rows, map[string]any{"id": string(x.ID), "customer_id": string(x.CustomerID), "status": x.Status})
		}
		out.Rows, out.HasMore = rows, p.HasMore
		out.Links = append(out.Links, DiscoveryLink{Title: "فرص البيع", URL: "/leads"})
	case "list_transactions":
		p, err := r.Transactions.Handle(ctx, queries.ListTransactionsQuery{Meta: meta, Limit: 12})
		if err != nil {
			return out, errors.Join(ErrReadUnavailable, err)
		}
		rows := make([]map[string]any, 0, len(p.Items))
		for _, x := range p.Items {
			if string(x.BusinessID) != business {
				return out, ErrInvalidSourceEvidence
			}
			rows = append(rows, map[string]any{
				"id": string(x.ID), "customer_id": string(x.CustomerID), "state": x.State,
				"type": x.TransactionType, "created_at": x.CreatedAt,
				"amount": x.TotalAmount, "currency": x.Currency,
			})
			out.Links = append(out.Links, DiscoveryLink{Title: "فتح طلب", URL: "/transactions?transaction_id=" + string(x.ID)})
		}
		out.Rows, out.HasMore = rows, p.HasMore
	case "list_catalogs":
		p, err := r.Catalogs.Handle(ctx, queries.ListCatalogsQuery{Meta: meta, Limit: 12})
		if err != nil {
			return out, errors.Join(ErrReadUnavailable, err)
		}
		rows := make([]map[string]any, 0, len(p.Items))
		for _, x := range p.Items {
			if string(x.BusinessID) != business {
				return out, ErrInvalidSourceEvidence
			}
			rows = append(rows, map[string]any{"id": string(x.ID), "name": x.Name, "status": x.Status})
		}
		out.Rows, out.HasMore = rows, p.HasMore
		out.Links = append(out.Links, DiscoveryLink{Title: "الكتالوج", URL: "/catalog"})
	case "list_channels":
		p, err := r.Channels.Handle(ctx, queries.ListChannelConnectionsQuery{Meta: meta, Limit: 12})
		if err != nil {
			return out, errors.Join(ErrReadUnavailable, err)
		}
		rows := make([]map[string]any, 0, len(p.Items))
		for _, x := range p.Items {
			if string(x.BusinessID) != business {
				return out, ErrInvalidSourceEvidence
			}
			rows = append(rows, map[string]any{"id": string(x.ID), "channel": x.Channel, "status": x.Status})
		}
		out.Rows, out.HasMore = rows, p.HasMore
		out.Links = append(out.Links, DiscoveryLink{Title: "القنوات", URL: "/channels"})
	case "subscription":
		v, err := r.Subscription.Handle(ctx, queries.GetBusinessSubscriptionQuery{Meta: meta})
		if err != nil {
			return out, errors.Join(ErrReadUnavailable, err)
		}
		if v.BusinessID != business {
			return out, ErrInvalidSourceEvidence
		}
		out.Rows = map[string]any{
			"has_subscription": v.HasSubscription, "plan": v.PlanName,
			"status": v.Status, "ai_reply_limit": v.AIReplyLimit,
			"ai_replies_used": v.AIRepliesUsed, "ai_replies_remaining": v.AIRepliesRemaining,
			"active_channels": v.ActiveChannels, "active_seats": v.ActiveSeats,
		}
		out.Coverage = "subscription_snapshot"
	default:
		return out, ErrInvalidReadRequest
	}
	if out.HasMore {
		out.Coverage = "partial_has_more_pages"
	}
	return out, nil
}
