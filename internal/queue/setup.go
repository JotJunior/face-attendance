// Package queue manages the RabbitMQ topology and message publishing/consuming.
// Topology: exchange → member.processing queue; DLX → member.processing.dlq.
// All declarations are idempotent (can be called multiple times safely).
package queue

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	// ExchangeName is the main exchange for member processing messages.
	ExchangeName = "member.processing.exchange"
	// QueueName is the main work queue.
	QueueName = "member.processing"
	// DLXName is the dead-letter exchange.
	DLXName = "member.processing.dlx"
	// DLQName is the dead-letter queue.
	DLQName = "member.processing.dlq"
	// RetryCountHeader is the AMQP header tracking retry attempts.
	RetryCountHeader = "x-retry-count"
)

// SetupTopology declares the RabbitMQ exchanges and queues.
// Idempotent: can be called on reconnect without errors.
func SetupTopology(ch *amqp.Channel) error {
	// 1. Declare DLX (fanout — routes everything to DLQ)
	if err := ch.ExchangeDeclare(DLXName, "fanout", true, false, false, false, nil); err != nil {
		return fmt.Errorf("queue: declare DLX %s: %w", DLXName, err)
	}

	// 2. Declare DLQ
	_, err := ch.QueueDeclare(DLQName, true, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("queue: declare DLQ %s: %w", DLQName, err)
	}

	// 3. Bind DLQ to DLX
	if err := ch.QueueBind(DLQName, "", DLXName, false, nil); err != nil {
		return fmt.Errorf("queue: bind DLQ to DLX: %w", err)
	}

	// 4. Declare main exchange
	if err := ch.ExchangeDeclare(ExchangeName, "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("queue: declare exchange %s: %w", ExchangeName, err)
	}

	// 5. Declare main queue with DLX configured
	args := amqp.Table{
		"x-dead-letter-exchange":    DLXName,
		"x-dead-letter-routing-key": "",
	}
	_, err = ch.QueueDeclare(QueueName, true, false, false, false, args)
	if err != nil {
		return fmt.Errorf("queue: declare queue %s: %w", QueueName, err)
	}

	// 6. Bind main queue to exchange
	if err := ch.QueueBind(QueueName, QueueName, ExchangeName, false, nil); err != nil {
		return fmt.Errorf("queue: bind queue to exchange: %w", err)
	}

	return nil
}

// Connect creates an AMQP connection and channel.
func Connect(url string) (*amqp.Connection, *amqp.Channel, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, nil, fmt.Errorf("queue: dial %s: %w", url, err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close() //nolint:errcheck
		return nil, nil, fmt.Errorf("queue: open channel: %w", err)
	}

	return conn, ch, nil
}
