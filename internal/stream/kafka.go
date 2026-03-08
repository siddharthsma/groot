package stream

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"

	eventpkg "groot/internal/event"
)

const EventsTopic = "events"

type Client struct {
	brokers []string
	writer  *kafka.Writer
}

func New(brokers []string) *Client {
	return &Client{
		brokers: brokers,
		writer: &kafka.Writer{
			Addr:         kafka.TCP(brokers...),
			Topic:        EventsTopic,
			Balancer:     &kafka.LeastBytes{},
			RequiredAcks: kafka.RequireAll,
			Async:        false,
		},
	}
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

func (c *Client) EnsureTopic(ctx context.Context) error {
	conn, err := kafka.DialContext(ctx, "tcp", c.brokers[0])
	if err != nil {
		return fmt.Errorf("dial kafka broker: %w", err)
	}
	defer func() {
		_ = conn.Close()
	}()

	controller, err := conn.Controller()
	if err != nil {
		return fmt.Errorf("get kafka controller: %w", err)
	}

	controllerConn, err := kafka.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", controller.Host, controller.Port))
	if err != nil {
		return fmt.Errorf("dial kafka controller: %w", err)
	}
	defer func() {
		_ = controllerConn.Close()
	}()

	err = controllerConn.CreateTopics(kafka.TopicConfig{
		Topic:             EventsTopic,
		NumPartitions:     1,
		ReplicationFactor: 1,
	})
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return fmt.Errorf("create kafka topic %q: %w", EventsTopic, err)
	}

	return nil
}

func (c *Client) PublishEvent(ctx context.Context, event eventpkg.Event) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := c.writer.WriteMessages(writeCtx, kafka.Message{
		Key:   []byte(event.EventID.String()),
		Value: payload,
		Time:  event.Timestamp,
	}); err != nil {
		return fmt.Errorf("write kafka message: %w", err)
	}

	return nil
}

func (c *Client) Close() error {
	return c.writer.Close()
}
