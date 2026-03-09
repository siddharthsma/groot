package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	sdkintegration "groot/sdk/integration"
)

const integrationName = "example_echo_integration"

type EchoIntegration struct{}

type echoConfig struct {
	Prefix string `json:"prefix"`
}

type echoParams struct {
	Text string `json:"text"`
}

var Integration sdkintegration.Integration = &EchoIntegration{}

func (EchoIntegration) Spec() sdkintegration.IntegrationSpec {
	return sdkintegration.IntegrationSpec{
		Name:                integrationName,
		SupportsTenantScope: true,
		SupportsGlobalScope: false,
		Config: sdkintegration.ConfigSpec{
			Fields: []sdkintegration.ConfigField{
				{Name: "prefix", Required: true},
			},
		},
		Operations: []sdkintegration.OperationSpec{
			{Name: "echo", Description: "Echo text with a configured prefix"},
		},
	}
}

func (EchoIntegration) ValidateConfig(config map[string]any) error {
	var decoded echoConfig
	if err := sdkintegration.DecodeInto(config, &decoded); err != nil {
		return err
	}
	if strings.TrimSpace(decoded.Prefix) == "" {
		return errors.New("config.prefix is required")
	}
	decoded.Prefix = strings.TrimSpace(decoded.Prefix)
	return sdkintegration.RewriteConfig(config, decoded)
}

func (EchoIntegration) ExecuteOperation(_ context.Context, req sdkintegration.OperationRequest) (sdkintegration.OperationResult, error) {
	if strings.TrimSpace(req.Operation) != "echo" {
		return sdkintegration.OperationResult{}, fmt.Errorf("unsupported operation %s", req.Operation)
	}
	var cfg echoConfig
	if err := sdkintegration.DecodeInto(req.Config, &cfg); err != nil {
		return sdkintegration.OperationResult{}, err
	}
	var params echoParams
	if len(req.Params) > 0 {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return sdkintegration.OperationResult{}, fmt.Errorf("decode params: %w", err)
		}
	}
	output, _ := json.Marshal(map[string]any{
		"message": cfg.Prefix + params.Text,
	})
	return sdkintegration.OperationResult{
		StatusCode: 200,
		Output:     output,
	}, nil
}
