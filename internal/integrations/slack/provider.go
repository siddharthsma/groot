package slack

import (
	"context"

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
		SupportsGlobalScope: true,
		Config: integration.ConfigSpec{
			Fields: []integration.ConfigField{
				{Name: "bot_token", Required: true, Secret: true},
				{Name: "default_channel"},
			},
		},
		Inbound: &integration.InboundSpec{
			RouteKeyStrategy: "slack_team",
			EventTypes: []string{
				"slack.message.created.v1",
				"slack.app_mentioned.v1",
				"slack.reaction.added.v1",
			},
		},
		Operations: []integration.OperationSpec{
			{Name: OperationPostMessage, Description: "Post a message to a Slack channel"},
			{Name: OperationCreateThreadReply, Description: "Post a reply into a Slack thread"},
		},
		Schemas: Schemas(),
	}
}

func (Integration) ValidateConfig(config map[string]any) error {
	return validateConfig(config)
}

func (Integration) ExecuteOperation(ctx context.Context, req integration.OperationRequest) (integration.OperationResult, error) {
	var client HTTPClient
	if req.HTTPClient != nil {
		client = req.HTTPClient
	}
	return New(req.Runtime.Slack.APIBaseURL, client).Execute(ctx, req.Operation, mustMarshalConfig(req.Config), req.Params, req.Event)
}
