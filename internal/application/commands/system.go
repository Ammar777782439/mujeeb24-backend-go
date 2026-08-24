package commands

type GetLivenessQuery struct{}
type GetReadinessQuery struct{}
type GetMetricsQuery struct{}
type HealthView struct {
	Status string
	Checks map[string]string
}
type MetricsView struct {
	Body string
}

type IngestWebhookCommand struct {
	RouteKey   string
	Signature  string
	Timestamp  string
	RequestID  string
	RawPayload []byte
}
type WebhookAcceptedResult struct {
	Accepted  bool
	RequestID string
}
type IngestSocialAPIWebhookHandler = CommandHandler[IngestWebhookCommand, WebhookAcceptedResult]
type IngestChatwootWebhookHandler = CommandHandler[IngestWebhookCommand, WebhookAcceptedResult]
