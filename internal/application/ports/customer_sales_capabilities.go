package ports

// CustomerSalesCapability names the bounded operations the B2C runtime supports.
// The model chooses a capability; it cannot supply tenant IDs or authorization.
type CustomerSalesCapability string

const (
	CustomerSalesCapabilityRespondWithoutAction CustomerSalesCapability = "respond_without_action"
	CustomerSalesCapabilityAsk                  CustomerSalesCapability = "ask_customer"
	CustomerSalesCapabilityHandoff              CustomerSalesCapability = "handoff_to_human"
	CustomerSalesCapabilityFullCatalog          CustomerSalesCapability = "evaluate_full_catalog"
)

// CustomerSalesCapabilities exposes implemented operations, independent of sector.
// Draft creation is absent until an actual B2C executor is registered.
func CustomerSalesCapabilities(fullCatalogAvailable bool) []CustomerSalesCapability {
	capabilities := make([]CustomerSalesCapability, 0, len(customerSalesCapabilityRegistry))
	for _, entry := range customerSalesCapabilityRegistry {
		if entry.Definition.Terminal || (entry.Definition.RequiresFullCatalogRuntime && fullCatalogAvailable) {
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
	Selected     []SelectedReference     `json:"selected,omitempty"`
}

// CustomerSalesCapabilityDefinition describes an executable operation, not intent.
// All currently supported terminal operations use the customer-message executor.
type CustomerSalesCapabilityDefinition struct {
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
}

func CustomerSalesCapabilityDefinitionFor(capability CustomerSalesCapability) (CustomerSalesCapabilityDefinition, bool) {
	for _, entry := range customerSalesCapabilityRegistry {
		if entry.Name == capability {
			return entry.Definition, true
		}
	}
	return CustomerSalesCapabilityDefinition{}, false
}
