package connectors

import (
	"context"
	"net/http"
)

type Inbound interface {
	Name() string
	HandleWebhook(context.Context, []byte, http.Header) error
}
