package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"notification_service_document_flow/internal/helper"
	"notification_service_document_flow/internal/messaging/events"
	"notification_service_document_flow/internal/model"
	"notification_service_document_flow/internal/repository"
)

// ErrInvalidEvent — событие нарушает контракт. Повтором не лечится: консьюмер
// снимет такое сообщение с очереди вместо бесконечного requeue.
var ErrInvalidEvent = errors.New("invalid notification event")

// Ограничение колонки notification.type. Длиннее — почти наверняка мусорный
// routing key, а не настоящий модуль. Обрезать молча нельзя: разные события
// схлопнулись бы в один тип.
const maxTypeLength = 50

// NotificationService создаёт уведомления из событий любого модуля.
//
// Про модули он ничего не знает и знать не должен: готовый текст присылает
// продюсер, тип собирается из routing key. Новый источник не требует здесь
// ни строки — в этом весь смысл общего контракта.
type NotificationService struct {
	repo   *repository.NotificationRepository
	clock  helper.Clock
	pinger *Pinger
	pusher *Pusher
}

func NewNotificationService(repo *repository.NotificationRepository, clk helper.Clock, pinger *Pinger, pusher *Pusher) *NotificationService {
	return &NotificationService{repo: repo, clock: clk, pinger: pinger, pusher: pusher}
}

// CreateFromEvent заводит по уведомлению на каждого известного получателя.
//
// Возвращает ошибку, если хоть одна вставка сорвалась по временной причине —
// консьюмер вернёт сообщение в очередь. Повторная обработка безопасна: дубли
// отсекаются по (event_id, user_id).
func (s *NotificationService) CreateFromEvent(ctx context.Context, routingKey string, evt events.NotificationEvent) error {
	notifType, err := typeFromRoutingKey(routingKey)
	if err != nil {
		return err
	}

	if err := validateEvent(evt); err != nil {
		return err
	}

	recipients := uniqueRecipients(evt.Recipients)
	if len(recipients) == 0 {
		return nil
	}

	var extra json.RawMessage
	if len(evt.Data) > 0 {
		if raw, err := json.Marshal(evt.Data); err == nil {
			extra = raw
		} else {
			slog.Warn("не удалось сериализовать data события", "event_id", evt.EventID, "error", err)
		}
	}

	link := normalizeLink(evt.Link)
	createdAt := s.clock.Now()

	var firstErr error
	fresh := make([]int64, 0, len(recipients))
	for _, recipientID := range recipients {
		created, err := s.repo.Create(ctx, &model.Notification{
			EventID:   evt.EventID,
			Type:      notifType,
			TypeLabel: evt.TypeLabel,
			Title:     evt.Title,
			Message:   evt.Message,
			Link:      link,
			CreatedAt: createdAt,
			UserID:    recipientID,
			Extra:     extra,
		})
		if err != nil {
			slog.Error("не удалось создать уведомление",
				"event_id", evt.EventID, "type", notifType, "user_id", recipientID, "error", err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if created == nil {
			// Повтор того же события — строка уже есть, это норма, а не ошибка.
			slog.Debug("дубль события пропущен",
				"event_id", evt.EventID, "type", notifType, "user_id", recipientID)
			continue
		}

		slog.Info("уведомление создано",
			"event_id", evt.EventID, "type", notifType, "user_id", recipientID, "title", evt.Title)
		fresh = append(fresh, recipientID)
		s.pusher.Push(created)
	}

	// Пингуем и при частичной ошибке: эти строки уже в базе. Повтор события их
	// не пропингует второй раз — дубли отсечены выше.
	s.pinger.Ping(fresh...)

	return firstErr
}

// typeFromRoutingKey собирает тип для базы из ключа. Модуль и событие в теле
// не дублируются, чтобы продюсер не мог разойтись сам с собой.
//
//	purchase.notification.submitted       → PURCHASE_SUBMITTED
//	document.notification.status.changed  → DOCUMENT_STATUS_CHANGED
func typeFromRoutingKey(routingKey string) (string, error) {
	const marker = ".notification."

	i := strings.Index(routingKey, marker)
	if i <= 0 || i+len(marker) >= len(routingKey) {
		return "", fmt.Errorf("%w: routing key %q не вида <модуль>.notification.<событие>",
			ErrInvalidEvent, routingKey)
	}

	module, event := routingKey[:i], routingKey[i+len(marker):]
	if strings.Contains(module, ".") {
		return "", fmt.Errorf("%w: в routing key %q модуль должен быть одним словом",
			ErrInvalidEvent, routingKey)
	}

	typ := strings.ToUpper(module + "_" + strings.NewReplacer(".", "_", "-", "_").Replace(event))
	if len(typ) > maxTypeLength {
		return "", fmt.Errorf("%w: тип %q длиннее %d символов (routing key %q)",
			ErrInvalidEvent, typ, maxTypeLength, routingKey)
	}

	return typ, nil
}

// validateEvent ловит нарушения контракта на входе. Контракт слабее компилятора:
// раньше забытый case давал заголовок из default, теперь забытое поле дало бы
// пустое уведомление в колокольчике — и опять молча.
func validateEvent(evt events.NotificationEvent) error {
	if strings.TrimSpace(evt.EventID) == "" {
		return fmt.Errorf("%w: пустой eventId", ErrInvalidEvent)
	}
	if strings.TrimSpace(evt.Title) == "" {
		return fmt.Errorf("%w: пустой title (event_id %s)", ErrInvalidEvent, evt.EventID)
	}
	if len(evt.Recipients) == 0 {
		return fmt.Errorf("%w: пустой recipients (event_id %s)", ErrInvalidEvent, evt.EventID)
	}
	return nil
}

// uniqueRecipients убирает повторы и нечисловой мусор: один человек мог попасть
// в список дважды — например, и как автор, и как участник.
//
// Сверки со справочником здесь нет и быть не должно. Реплика users из сервиса
// удалена: колокольчик её не спрашивал (пользователь берётся из JWT, уведомления
// выбираются по user_id без join'а), а отсев по ней съедал уведомления живых
// людей, которых просто не успел донести user sync.
func uniqueRecipients(ids []int64) []int64 {
	unique := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))

	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}

	return unique
}

// normalizeLink оставляет от ссылки только путь с query.
//
// Уведомление ведёт внутрь портала, а абсолютный адрес из события был бы
// готовым редиректом на чужой сайт по клику из колокольчика.
//
// Прежняя версия ещё и переписывала легаси-адреса монолита (/kanban_board/12,
// /document_view/45) в маршруты SPA. Это больше не нужно: продюсеры общего
// контракта присылают готовый маршрут, а монолитные источники уходят.
func normalizeLink(raw *string) *string {
	if raw == nil {
		return nil
	}

	s := strings.TrimSpace(*raw)
	if s == "" {
		return nil
	}

	u, err := url.Parse(s)
	if err != nil || !strings.HasPrefix(u.Path, "/") {
		slog.Warn("ссылка события отброшена: ожидался путь внутри портала", "link", s)
		return nil
	}

	path := u.Path
	if u.RawQuery != "" {
		path += "?" + u.RawQuery
	}

	return &path
}
