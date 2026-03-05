package stream

import (
	"context"
	"fmt"

	"github.com/segmentio/kafka-go"
)

type Client struct {
	brokers []string
}

func New(brokers []string) *Client {
	return &Client{brokers: brokers}
}

func (c *Client) Check(ctx context.Context) error {
	var firstErr error

	for _, broker := range c.brokers {
		conn, err := kafka.DialContext(ctx, "tcp", broker)
		if err == nil {
			if closeErr := conn.Close(); closeErr != nil {
				return fmt.Errorf("close kafka connection: %w", closeErr)
			}
			return nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}

	return fmt.Errorf("dial kafka brokers %v: %w", c.brokers, firstErr)
}
