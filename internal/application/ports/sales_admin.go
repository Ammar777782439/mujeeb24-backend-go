package ports

import (
	"context"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"time"
)

type SalesWorkflowPage struct {
	Items      []SalesWorkflowRecord
	NextCursor string
	HasMore    bool
}
type SalesWorkflowReview struct {
	BusinessID, WorkflowID, ActorReference string
	ExpectedVersion                        int64
	Approved                               bool
	Now                                    time.Time
}
type SalesWorkflowAdminRepository interface {
	GetWorkflow(context.Context, string, string) (SalesWorkflowRecord, error)
	ListWorkflows(context.Context, string, string, string, int, string) (SalesWorkflowPage, error)
	ReviewWorkflow(context.Context, SalesWorkflowReview) (SalesWorkflowResult, error)
	PutRequirement(context.Context, string, SalesRequirement) error
	ListRequirements(context.Context, string) ([]SalesRequirement, error)
}
type SalesAdminPort interface {
	Get(context.Context, commands.ActorContext, string) (SalesWorkflowRecord, error)
	List(context.Context, commands.ActorContext, string, string, int, string) (SalesWorkflowPage, error)
	Review(context.Context, commands.ActorContext, string, int64, bool) (SalesWorkflowResult, error)
	PutRequirement(context.Context, commands.ActorContext, SalesRequirement) error
	Requirements(context.Context, commands.ActorContext) ([]SalesRequirement, error)
}
