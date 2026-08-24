package commands

type AuthenticatePrincipalCommand struct {
	RequestID     string
	CorrelationID string
	Email         string
	Password      string
}
type RotateRefreshSessionCommand struct {
	RequestID        string
	CorrelationID    string
	SessionReference string
	RefreshToken     string
}
type RevokeRefreshSessionCommand struct {
	Meta             CommandMeta
	SessionReference string
}
type AuthResult struct {
	AccessToken string
	ExpiresAt   string
	Principal   PrincipalAuthView
}
type PrincipalAuthView struct {
	ID          PrincipalID
	DisplayName string
	Email       string
}
type AuthenticatePrincipalHandler = CommandHandler[AuthenticatePrincipalCommand, AuthResult]
type RotateRefreshSessionHandler = CommandHandler[RotateRefreshSessionCommand, AuthResult]
type RevokeRefreshSessionHandler = CommandHandler[RevokeRefreshSessionCommand, EmptyResult]

type RequestHumanReviewCommand struct {
	Meta       CommandMeta
	DecisionID AIDecisionID
	Reason     string
}
type AIDecisionResult struct {
	MutationResult
	Decision AIDecisionView
}
type RequestHumanReviewHandler = CommandHandler[RequestHumanReviewCommand, AIDecisionResult]
