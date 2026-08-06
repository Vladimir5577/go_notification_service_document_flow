package service

import (
	"testing"

	"notification_service_document_flow/internal/messaging/events"
)

func TestTypeFromRoutingKey(t *testing.T) {
	ok := []struct {
		key  string
		want string
	}{
		{"purchase.notification.submitted", "PURCHASE_SUBMITTED"},
		{"kanban.notification.card_created", "KANBAN_CARD_CREATED"},
		// Составное событие: ради него привязка стоит `#`, а не `*`.
		{"document.notification.status.changed", "DOCUMENT_STATUS_CHANGED"},
		{"inventory.notification.upd-uploaded", "INVENTORY_UPD_UPLOADED"},
	}
	for _, c := range ok {
		got, err := typeFromRoutingKey(c.key)
		if err != nil {
			t.Errorf("%s: неожиданная ошибка %v", c.key, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: получили %q, ожидали %q", c.key, got, c.want)
		}
	}

	bad := []string{
		"",
		"purchase.notification.",     // событие пустое
		".notification.submitted",    // модуль пустой
		"purchase.submitted",         // не тот вид ключа
		"a.b.notification.submitted", // модуль не одно слово
		"user.upserted",              // чужое событие, не уведомление
	}
	for _, key := range bad {
		if got, err := typeFromRoutingKey(key); err == nil {
			t.Errorf("%q: ожидали ошибку, получили %q", key, got)
		}
	}

	// Тип не должен молча обрезаться под VARCHAR(50): разные события
	// схлопнулись бы в один тип.
	long := "purchase.notification." + string(make([]byte, maxTypeLength))
	if _, err := typeFromRoutingKey(long); err == nil {
		t.Error("слишком длинный тип: ожидали ошибку")
	}
}

func TestValidateEvent(t *testing.T) {
	valid := newEvent()
	if err := validateEvent(valid); err != nil {
		t.Fatalf("корректное событие отклонено: %v", err)
	}

	noID := newEvent()
	noID.EventID = "  "
	if err := validateEvent(noID); err == nil {
		t.Error("пустой eventId: ожидали ошибку")
	}

	noTitle := newEvent()
	noTitle.Title = ""
	if err := validateEvent(noTitle); err == nil {
		t.Error("пустой title: ожидали ошибку")
	}

	noRecipients := newEvent()
	noRecipients.Recipients = nil
	if err := validateEvent(noRecipients); err == nil {
		t.Error("пустой recipients: ожидали ошибку")
	}
}

func TestNormalizeLink(t *testing.T) {
	keep := map[string]string{
		"/purchases/42":              "/purchases/42",
		"/projects/1?board=2&task=3": "/projects/1?board=2&task=3",
		"  /documents/7  ":           "/documents/7",
		// Абсолютный адрес своего же портала ужимаем до пути.
		"https://portal.local/projects/1?task=5": "/projects/1?task=5",
	}
	for raw, want := range keep {
		got := normalizeLink(&raw)
		if got == nil {
			t.Errorf("%q: ссылка отброшена, ожидали %q", raw, want)
			continue
		}
		if *got != want {
			t.Errorf("%q: получили %q, ожидали %q", raw, *got, want)
		}
	}

	// Внешний адрес без пути стал бы редиректом на чужой сайт из колокольчика.
	drop := []string{"", "   ", "https://evil.example", "javascript:alert(1)", "relative/path"}
	for _, raw := range drop {
		if got := normalizeLink(&raw); got != nil {
			t.Errorf("%q: ожидали nil, получили %q", raw, *got)
		}
	}

	if normalizeLink(nil) != nil {
		t.Error("nil-ссылка: ожидали nil")
	}
}

func newEvent() events.NotificationEvent {
	return events.NotificationEvent{
		EventID:    "018f0c3e-7a11-7c9d-9f2a-4d5b6e7f8a90",
		Recipients: []int64{187},
		Title:      "Новая заявка на закупку «Ноутбуки» на рассмотрении",
	}
}

// Дубли схлопываются, мусор отсеивается — но живые id не трогаются никогда.
// Прежняя версия сверялась с репликой users и молча съедала уведомления тех,
// кого реплика не успела получить: у неё нет ни начальной загрузки, ни ресинка.
func TestUniqueRecipients(t *testing.T) {
	got := uniqueRecipients([]int64{187, 1745, 187, 0, -3, 435})
	want := []int64{187, 1745, 435}

	if len(got) != len(want) {
		t.Fatalf("получили %v, ожидали %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("получили %v, ожидали %v (порядок важен: он же порядок вставки)", got, want)
		}
	}

	if r := uniqueRecipients(nil); len(r) != 0 {
		t.Errorf("пустой вход — пустой выход, получили %v", r)
	}
}
