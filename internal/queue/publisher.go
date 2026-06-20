package queue

import (
	"context"
	"encoding/json"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/jotjunior/face-attendance/internal/domain"
)

// Publisher publishes ProcessingMessage values to the member.processing queue.
type Publisher struct {
	ch *amqp.Channel
}

// NewPublisher creates a Publisher using the given AMQP channel.
func NewPublisher(ch *amqp.Channel) *Publisher {
	return &Publisher{ch: ch}
}

// Publish serializes msg as JSON and publishes it to the main queue with x-retry-count=0.
// If the channel fails, the error is returned immediately (no partial publish — US2 cenario 3).
func (p *Publisher) Publish(ctx context.Context, msg domain.ProcessingMessage) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("publisher: marshal message: %w", err)
	}

	pub := amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body:         body,
		Headers: amqp.Table{
			RetryCountHeader: int32(0),
		},
	}

	// Use mandatory=false, immediate=false (standard fanout semantics)
	if err := p.ch.PublishWithContext(ctx, ExchangeName, QueueName, false, false, pub); err != nil {
		return fmt.Errorf("publisher: publish to %s: %w", QueueName, err)
	}

	return nil
}
