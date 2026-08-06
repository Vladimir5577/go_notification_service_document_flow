package notifications

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	amqp "github.com/rabbitmq/amqp091-go"

	"notification_service_document_flow/internal/service"
)

// В тестах интервал микроскопический: проверяем логику попыток, а не часы.
const testInterval = time.Millisecond

var errTemporary = errors.New("база недоступна")

func TestIsPermanent(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"value too long 22001", &pgconn.PgError{Code: "22001"}, true},
		{"invalid text 22P02", &pgconn.PgError{Code: "22P02"}, true},
		{"unique violation 23505", &pgconn.PgError{Code: "23505"}, true},
		{"not null 23502", &pgconn.PgError{Code: "23502"}, true},
		{"invalid message sentinel", fmt.Errorf("wrap: %w", errInvalidMessage), true},
		{"contract violation sentinel", fmt.Errorf("wrap: %w", service.ErrInvalidEvent), true},
		{"wrapped pg error", fmt.Errorf("create: %w", &pgconn.PgError{Code: "22001"}), true},
		{"connection failure 08006 — transient", &pgconn.PgError{Code: "08006"}, false},
		{"deadlock 40P01 — transient", &pgconn.PgError{Code: "40P01"}, false},
		{"plain error — transient", errors.New("dial tcp: connection refused"), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isPermanent(tc.err); got != tc.want {
				t.Fatalf("isPermanent(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestRetrySucceedsImmediately(t *testing.T) {
	calls := 0
	err := retryTemporary(context.Background(), 5, testInterval, func() error {
		calls++
		return nil
	})

	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if calls != 1 {
		t.Errorf("успех с первого раза должен стоить одной попытки, было %d", calls)
	}
}

// Отравленное сообщение повторять бессмысленно: минута ожидания на каждое
// такое застопорила бы всю очередь.
func TestRetryDoesNotRepeatPermanent(t *testing.T) {
	calls := 0
	err := retryTemporary(context.Background(), 5, testInterval, func() error {
		calls++
		return errInvalidMessage
	})

	if !errors.Is(err, errInvalidMessage) {
		t.Fatalf("ожидали errInvalidMessage, получили %v", err)
	}
	if calls != 1 {
		t.Errorf("постоянная ошибка не должна повторяться, попыток было %d", calls)
	}
}

func TestRetryRecoversOnLaterAttempt(t *testing.T) {
	calls := 0
	err := retryTemporary(context.Background(), 5, testInterval, func() error {
		calls++
		if calls < 3 {
			return errTemporary
		}
		return nil
	})

	if err != nil {
		t.Fatalf("после восстановления базы ошибки быть не должно: %v", err)
	}
	if calls != 3 {
		t.Errorf("ожидали 3 попытки, было %d", calls)
	}
}

// Ради этого всё и затевалось: попытки кончаются, сообщение уходит в DLQ,
// а не крутится в очереди вечно.
func TestRetryGivesUpAfterAttempts(t *testing.T) {
	calls := 0
	err := retryTemporary(context.Background(), temporaryAttempts, testInterval, func() error {
		calls++
		return errTemporary
	})

	if !errors.Is(err, errTemporary) {
		t.Fatalf("ожидали временную ошибку, получили %v", err)
	}
	if calls != temporaryAttempts {
		t.Errorf("ожидали ровно %d попыток, было %d", temporaryAttempts, calls)
	}
	if isPermanent(err) {
		t.Error("временная ошибка не должна попадать в isPermanent — иначе лог соврёт о причине")
	}
}

// Выключение сервиса не должно ждать оставшиеся паузы: при интервале в 15
// секунд это минута на каждое сообщение в работе.
func TestRetryStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	calls := 0
	start := time.Now()
	err := retryTemporary(ctx, 5, time.Hour, func() error {
		calls++
		cancel()
		return errTemporary
	})

	if !errors.Is(err, errTemporary) {
		t.Fatalf("ожидали временную ошибку, получили %v", err)
	}
	if calls != 1 {
		t.Errorf("после отмены контекста повторов быть не должно, было %d", calls)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("ждали паузу вместо выхода по контексту: %s", elapsed)
	}
}

// Паника не должна убивать процесс: иначе сообщение остаётся неподтверждённым,
// RabbitMQ возвращает его в очередь, и после перезапуска сервис падает на нём
// снова. Тот самый цикл, который не лечится никакой политикой повторов.
//
// Consumer без svc: обращение к нему внутри обработки даст nil-разыменование.
func TestSafeProcessRecoversPanic(t *testing.T) {
	c := &Consumer{}
	delivery := amqp.Delivery{
		RoutingKey: "kanban.notification.card_created",
		Body: []byte(`{"eventId":"018f0c3e-7a11-7c9d-9f2a-4d5b6e7f8a90",` +
			`"title":"Новая задача","recipients":[187]}`),
	}

	err := c.safeProcess(context.Background(), delivery)
	if err == nil {
		t.Fatal("паника должна превратиться в ошибку, а не уронить процесс")
	}
	if !isPermanent(err) {
		t.Error("паника должна считаться постоянной ошибкой — иначе сообщение вернётся и уронит сервис снова")
	}
}
