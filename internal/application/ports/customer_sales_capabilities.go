package ports

// CustomerSalesCapability names the bounded operations the B2C runtime supports.
// The model chooses a capability; it cannot supply tenant IDs or authorization.
type CustomerSalesCapability string

const (
	CustomerSalesCapabilityReply       CustomerSalesCapability = "reply_customer"
	CustomerSalesCapabilityAsk         CustomerSalesCapability = "ask_customer"
	CustomerSalesCapabilityHandoff     CustomerSalesCapability = "handoff_to_human"
	CustomerSalesCapabilityFullCatalog CustomerSalesCapability = "evaluate_full_catalog"
)

// CustomerSalesCapabilities exposes executable operations, independent of sector.
// Draft creation is deliberately absent until the B2C runtime has a draft executor.
func CustomerSalesCapabilities(fullCatalogAvailable bool) []CustomerSalesCapability {
	capabilities := []CustomerSalesCapability{CustomerSalesCapabilityReply, CustomerSalesCapabilityAsk, CustomerSalesCapabilityHandoff}
	if fullCatalogAvailable {
		capabilities = append(capabilities, CustomerSalesCapabilityFullCatalog)
	}
	return capabilities
}
