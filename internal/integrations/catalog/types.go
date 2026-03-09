package catalog

type IntegrationSummary struct {
	Name                string `json:"name"`
	Source              string `json:"source"`
	Version             string `json:"version,omitempty"`
	Publisher           string `json:"publisher,omitempty"`
	SupportsTenantScope bool   `json:"supports_tenant_scope"`
	SupportsGlobalScope bool   `json:"supports_global_scope"`
	HasInbound          bool   `json:"has_inbound"`
	OperationCount      int    `json:"operation_count"`
	SchemaCount         int    `json:"schema_count"`
}

type IntegrationDetail struct {
	Name                string             `json:"name"`
	Source              string             `json:"source"`
	Version             string             `json:"version,omitempty"`
	Publisher           string             `json:"publisher,omitempty"`
	SupportsTenantScope bool               `json:"supports_tenant_scope"`
	SupportsGlobalScope bool               `json:"supports_global_scope"`
	Config              ConfigCatalog      `json:"config"`
	Inbound             *InboundCatalog    `json:"inbound,omitempty"`
	Operations          []OperationCatalog `json:"operations"`
	Schemas             []SchemaCatalog    `json:"schemas"`
}

type ConfigCatalog struct {
	Fields []ConfigFieldCatalog `json:"fields"`
}

type ConfigFieldCatalog struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
	Secret   bool   `json:"secret"`
}

type InboundCatalog struct {
	RouteKeyStrategy string   `json:"route_key_strategy"`
	EventTypes       []string `json:"event_types"`
}

type OperationCatalog struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type SchemaCatalog struct {
	EventType string `json:"event_type"`
	Version   int    `json:"version"`
}
