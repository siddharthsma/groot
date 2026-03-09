package stripe

import (
	"context"
	"fmt"

	"groot/internal/integrations"
	"groot/internal/integrations/registry"
)

type Integration struct{}

func init() {
	registry.RegisterIntegration(Integration{})
}

func (Integration) Spec() integration.IntegrationSpec {
	return integration.IntegrationSpec{
		Name:                IntegrationName,
		SupportsTenantScope: true,
		SupportsGlobalScope: false,
		Config: integration.ConfigSpec{
			Fields: []integration.ConfigField{
				{Name: "stripe_account_id", Required: true},
				{Name: "webhook_secret", Required: true, Secret: true},
			},
		},
		Inbound: &integration.InboundSpec{
			RouteKeyStrategy: "stripe_account",
			EventTypes:       []string{"stripe.payment_intent.succeeded.v1"},
		},
		Schemas: Schemas(),
	}
}

func (Integration) ValidateConfig(config map[string]any) error {
	return validateConfig(config)
}

func (Integration) ExecuteOperation(context.Context, integration.OperationRequest) (integration.OperationResult, error) {
	return integration.OperationResult{}, fmt.Errorf("stripe does not support outbound operations")
}
