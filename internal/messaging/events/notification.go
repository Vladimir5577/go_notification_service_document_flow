package events

import (
	"bytes"
	"encoding/json"
)

// Payload — непрозрачный мешок данных события.
//
// Существует отдельным типом ради одной строчки в UnmarshalJSON: PHP
// сериализует пустой ассоциативный массив как JSON-массив `[]`, а не `{}`.
// Обычный map[string]any на таком теле падает, сообщение снимается с очереди,
// и уведомление теряется молча. Ловушка не разовая — её повторит любой
// следующий PHP-продюсер, поэтому терпимость живёт на стороне потребителя.
type Payload map[string]any

func (p *Payload) UnmarshalJSON(b []byte) error {
	if bytes.Equal(bytes.TrimSpace(b), []byte("[]")) {
		return nil
	}

	return json.Unmarshal(b, (*map[string]any)(p))
}

// NotificationEvent — общий контракт уведомления для всех модулей-источников.
//
// Routing key: "<модуль>.notification.<событие>", например
// "purchase.notification.submitted" или "kanban.notification.card_created".
// Модуль и тип события в теле НЕ дублируются — сервис берёт их из ключа и
// собирает из них Type для базы. Одно место истины: продюсер не может
// разойтись сам с собой.
//
// Готовый текст присылает продюсер: только он знает формулировки своего
// модуля. Сервис их не собирает и потому ничего про модули не знает —
// новый источник не требует здесь ни строки.
type NotificationEvent struct {
	// EventID — uuid v7, один на событие (не на получателя). Продюсер обязан
	// сгенерировать его при СОЗДАНИИ события и переиспользовать при повторной
	// отправке: новый id на ретрае убивает дедупликацию.
	EventID string `json:"eventId"`

	// Recipients — id пользователей монолита, те же, что приходят в user sync.
	Recipients []int64 `json:"recipients"`

	// Title — заголовок уведомления, готовый к показу.
	Title string `json:"title"`

	// TypeLabel — короткая подпись категории для списка («Создана задача»).
	// Необязательное: пусто — список покажет только заголовок.
	TypeLabel string `json:"typeLabel,omitempty"`

	// Message — дополнительный текст, например комментарий.
	Message *string `json:"message,omitempty"`

	// Link — маршрут SPA, куда ведёт уведомление.
	Link *string `json:"link,omitempty"`

	// ActorID — кто совершил действие. Исключать актора из получателей —
	// задача продюсера, здесь поле только для справки и отладки.
	ActorID int64 `json:"actorId,omitempty"`

	// Data — непрозрачный мешок, уезжает в extra как есть. Сервис его не
	// интерпретирует. Тип Payload, а не map, чтобы пережить пустой `[]` от PHP.
	Data Payload `json:"data,omitempty"`
}
