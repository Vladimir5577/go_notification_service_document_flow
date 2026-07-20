package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	amqp "github.com/rabbitmq/amqp091-go"

	"notification_service_document_flow/internal/config"
	"notification_service_document_flow/internal/messaging/events"
	"notification_service_document_flow/internal/service"
)

type Consumer struct {
	dsn            string
	exchange       string
	queue          string
	svc            *service.NotificationService
	prefetchCount  int
	reconnectDelay time.Duration
}

func NewConsumer(cfg *config.Config, svc *service.NotificationService) *Consumer {
	return &Consumer{
		dsn:            cfg.RabbitMQDSN,
		exchange:       cfg.RabbitMQExchange,
		queue:          "notification.kanban_events",
		svc:            svc,
		prefetchCount:  20,
		reconnectDelay: 5 * time.Second,
	}
}

func (c *Consumer) Run(ctx context.Context) error {
	if strings.TrimSpace(c.dsn) == "" {
		slog.Warn("RabbitMQ DSN не задан, потребление уведомлений отключено")
		<-ctx.Done()
		return nil
	}

	for {
		if err := c.consume(ctx); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			slog.Warn("RabbitMQ consumer (kanban notifications) waiting to reconnect", "error", err)
		}

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(c.reconnectDelay):
		}
	}
}

func (c *Consumer) consume(ctx context.Context) error {
	conn, err := amqp.Dial(c.dsn)
	if err != nil {
		return fmt.Errorf("connect rabbitmq: %w", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("open rabbitmq channel: %w", err)
	}
	defer ch.Close()

	if err := ch.ExchangeDeclare(c.exchange, "topic", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare exchange: %w", err)
	}

	q, err := ch.QueueDeclare(c.queue, true, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("declare queue: %w", err)
	}

	// Bind to all kanban.notification.* events
	routingKeys := []string{
		"kanban.notification.*",
	}

	for _, rk := range routingKeys {
		if err := ch.QueueBind(q.Name, rk, c.exchange, false, nil); err != nil {
			return fmt.Errorf("bind %s: %w", rk, err)
		}
	}

	if err := ch.Qos(c.prefetchCount, 0, false); err != nil {
		return fmt.Errorf("qos: %w", err)
	}

	deliveries, err := ch.Consume(q.Name, "notification-kanban", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume: %w", err)
	}

	connClosed := conn.NotifyClose(make(chan *amqp.Error, 1))
	chClosed := ch.NotifyClose(make(chan *amqp.Error, 1))

	slog.Info("Kanban notification consumer запущен", "queue", q.Name)

	for {
		select {
		case <-ctx.Done():
			return nil
		case amqpErr := <-connClosed:
			if amqpErr != nil {
				return amqpErr
			}
			return errors.New("connection closed")
		case amqpErr := <-chClosed:
			if amqpErr != nil {
				return amqpErr
			}
			return errors.New("channel closed")
		case delivery, ok := <-deliveries:
			if !ok {
				return errors.New("deliveries closed")
			}
			c.handleDelivery(ctx, delivery)
		}
	}
}

var errInvalidMessage = errors.New("invalid kanban notification message")

func (c *Consumer) handleDelivery(ctx context.Context, delivery amqp.Delivery) {
	if err := c.processDelivery(ctx, delivery); err != nil {
		// Постоянные ошибки (битый JSON, слишком длинное значение, нарушение
		// constraint) повтором не лечатся — сколько ни возвращай в очередь,
		// результат тот же. Requeue такого сообщения = бесконечный цикл, ровно
		// он и уронил сервис. Убираем из очереди: Nack(requeue=false) отправит
		// его в dead-letter exchange (если задана политика) либо отбросит.
		if isPermanent(err) {
			slog.Error("kanban notification событие необрабатываемо, снято с очереди",
				"routing_key", delivery.RoutingKey, "error", err, "body", bodyPreview(delivery.Body))
			if nackErr := delivery.Nack(false, false); nackErr != nil {
				slog.Error("Не удалось снять сообщение с очереди RabbitMQ", "error", nackErr)
			}
			return
		}

		// Временная ошибка (например, БД недоступна) — возвращаем на повтор.
		slog.Warn("kanban notification событие: временная ошибка, будет повторено",
			"routing_key", delivery.RoutingKey, "error", err)
		if nackErr := delivery.Nack(false, true); nackErr != nil {
			slog.Error("Не удалось вернуть сообщение в RabbitMQ", "error", nackErr)
		}
		return
	}

	if err := delivery.Ack(false); err != nil {
		slog.Error("Не удалось подтвердить сообщение RabbitMQ", "error", err)
	}
}

// isPermanent сообщает, что повтор бесполезен: сообщение отравленное
// (poison message) и должно уйти из очереди, а не крутиться в ней.
func isPermanent(err error) bool {
	if errors.Is(err, errInvalidMessage) {
		return true
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && len(pgErr.Code) >= 2 {
		switch pgErr.Code[:2] {
		case "22", // data exception (22001 value too long, 22P02 invalid text, ...)
			"23": // integrity constraint violation (not-null, unique, check, fk)
			return true
		}
	}
	return false
}

func bodyPreview(body []byte) string {
	const max = 512
	if len(body) > max {
		return string(body[:max]) + "…"
	}
	return string(body)
}

func (c *Consumer) processDelivery(ctx context.Context, delivery amqp.Delivery) error {
	routingKey := delivery.RoutingKey

	if strings.HasPrefix(routingKey, "kanban.notification.") {
		var evt events.KanbanNotificationEvent
		if err := json.Unmarshal(delivery.Body, &evt); err != nil {
			return fmt.Errorf("%w: %w", errInvalidMessage, err)
		}
		if err := c.svc.CreateFromKanbanEvent(ctx, evt); err != nil {
			return err
		}
		return nil
	}

	// Future: document.* etc.
	// if strings.HasPrefix(routingKey, "document.notification.") { ... }

	slog.Debug("Unhandled notification routing key", "routing", routingKey)
	return nil
}
