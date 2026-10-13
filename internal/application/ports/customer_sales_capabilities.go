package ports

// CustomerSalesCapability names the bounded operations the B2C runtime supports.
// The model chooses a capability; it cannot supply tenant IDs or authorization.
type CustomerSalesCapability string

const (
	CustomerSalesCapabilityRespondWithoutAction CustomerSalesCapability = "respond_without_action"
	CustomerSalesCapabilityAsk                  CustomerSalesCapability = "ask_customer"
	CustomerSalesCapabilityHandoff              CustomerSalesCapability = "handoff_to_human"
	CustomerSalesCapabilityFullCatalog          CustomerSalesCapability = "evaluate_full_catalog"
	CustomerSalesCapabilityRecordLead           CustomerSalesCapability = "record_lead"
	CustomerSalesCapabilityCreateDraft          CustomerSalesCapability = "create_transaction_draft"
	CustomerSalesCapabilityUpdateDraft          CustomerSalesCapability = "update_transaction_draft"
)

// CustomerSalesCapabilities exposes implemented operations, independent of sector.
// Draft creation is absent until an actual B2C executor is registered.
func CustomerSalesCapabilities(fullCatalogAvailable bool, salesAvailable ...bool) []CustomerSalesCapability {
	capabilities := make([]CustomerSalesCapability, 0, len(customerSalesCapabilityRegistry))
	for _, entry := range customerSalesCapabilityRegistry {
		if (entry.Definition.Terminal && (!entry.Definition.RequiresSalesRuntime || (len(salesAvailable) > 0 && salesAvailable[0]))) || (entry.Definition.RequiresFullCatalogRuntime && fullCatalogAvailable) {
			capabilities = append(capabilities, entry.Name)
		}
	}
	return capabilities
}

// CapabilityDecision carries the model's choice without legacy status/action routing.
// Tenant identity, policy and execution results are never supplied by the model.
type CapabilityDecision struct {
	Capability   CustomerSalesCapability `json:"capability"`
	ResponseText string                  `json:"response_text,omitempty"`
	// Presentation is chosen by Gemini only for evaluate_full_catalog.
	// "hierarchy" displays every active catalog item without reduction.
	Presentation string                 `json:"presentation,omitempty"`
	Selected     []SelectedReference    `json:"selected,omitempty"`
	Sales        *SalesWorkflowProposal `json:"sales,omitempty"`
}

// CustomerSalesCapabilityDefinition describes an executable operation, not intent.
// All currently supported terminal operations use the customer-message executor.
type CustomerSalesCapabilityDefinition struct {
	Executor                   string
	RequiresSalesRuntime       bool
	Terminal                   bool
	SendsCustomerMessage       bool
	RequiresHuman              bool
	ConversationState          string
	ConversationOwnership      string
	RequiresFullCatalogRuntime bool
}

// The registry is the single runtime definition for validation, policy requirements,
// executor conversation effects and local capability availability. Non-terminal
// capabilities are never exposed just because another non-terminal capability is ready.
var customerSalesCapabilityRegistry = [...]struct {
	Name       CustomerSalesCapability
	Definition CustomerSalesCapabilityDefinition
}{
	{CustomerSalesCapabilityRespondWithoutAction, CustomerSalesCapabilityDefinition{Terminal: true, SendsCustomerMessage: true, ConversationState: "waiting_customer", ConversationOwnership: "ai"}},
	{CustomerSalesCapabilityAsk, CustomerSalesCapabilityDefinition{Terminal: true, SendsCustomerMessage: true, ConversationState: "waiting_customer", ConversationOwnership: "ai"}},
	{CustomerSalesCapabilityHandoff, CustomerSalesCapabilityDefinition{Terminal: true, SendsCustomerMessage: true, RequiresHuman: true, ConversationState: "waiting_human"}},
	{CustomerSalesCapabilityFullCatalog, CustomerSalesCapabilityDefinition{RequiresFullCatalogRuntime: true}},
	{CustomerSalesCapabilityRecordLead, CustomerSalesCapabilityDefinition{Terminal: true, Executor: "sales", RequiresSalesRuntime: true}},
	{CustomerSalesCapabilityCreateDraft, CustomerSalesCapabilityDefinition{Terminal: true, Executor: "sales", RequiresSalesRuntime: true}},
	{CustomerSalesCapabilityUpdateDraft, CustomerSalesCapabilityDefinition{Terminal: true, Executor: "sales", RequiresSalesRuntime: true}},
}

func CustomerSalesCapabilityDefinitionFor(capability CustomerSalesCapability) (CustomerSalesCapabilityDefinition, bool) {
	for _, entry := range customerSalesCapabilityRegistry {
		if entry.Name == capability {
			return entry.Definition, true
		}
	}
	return CustomerSalesCapabilityDefinition{}, false
}
