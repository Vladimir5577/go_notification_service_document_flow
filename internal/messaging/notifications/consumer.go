package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
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
			slog.Warn("RabbitMQ consumer (notifications) waiting to reconnect", "error", err)
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

	// Одна привязка на все модули-источники: <модуль>.notification.<событие>.
	//
	// Не `*.notification.*`: в RabbitMQ `*` — ровно одно слово между точками,
	// и составное событие (document.notification.status.changed) под такую
	// маску не попадёт, а topic exchange выбросит его молча. `#` — любое
	// число слов.
	//
	// Захардкоженный список ключей стоил тринадцати дней тишины по закупкам:
	// модуль публиковал, привязки не было, следов не осталось нигде.
	const routingKey = "*.notification.#"

	if err := ch.QueueBind(q.Name, routingKey, c.exchange, false, nil); err != nil {
		return fmt.Errorf("bind %s: %w", routingKey, err)
	}

	if err := ch.Qos(c.prefetchCount, 0, false); err != nil {
		return fmt.Errorf("qos: %w", err)
	}

	deliveries, err := ch.Consume(q.Name, "notification-events", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume: %w", err)
	}

	connClosed := conn.NotifyClose(make(chan *amqp.Error, 1))
	chClosed := ch.NotifyClose(make(chan *amqp.Error, 1))

	slog.Info("Notification consumer запущен", "queue", q.Name, "routing_key", routingKey)

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

// Модуль в текст не зашиваем: консьюмер общий на все источники, а какой именно
// прислал битое тело — видно в routing_key, который логируется рядом.
var errInvalidMessage = errors.New("invalid notification message")

// Сколько раз пытаемся пережить временную ошибку, не выпуская сообщение из рук,
// и пауза между попытками. 5 попыток по 15 секунд — это минута ожидания, за
// которую перезапуск базы или переключение на реплику успевают закончиться.
//
// Пока обработчик ждёт, сообщения просто лежат в очереди: не теряются, не
// крутятся, никого не греют. Единственная плата — задержка уведомления на
// время сбоя, для колокольчика несущественная.
//
// ponytail: фиксированный интервал, не экспонента. Подкрутить обе константы —
// дело одной строки, когда станет видно реальное поведение базы.
const (
	temporaryAttempts = 5
	temporaryInterval = 15 * time.Second
)

func (c *Consumer) handleDelivery(ctx context.Context, delivery amqp.Delivery) {
	err := c.safeProcess(ctx, delivery)

	if err != nil {
		// Requeue не делаем ни при какой ошибке. Постоянные (битый JSON,
		// нарушение контракта, constraint) повтором не лечатся, а временные
		// уже пережили минуту попыток. Именно бесконечный requeue когда-то
		// и уронил сервис. Nack(requeue=false) отправит сообщение в
		// events.dlx — политика dlx-go на очередь настроена.
		if isPermanent(err) {
			slog.Error("событие необрабатываемо, снято с очереди",
				"routing_key", delivery.RoutingKey, "error", err, "body", bodyPreview(delivery.Body))
		} else {
			slog.Error("событие не обработано за все попытки, снято с очереди",
				"routing_key", delivery.RoutingKey, "attempts", temporaryAttempts,
				"error", err, "body", bodyPreview(delivery.Body))
		}

		if nackErr := delivery.Nack(false, false); nackErr != nil {
			slog.Error("Не удалось снять сообщение с очереди RabbitMQ", "error", nackErr)
		}
		return
	}

	if err := delivery.Ack(false); err != nil {
		slog.Error("Не удалось подтвердить сообщение RabbitMQ", "error", err)
	}
}

// safeProcess обрабатывает доставку с повторами и ловит панику.
//
// Паника опаснее любой ошибки: непойманная, она убивает процесс целиком,
// сообщение остаётся неподтверждённым, RabbitMQ возвращает его в очередь — и
// после перезапуска сервис падает на нём снова. Такой цикл не лечится никакой
// политикой повторов, а консьюмер работает в своей горутине, где HTTP-мидлварь
// его не прикрывает.
//
// Превращаем панику в постоянную ошибку: сообщение уедет в DLQ вместе со
// стектрейсом в логе, а сервис продолжит работать.
func (c *Consumer) safeProcess(ctx context.Context, delivery amqp.Delivery) (err error) {
	defer func() {
		if p := recover(); p != nil {
			slog.Error("Паника при обработке события уведомления",
				"routing_key", delivery.RoutingKey, "panic", p,
				"stack", string(debug.Stack()))
			err = fmt.Errorf("%w: паника: %v", errInvalidMessage, p)
		}
	}()

	return retryTemporary(ctx, temporaryAttempts, temporaryInterval, func() error {
		return c.processDelivery(ctx, delivery)
	})
}

// retryTemporary повторяет do на временных ошибках. Постоянные возвращает сразу:
// повторять битое сообщение бессмысленно, а минута ожидания на каждое такое
// сообщение застопорила бы всю очередь.
//
// Повтор безопасен, потому что у события есть eventId: уже созданные уведомления
// отсекаются по (event_id, user_id), и частично прошедшее событие не задвоится.
func retryTemporary(ctx context.Context, attempts int, interval time.Duration, do func() error) error {
	var err error

	for attempt := 1; attempt <= attempts; attempt++ {
		err = do()
		if err == nil || isPermanent(err) {
			return err
		}
		if attempt == attempts {
			break
		}

		slog.Warn("временная ошибка обработки события, повтор",
			"attempt", attempt, "of", attempts, "retry_in", interval, "error", err)

		select {
		case <-ctx.Done():
			// Иначе выключение сервиса ждало бы все оставшиеся паузы.
			return err
		case <-time.After(interval):
		}
	}

	return err
}

// isPermanent сообщает, что повтор бесполезен: сообщение отравленное
// (poison message) и должно уйти из очереди, а не крутиться в ней.
func isPermanent(err error) bool {
	// Битый JSON и нарушение контракта (пустой title, чужой routing key) —
	// одного класса: сколько ни повторяй, продюсер уже прислал что прислал.
	if errors.Is(err, errInvalidMessage) || errors.Is(err, service.ErrInvalidEvent) {
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

// processDelivery не разбирает, из какого модуля пришло событие: тело у всех
// одно, тип сервис собирает из routing key. Добавление модуля-источника здесь
// не требует ни строки.
func (c *Consumer) processDelivery(ctx context.Context, delivery amqp.Delivery) error {
	var evt events.NotificationEvent
	if err := json.Unmarshal(delivery.Body, &evt); err != nil {
		return fmt.Errorf("%w: %w", errInvalidMessage, err)
	}

	return c.svc.CreateFromEvent(ctx, delivery.RoutingKey, evt)
}
