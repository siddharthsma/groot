package temporal

import (
	"context"
	"fmt"

	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Client struct {
	address   string
	namespace string
}

func New(address, namespace string) *Client {
	return &Client{
		address:   address,
		namespace: namespace,
	}
}

func (c *Client) Options() client.Options {
	return client.Options{
		HostPort:  c.address,
		Namespace: c.namespace,
	}
}

func (c *Client) Dial() (client.Client, error) {
	temporalClient, err := client.Dial(c.Options())
	if err != nil {
		return nil, fmt.Errorf("dial temporal client: %w", err)
	}
	return temporalClient, nil
}

func (c *Client) Check(ctx context.Context) error {
	conn, err := grpc.DialContext(
		ctx,
		c.address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return fmt.Errorf("dial temporal frontend: %w", err)
	}

	service := workflowservice.NewWorkflowServiceClient(conn)
	if _, err := service.GetSystemInfo(ctx, &workflowservice.GetSystemInfoRequest{}); err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			return fmt.Errorf("close temporal connection after failed system info call: %w", closeErr)
		}
		return fmt.Errorf("call temporal system info: %w", err)
	}

	if err := conn.Close(); err != nil {
		return fmt.Errorf("close temporal connection: %w", err)
	}

	return nil
}
