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
	Guard                 ports.BusinessAIExecutionGuard
	Overview              queries.GetDashboardOverviewHandler
	Customers             queries.ListCustomersHandler
	GetCustomer           queries.GetCustomerHandler
	CustomerConversations queries.ListCustomerConversationsHandler
	CustomerTransactions  queries.ListCustomerTransactionsHandler
	Conversations         queries.ListConversationsHandler
	Leads                 queries.ListLeadsHandler
	Transactions          queries.ListTransactionsHandler
	Catalogs              queries.ListCatalogsHandler
	Channels              queries.ListChannelConnectionsHandler
	Subscription          queries.GetBusinessSubscriptionHandler
}

type DiscoveryCall struct {
	Name     string `json:"name"`
	Search   string `json:"search,omitempty"`
	RecordID string `json:"record_id,omitempty"`
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
	"platform_guide": authorization.PermissionMerchantAIUse,
	"dashboard_overview": authorization.PermissionDashboardRead,
	"find_customers":     authorization.PermissionCustomersRead,
	"customer_activity":  authorization.PermissionCustomersRead,
	"list_conversations": authorization.PermissionConversationsRead,
	"list_leads":         authorization.PermissionSalesRead,
	"list_transactions":  authorization.PermissionSalesRead,
	"list_catalogs":      authorization.PermissionCatalogRead,
	"list_channels":      authorization.PermissionChannelsRead,
	"subscription":       authorization.PermissionBusinessRead,
}

