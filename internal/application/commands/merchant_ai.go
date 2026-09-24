package commands

type MerchantAIChatCommand struct {
	Meta      CommandMeta
	SessionID *string
	Message   string
}

type MerchantAIChatResult struct {
	Message   string
	Action    string
	SessionID string
}

type MerchantAIChatHandler = CommandHandler[MerchantAIChatCommand, MerchantAIChatResult]
