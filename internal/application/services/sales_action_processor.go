package services

import (
	"context"
	"fmt"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"log"
	"strings"
	"time"
)

type SalesActionProcessor struct {
	Jobs  ports.SalesActionJobRepository
	Sales CustomerSalesWorkflowService
}

func (p SalesActionProcessor) Process(ctx context.Context, limit int) (int, error) {
	if p.Jobs == nil {
		return 0, fmt.Errorf("sales action jobs are not configured")
	}
	jobs, err := p.Jobs.Claim(ctx, limit, time.Now().UTC())
	if err != nil {
		return 0, err
	}
	for i, j := range jobs {
		code := ""
		var err error
		if strings.HasPrefix(j.Action.Payload, "m24.sales.start:") {
			_, err = p.Sales.HandleStartButton(ctx, j.Action)
		} else {
			_, err = p.Sales.HandleButton(ctx, j.Action)
		}
		if err != nil {
			code = "sales_action_failed"
			log.Printf("[SalesAction] FAILED job=%s business=%s", j.ID, j.Action.BusinessID)
		}
		if err := p.Jobs.Finish(ctx, j.ID, j.LeaseToken, code, time.Now().UTC()); err != nil {
			return i, err
		}
	}
	return len(jobs), nil
}
