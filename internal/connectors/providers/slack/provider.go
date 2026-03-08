package slack

import (
	"context"

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
		SupportsGlobalScope: true,
		Config: provider.ConfigSpec{
			Fields: []provider.ConfigField{
				{Name: "bot_token", Required: true, Secret: true},
				{Name: "default_channel"},
			},
		},
		Inbound: &provider.InboundSpec{
			RouteKeyStrategy: "slack_team",
			EventTypes: []string{
				"slack.message.created.v1",
				"slack.app_mentioned.v1",
				"slack.reaction.added.v1",
			},
		},
		Operations: []provider.OperationSpec{
			{Name: OperationPostMessage, Description: "Post a message to a Slack channel"},
			{Name: OperationCreateThreadReply, Description: "Post a reply into a Slack thread"},
		},
		Schemas: Schemas(),
	}
}

func (Provider) ValidateConfig(config map[string]any) error {
	return validateConfig(config)
}

func (Provider) ExecuteOperation(ctx context.Context, req provider.OperationRequest) (provider.OperationResult, error) {
	var client HTTPClient
	if req.HTTPClient != nil {
		client = req.HTTPClient
	}
	return New(req.Runtime.Slack.APIBaseURL, client).Execute(ctx, req.Operation, mustMarshalConfig(req.Config), req.Params, req.Event)
}
