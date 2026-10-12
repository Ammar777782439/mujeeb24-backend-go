package ports

import (
	"context"
	"time"
)

type SalesActionJob struct {
	ID, LeaseToken string
	Action         SalesButtonAction
}
type SalesActionJobRepository interface {
	Claim(context.Context, int, time.Time) ([]SalesActionJob, error)
	Finish(context.Context, string, string, string, time.Time) error
}
