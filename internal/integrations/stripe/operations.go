package stripe

import (
	"context"
	"fmt"

	"groot/internal/integrations"
)

func executeUnsupportedOperation(context.Context, integration.OperationRequest) (integration.OperationResult, error) {
	return integration.OperationResult{}, fmt.Errorf("stripe does not support outbound operations")
}
