package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	sdkprovider "groot/sdk/provider"
)

const providerName = "example_echo_provider"

type EchoProvider struct{}

type echoConfig struct {
	Prefix string `json:"prefix"`
}

type echoParams struct {
	Text string `json:"text"`
}

var Provider sdkprovider.Provider = &EchoProvider{}

func (EchoProvider) Spec() sdkprovider.ProviderSpec {
	return sdkprovider.ProviderSpec{
		Name:                providerName,
		SupportsTenantScope: true,
		SupportsGlobalScope: false,
		Config: sdkprovider.ConfigSpec{
			Fields: []sdkprovider.ConfigField{
				{Name: "prefix", Required: true},
			},
		},
		Operations: []sdkprovider.OperationSpec{
			{Name: "echo", Description: "Echo text with a configured prefix"},
		},
	}
}

func (EchoProvider) ValidateConfig(config map[string]any) error {
	var decoded echoConfig
	if err := sdkprovider.DecodeInto(config, &decoded); err != nil {
		return err
	}
	if strings.TrimSpace(decoded.Prefix) == "" {
		return errors.New("config.prefix is required")
	}
	decoded.Prefix = strings.TrimSpace(decoded.Prefix)
	return sdkprovider.RewriteConfig(config, decoded)
}

func (EchoProvider) ExecuteOperation(_ context.Context, req sdkprovider.OperationRequest) (sdkprovider.OperationResult, error) {
	if strings.TrimSpace(req.Operation) != "echo" {
		return sdkprovider.OperationResult{}, fmt.Errorf("unsupported operation %s", req.Operation)
	}
	var cfg echoConfig
	if err := sdkprovider.DecodeInto(req.Config, &cfg); err != nil {
		return sdkprovider.OperationResult{}, err
	}
	var params echoParams
	if len(req.Params) > 0 {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return sdkprovider.OperationResult{}, fmt.Errorf("decode params: %w", err)
		}
	}
	output, _ := json.Marshal(map[string]any{
		"message": cfg.Prefix + params.Text,
	})
	return sdkprovider.OperationResult{
		StatusCode: 200,
		Output:     output,
	}, nil
}
