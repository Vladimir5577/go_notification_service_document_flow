package service

import (
	"encoding/json"
	"testing"

	"notification_service_document_flow/internal/messaging/events"
)

// Тело — байт в байт то, что реально сериализует канбан-продюсер.
// Парный тест-источник: go_kanban_service/internal/messaging/events/kanban_notifications_test.go
//
// Контракт держится на именах json-полей, а их не проверяет ни один компилятор:
// переименуй поле на одной стороне — вторая молча получит пустой заголовок.
// Эти два теста и есть та проверка, которой раньше не было.
const kanbanPayload = `{` +
	`"eventId":"018f0c3e-7a11-7c9d-9f2a-4d5b6e7f8a90",` +
	`"title":"Новая задача «Свет в холле» на доске «Спринт 12»",` +
	`"typeLabel":"Создана задача",` +
	`"link":"/projects/7?board=3&task=42",` +
	`"type":"card_created",` +
	`"actorId":144,` +
	`"projectId":7,` +
	`"boardId":3,` +
	`"cardId":42,` +
	`"recipients":[187,1745],` +
	`"data":{"boardTitle":"Спринт 12"}` +
	`}`

func TestParseKanbanPayload(t *testing.T) {
	var evt events.NotificationEvent
	if err := json.Unmarshal([]byte(kanbanPayload), &evt); err != nil {
		t.Fatalf("тело продюсера не разбирается: %v", err)
	}

	if err := validateEvent(evt); err != nil {
		t.Fatalf("тело продюсера не проходит валидацию: %v", err)
	}

	if evt.EventID != "018f0c3e-7a11-7c9d-9f2a-4d5b6e7f8a90" {
		t.Errorf("eventId: получили %q", evt.EventID)
	}
	if evt.Title != "Новая задача «Свет в холле» на доске «Спринт 12»" {
		t.Errorf("title: получили %q", evt.Title)
	}
	if evt.TypeLabel != "Создана задача" {
		t.Errorf("typeLabel: получили %q", evt.TypeLabel)
	}
	if len(evt.Recipients) != 2 || evt.Recipients[0] != 187 || evt.Recipients[1] != 1745 {
		t.Errorf("recipients: получили %v", evt.Recipients)
	}

	// Экранированный на проводе `&` обязан вернуться обычным `&`,
	// иначе ссылка в колокольчике ведёт не туда.
	if evt.Link == nil {
		t.Fatal("link потерялся при разборе")
	}
	if want := "/projects/7?board=3&task=42"; *evt.Link != want {
		t.Errorf("link: получили %q, ожидали %q", *evt.Link, want)
	}
	if got := normalizeLink(evt.Link); got == nil || *got != "/projects/7?board=3&task=42" {
		t.Errorf("нормализация ссылки испортила маршрут: %v", got)
	}

	// Модуль и событие берутся из routing key, а не из тела: поле "type" в
	// теле — хвост прежнего формата, и на тип в базе оно влиять не должно.
	typ, err := typeFromRoutingKey("kanban.notification.card_created")
	if err != nil {
		t.Fatalf("routing key канбана не разбирается: %v", err)
	}
	if typ != "KANBAN_CARD_CREATED" {
		t.Errorf("type: получили %q, ожидали KANBAN_CARD_CREATED", typ)
	}
}
