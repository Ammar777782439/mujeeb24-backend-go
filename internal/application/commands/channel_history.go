package commands

type PreviewChannelHistoryQuery struct {
	Meta         QueryMeta
	ConnectionID ConnectionID
}
type PurgeChannelHistoryCommand struct {
	Meta            CommandMeta
	ConnectionID    ConnectionID
	Confirmation    string
	ExpectedVersion ResourceVersion
}
type ChannelHistoryResult struct {
	BusinessID    BusinessID
	ConnectionID  ConnectionID
	Conversations int64
	Messages      int64
	PurgedAt      string
}
type PreviewChannelHistoryHandler = QueryHandler[PreviewChannelHistoryQuery, ChannelHistoryResult]
type PurgeChannelHistoryHandler = CommandHandler[PurgeChannelHistoryCommand, ChannelHistoryResult]