var discoveryNames = []string{
	"platform_guide", "dashboard_overview", "find_customers", "list_conversations",
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
	case "platform_guide":
		return true
	case "dashboard_overview":
		return r.Overview != nil
	case "find_customers":
		return r.Customers != nil
	case "customer_activity":
		return r.GetCustomer != nil
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
	if !r.hasHandler(call.Name) || len(call.Search) > 80 || len(call.RecordID) > 128 || strings.ContainsAny(call.Search, "\n\r") {
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
	case "platform_guide":
		areas := make([]map[string]any, 0, 7)
		addArea := func(permission, name, url, detail string) {
			if !authorization.Has(actor, permission) { return }
			areas = append(areas, map[string]any{"name": name, "url": url, "description": detail})
			out.Links = append(out.Links, DiscoveryLink{Title: name, URL: url})
		}
		addArea(authorization.PermissionDashboardRead, "لوحة التحكم", "/dashboard", "ملخص المتجر ومؤشرات المتابعة")
		addArea(authorization.PermissionConversationsRead, "المحادثات", "/chat", "قراءة المحادثات وحالاتها وما ينتظر موظفًا")
		addArea(authorization.PermissionCustomersRead, "العملاء", "/customers", "استعراض بيانات العملاء والبحث عنهم")
		addArea(authorization.PermissionSalesRead, "فرص البيع", "/leads", "متابعة فرص البيع وحالاتها")
		addArea(authorization.PermissionSalesRead, "الطلبات", "/transactions", "مراجعة الطلبات والمعاملات وحالاتها")
		addArea(authorization.PermissionCatalogRead, "الكتالوج", "/catalog", "مساعد الكتالوج الذكي مسار مستقل يقترح إضافة وتعديل المنتجات؛ التنفيذ بعد التحقق والصلاحيات والتأكيد. اسأل مجيب هنا للقراءة والشرح فقط")
		addArea(authorization.PermissionChannelsRead, "القنوات", "/channels", "متابعة حالة ربط قنوات التواصل")
		out.Rows = map[string]any{"assistant_role": "read_only_business_assistance", "catalog_authoring_role": "separate_reviewed_proposal_workflow", "areas": areas}
		out.Coverage = "verified_navigation_with_actor_permissions"
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
		scan, err := scanAllPages(ctx, func(cursor string) (commands.ListResult[commands.CustomerView], error) {
			return r.Customers.Handle(ctx, queries.ListCustomersQuery{Meta: meta, Limit: assistScanPageSize, Cursor: cursor, Search: call.Search})
		}, func(x commands.CustomerView) (map[string]any, string, string, *DiscoveryLink, error) {
			if string(x.BusinessID) != business {
				return nil, "", "", nil, ErrInvalidSourceEvidence
			}
			return map[string]any{"id": string(x.ID), "name": x.DisplayName, "status": x.Status}, string(x.ID), x.Status, &DiscoveryLink{Title: "العملاء", URL: "/customers"}, nil
		})
		if err != nil {
			return out, err
		}
		out.Rows, out.Links = scan.Evidence(), scan.Links
		out.Coverage = "all_pages_scanned_non_snapshot"
	case "customer_activity":
		if call.RecordID == "" {
			return out, ErrInvalidReadRequest
		}
		customer, err := r.GetCustomer.Handle(ctx, queries.GetCustomerQuery{Meta: meta, CustomerID: commands.CustomerID(call.RecordID)})
		if err != nil {
			return out, errors.Join(ErrReadUnavailable, err)
		}
		if string(customer.BusinessID) != business || string(customer.ID) != call.RecordID {
			return out, ErrInvalidSourceEvidence
		}
		related := map[string]any{"id": string(customer.ID), "name": customer.DisplayName, "status": customer.Status}
		if authorization.Has(actor, authorization.PermissionConversationsRead) && r.CustomerConversations != nil {
			scan, err := scanAllPages(ctx, func(cursor string) (commands.ListResult[commands.ConversationView], error) {
				return r.CustomerConversations.Handle(ctx, queries.ListCustomerConversationsQuery{Meta: meta, CustomerID: customer.ID, Limit: assistScanPageSize, Cursor: cursor})
			}, func(item commands.ConversationView) (map[string]any, string, string, *DiscoveryLink, error) {
				if string(item.BusinessID) != business || item.CustomerID != customer.ID {
					return nil, "", "", nil, ErrInvalidSourceEvidence
				}
				return map[string]any{
					"id": string(item.ID), "state": item.State, "ownership": item.Ownership,
					"priority": item.Priority, "last_activity": item.LastActivityAt,
				}, string(item.ID), item.State, &DiscoveryLink{Title: "فتح محادثة", URL: "/chat?conversation_id=" + string(item.ID)}, nil
			})
			if err != nil {
				return out, err
			}
			related["conversations"] = scan.Evidence()
			out.Links = append(out.Links, scan.Links...)
		}
		if authorization.Has(actor, authorization.PermissionSalesRead) && r.CustomerTransactions != nil {
			scan, err := scanAllPages(ctx, func(cursor string) (commands.ListResult[commands.TransactionView], error) {
				return r.CustomerTransactions.Handle(ctx, queries.ListCustomerTransactionsQuery{Meta: meta, CustomerID: customer.ID, Limit: assistScanPageSize, Cursor: cursor})
			}, func(item commands.TransactionView) (map[string]any, string, string, *DiscoveryLink, error) {
				if string(item.BusinessID) != business || item.CustomerID != customer.ID {
					return nil, "", "", nil, ErrInvalidSourceEvidence
				}
				return map[string]any{
					"id": string(item.ID), "state": item.State, "type": item.TransactionType,
					"created_at": item.CreatedAt, "amount": item.TotalAmount, "currency": item.Currency,
				}, string(item.ID), item.State, &DiscoveryLink{Title: "فتح طلب", URL: "/transactions?transaction_id=" + string(item.ID)}, nil
			})
			if err != nil {
				return out, err
			}
			related["transactions"] = scan.Evidence()
			out.Links = append(out.Links, scan.Links...)
		}
		out.Rows = related
		out.Coverage = "all_authorized_related_pages_scanned_non_snapshot"
		out.Links = append(out.Links, DiscoveryLink{Title: "العملاء", URL: "/customers"})
	case "list_conversations":
		scan, err := scanAllPages(ctx, func(cursor string) (commands.ListResult[commands.ConversationView], error) {
			return r.Conversations.Handle(ctx, queries.ListConversationsQuery{Meta: meta, Limit: assistScanPageSize, Cursor: cursor})
		}, func(x commands.ConversationView) (map[string]any, string, string, *DiscoveryLink, error) {
			if string(x.BusinessID) != business {
				return nil, "", "", nil, ErrInvalidSourceEvidence
			}
			return map[string]any{"id": string(x.ID), "customer_id": string(x.CustomerID), "customer_name": x.CustomerDisplayName, "state": x.State, "ownership": x.Ownership, "priority": x.Priority, "last_activity": x.LastActivityAt}, string(x.ID), x.State, &DiscoveryLink{Title: "فتح محادثة", URL: "/chat?conversation_id=" + string(x.ID)}, nil
		})
		if err != nil {
			return out, err
		}
		out.Rows, out.Links = scan.Evidence(), scan.Links
		out.Coverage = "all_pages_scanned_non_snapshot"
	case "list_leads":
		scan, err := scanAllPages(ctx, func(cursor string) (commands.ListResult[commands.LeadView], error) {
			return r.Leads.Handle(ctx, queries.ListLeadsQuery{Meta: meta, Limit: assistScanPageSize, Cursor: cursor})
		}, func(x commands.LeadView) (map[string]any, string, string, *DiscoveryLink, error) {
			if string(x.BusinessID) != business {
				return nil, "", "", nil, ErrInvalidSourceEvidence
			}
			return map[string]any{"id": string(x.ID), "customer_id": string(x.CustomerID), "status": x.Status}, string(x.ID), x.Status, &DiscoveryLink{Title: "فرص البيع", URL: "/leads"}, nil
		})
		if err != nil {
			return out, err
		}
		out.Rows, out.Links = scan.Evidence(), scan.Links
		out.Coverage = "all_pages_scanned_non_snapshot"
	case "list_transactions":
		scan, err := scanAllPages(ctx, func(cursor string) (commands.ListResult[commands.TransactionView], error) {
			return r.Transactions.Handle(ctx, queries.ListTransactionsQuery{Meta: meta, Limit: assistScanPageSize, Cursor: cursor})
		}, func(x commands.TransactionView) (map[string]any, string, string, *DiscoveryLink, error) {
			if string(x.BusinessID) != business {
				return nil, "", "", nil, ErrInvalidSourceEvidence
			}
			return map[string]any{"id": string(x.ID), "customer_id": string(x.CustomerID), "state": x.State, "type": x.TransactionType, "created_at": x.CreatedAt, "amount": x.TotalAmount, "currency": x.Currency}, string(x.ID), x.State, &DiscoveryLink{Title: "فتح طلب", URL: "/transactions?transaction_id=" + string(x.ID)}, nil
		})
		if err != nil {
			return out, err
		}
		out.Rows, out.Links = scan.Evidence(), scan.Links
		out.Coverage = "all_pages_scanned_non_snapshot"
	case "list_catalogs":
		scan, err := scanAllPages(ctx, func(cursor string) (commands.ListResult[commands.CatalogView], error) {
			return r.Catalogs.Handle(ctx, queries.ListCatalogsQuery{Meta: meta, Limit: assistScanPageSize, Cursor: cursor})
		}, func(x commands.CatalogView) (map[string]any, string, string, *DiscoveryLink, error) {
			if string(x.BusinessID) != business {
				return nil, "", "", nil, ErrInvalidSourceEvidence
			}
			return map[string]any{"id": string(x.ID), "name": x.Name, "status": x.Status}, string(x.ID), x.Status, &DiscoveryLink{Title: "الكتالوج", URL: "/catalog"}, nil
		})
		if err != nil {
			return out, err
		}
		out.Rows, out.Links = scan.Evidence(), scan.Links
		out.Coverage = "all_pages_scanned_non_snapshot"
	case "list_channels":
		scan, err := scanAllPages(ctx, func(cursor string) (commands.ListResult[commands.ChannelConnectionView], error) {
			return r.Channels.Handle(ctx, queries.ListChannelConnectionsQuery{Meta: meta, Limit: assistScanPageSize, Cursor: cursor})
		}, func(x commands.ChannelConnectionView) (map[string]any, string, string, *DiscoveryLink, error) {
			if string(x.BusinessID) != business {
				return nil, "", "", nil, ErrInvalidSourceEvidence
			}
			return map[string]any{"id": string(x.ID), "channel": x.Channel, "status": x.Status}, string(x.ID), x.Status, &DiscoveryLink{Title: "القنوات", URL: "/channels"}, nil
		})
		if err != nil {
			return out, err
		}
		out.Rows, out.Links = scan.Evidence(), scan.Links
		out.Coverage = "all_pages_scanned_non_snapshot"
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
