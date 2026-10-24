package handlers

import (
	"context"
	"encoding/json"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/contract"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"strconv"
	"strings"
)

func (s *Server) dispatchSalesWorkflow(ctx context.Context, op string, input any) (any, bool) {
	business := ""
	switch op {
	case "listSalesWorkflows":
		business = string(input.(*contract.SalesWorkflowListInput).BusinessID)
	case "getSalesWorkflow":
		business = string(input.(*contract.SalesWorkflowPath).BusinessID)
	case "reviewSalesWorkflow":
		business = string(input.(*contract.SalesWorkflowReviewInput).BusinessID)
	case "listSalesRequirements":
		business = string(input.(*contract.BusinessPath).BusinessID)
	case "createSalesRequirement":
		business = string(input.(*contract.SalesRequirementCreateInput).BusinessID)
	default:
		return nil, false
	}
	actor, err := s.requireScope(ctx, contract.UUID(business))
	if err != nil {
		return mapApplicationError(err), true
	}
	if s.deps.SalesAdmin == nil {
		return mapApplicationError(appErrors.NotImplemented()), true
	}
	switch op {
	case "listSalesWorkflows":
		in := input.(*contract.SalesWorkflowListInput)
		page, err := s.deps.SalesAdmin.List(ctx, actor, in.CustomerID, in.State, in.Limit, in.Cursor)
		if err != nil {
			return mapApplicationError(err), true
		}
		items := []contract.SalesWorkflow{}
		for _, w := range page.Items {
			items = append(items, salesWorkflowProjection(w))
		}
		return listPage(items, page.NextCursor, page.HasMore), true
	case "getSalesWorkflow":
		in := input.(*contract.SalesWorkflowPath)
		w, err := s.deps.SalesAdmin.Get(ctx, actor, string(in.WorkflowID))
		if err != nil {
			return mapApplicationError(err), true
		}
		out := &contract.Single[contract.SalesWorkflow]{}
		out.Body.Data = salesWorkflowProjection(w)
		return out, true
	case "reviewSalesWorkflow":
		in := input.(*contract.SalesWorkflowReviewInput)
		version, err := strconv.ParseInt(strings.Trim(in.IfMatch, "\""), 10, 64)
		if err != nil || version <= 0 {
			return mapApplicationError(appErrors.New(appErrors.CodeValidation, "If-Match positive version required")), true
		}
		result, err := s.deps.SalesAdmin.Review(ctx, actor, string(in.WorkflowID), version, in.Body.Approved)
		if err != nil {
			return mapApplicationError(err), true
		}
		out := &contract.Single[contract.SalesWorkflow]{}
		out.Body.Data = salesWorkflowProjection(result.Workflow)
		return out, true
	case "listSalesRequirements":
		reqs, err := s.deps.SalesAdmin.Requirements(ctx, actor)
		if err != nil {
			return mapApplicationError(err), true
		}
		items := []contract.SalesRequirement{}
		for _, req := range reqs {
			items = append(items, salesRequirementProjection(req))
		}
		return listPage(items, "", false), true
	case "createSalesRequirement":
		in := input.(*contract.SalesRequirementCreateInput)
		req := ports.SalesRequirement{TransactionType: in.Body.TransactionType, SchemaVersion: in.Body.SchemaVersion}
		for _, d := range in.Body.Definitions {
			req.Definitions = append(req.Definitions, ports.SalesRequirementDefinition{Key: d.Key, Label: d.Label, DataType: d.DataType, Required: d.Required, Rules: d.Rules})
		}
		if err := s.deps.SalesAdmin.PutRequirement(ctx, actor, req); err != nil {
			return mapApplicationError(err), true
		}
		out := &contract.Single[contract.SalesRequirement]{}
		out.Body.Data = salesRequirementProjection(req)
		return out, true
	}
	return nil, false
}
func salesWorkflowProjection(w ports.SalesWorkflowRecord) contract.SalesWorkflow {
	data, _ := json.Marshal(w.Proposal)
	var proposal map[string]any
	_ = json.Unmarshal(data, &proposal)
	out := contract.SalesWorkflow{ID: contract.UUID(w.ID), CustomerID: contract.UUID(w.CustomerID), State: w.State, PendingAction: w.PendingAction, Proposal: proposal, ResourceVersion: strconv.FormatInt(w.ResourceVersion, 10)}
	if w.ConversationID != "" {
		id := contract.UUID(w.ConversationID)
		out.ConversationID = &id
	}
	if w.LeadID != nil {
		id := contract.UUID(*w.LeadID)
		out.LeadID = &id
	}
	if w.TransactionID != nil {
		id := contract.UUID(*w.TransactionID)
		out.TransactionID = &id
	}
	return out
}
func salesRequirementProjection(req ports.SalesRequirement) contract.SalesRequirement {
	out := contract.SalesRequirement{TransactionType: req.TransactionType, SchemaVersion: req.SchemaVersion, Definitions: []contract.SalesRequirementDefinition{}}
	for _, d := range req.Definitions {
		out.Definitions = append(out.Definitions, contract.SalesRequirementDefinition{Key: d.Key, Label: d.Label, DataType: d.DataType, Required: d.Required, Rules: d.Rules})
	}
	return out
}
