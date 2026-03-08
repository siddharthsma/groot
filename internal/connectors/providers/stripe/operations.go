package stripe

import (
	"context"
	"fmt"

	"groot/internal/connectors/provider"
)

func executeUnsupportedOperation(context.Context, provider.OperationRequest) (provider.OperationResult, error) {
	return provider.OperationResult{}, fmt.Errorf("stripe does not support outbound operations")
}
