package stripe

import (
	"context"
	"fmt"

	"groot/internal/connectors/provider"
	"groot/internal/connectors/registry"
)

type Provider struct{}

func init() {
	registry.RegisterProvider(Provider{})
}

func (Provider) Spec() provider.ProviderSpec {
	return provider.ProviderSpec{
		Name:                ConnectorName,
		SupportsTenantScope: true,
		SupportsGlobalScope: false,
		Config: provider.ConfigSpec{
			Fields: []provider.ConfigField{
				{Name: "stripe_account_id", Required: true},
				{Name: "webhook_secret", Required: true, Secret: true},
			},
		},
		Inbound: &provider.InboundSpec{
			RouteKeyStrategy: "stripe_account",
			EventTypes:       []string{"stripe.payment_intent.succeeded.v1"},
		},
		Schemas: Schemas(),
	}
}

func (Provider) ValidateConfig(config map[string]any) error {
	return validateConfig(config)
}

func (Provider) ExecuteOperation(context.Context, provider.OperationRequest) (provider.OperationResult, error) {
	return provider.OperationResult{}, fmt.Errorf("stripe does not support outbound operations")
}
