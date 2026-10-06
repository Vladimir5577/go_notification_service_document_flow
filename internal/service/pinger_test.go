package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

func TestPingerPublish(t *testing.T) {
	type hit struct {
		form url.Values
		auth string
	}
	hits := make(chan hit, 1)
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		hits <- hit{form: r.PostForm, auth: r.Header.Get("Authorization")}
	}))
	defer hub.Close()

	p := NewPinger(hub.URL, "secret")
	if err := p.publish(context.Background(), []int64{7, 42}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	got := <-hits

	// Один запрос на всех получателей, по топику на каждого.
	if want := []string{"/notifications/user/7", "/notifications/user/42"}; !slices.Equal(got.form["topic"], want) {
		t.Errorf("topic = %v, ожидали %v", got.form["topic"], want)
	}
	// Пустой data EventSource не доставит, и пинг потеряется молча.
	if got.form.Get("data") != `{"ping":true}` {
		t.Errorf("data = %q", got.form.Get("data"))
	}
	// Хаб примет только токен, подписанный его секретом и с правом публикации.
	claims := jwt.MapClaims{}
	if _, err := jwt.ParseWithClaims(strings.TrimPrefix(got.auth, "Bearer "), claims,
		func(*jwt.Token) (any, error) { return []byte("secret"), nil }); err != nil {
		t.Fatalf("токен не проходит проверку секретом хаба: %v", err)
	}
	if _, ok := claims["mercure"].(map[string]any)["publish"]; !ok {
		t.Errorf("в токене нет mercure.publish: %v", claims)
	}

	// Хаб отказал — ошибка, а не тихий успех.
	deny := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	}))
	defer deny.Close()
	if err := NewPinger(deny.URL, "secret").publish(context.Background(), []int64{1}); err == nil {
		t.Error("401 от хаба: ожидали ошибку")
	}

	// Без настроек пингер выключен, и вызывать его всё равно безопасно.
	if NewPinger("", "secret") != nil || NewPinger(hub.URL, "") != nil {
		t.Error("без URL или секрета пингер должен быть nil")
	}
	var off *Pinger
	off.Ping(1)
}
