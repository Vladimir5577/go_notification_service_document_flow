package events

import (
	"encoding/json"
	"testing"
)

// Пустой ассоциативный массив PHP уезжает на провод как `[]`, а не `{}`.
// Именно на этом теле консьюмер ронял уведомления документооборота.
func TestPayloadUnmarshalTolerantToPHPEmptyArray(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		wantLen int
		wantErr bool
	}{
		{name: "пустой массив от PHP", raw: `[]`, wantLen: 0},
		{name: "пустой объект", raw: `{}`, wantLen: 0},
		{name: "null", raw: `null`, wantLen: 0},
		{name: "обычные данные", raw: `{"documentId":139}`, wantLen: 1},
		{name: "непустой массив — всё ещё ошибка", raw: `[1,2]`, wantErr: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var p Payload
			err := json.Unmarshal([]byte(c.raw), &p)

			if c.wantErr {
				if err == nil {
					t.Fatalf("ожидали ошибку на %s, её нет", c.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("неожиданная ошибка на %s: %v", c.raw, err)
			}
			if len(p) != c.wantLen {
				t.Fatalf("на %s ожидали %d ключей, получили %d", c.raw, c.wantLen, len(p))
			}
		})
	}
}

// Настоящее тело события из лога падения — целиком, как пришло из монолита.
func TestNotificationEventFromProductionBody(t *testing.T) {
	const body = `{"eventId":"019fdc15-21ba-7783-a8b3-6c4b12683186","recipients":[187],` +
		`"title":"Новый входящий документ: Test","typeLabel":"Новый входящий документ",` +
		`"message":null,"link":"/document-in?doc=139","actorId":0,"data":[]}`

	var evt NotificationEvent
	if err := json.Unmarshal([]byte(body), &evt); err != nil {
		t.Fatalf("разбор реального события упал: %v", err)
	}
	if len(evt.Recipients) != 1 || evt.Recipients[0] != 187 {
		t.Fatalf("получатели разобраны неверно: %v", evt.Recipients)
	}
	if len(evt.Data) != 0 {
		t.Fatalf("data должна быть пустой, получили %v", evt.Data)
	}
}
