package bootstrap

import (
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/persistence/postgres"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
)

func buildSalesWorkflowService(database *postgres.Adapter, outbox ports.OutboxStore) services.CustomerSalesWorkflowService {
	return services.CustomerSalesWorkflowService{Repository: postgres.NewSalesWorkflowRepository(database), Transactions: database, References: postgres.NewConversationReferenceRepository(database), Connections: postgres.NewChannelConnectionRepository(database), Outbound: postgres.NewOutboundMessageRepository(database), Messages: postgres.NewMessageRepository(database), Outbox: outbox, Audit: postgres.NewAuditEventRepository(database)}
}
