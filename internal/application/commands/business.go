package commands

type UpdateBusinessProfileCommand struct {
	Meta            CommandMeta
	Name            *string
	VerticalType    *string
	Timezone        *string
	DefaultCurrency *string
	Locale          *string
}
type UpdateBusinessPolicyCommand struct {
	Meta                      CommandMeta
	AIMode                    *string
	DefaultHumanReview        *bool
	AllowAutoReply            *bool
	AllowAutoLeadCreation     *bool
	AllowAutoTransactionDraft *bool
	AllowAutoConfirmation     *bool
}
type UpdateBusinessProfileResult struct {
	MutationResult
	Business BusinessView
}
type UpdateBusinessPolicyResult struct {
	MutationResult
	Policy BusinessPolicyView
}
type UpdateBusinessProfileHandler = CommandHandler[UpdateBusinessProfileCommand, UpdateBusinessProfileResult]
type UpdateBusinessPolicyHandler = CommandHandler[UpdateBusinessPolicyCommand, UpdateBusinessPolicyResult]
