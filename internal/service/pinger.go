package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const pingTimeout = 5 * time.Second

// Pinger шлёт в Mercure сигнал «обнови колокольчик» в топики
// /notifications/user/{id}. Фронт по нему сам перечитывает /latest со своим JWT.
//
// Содержимого в пинге нет намеренно: хаб запущен с anonymous, подписаться на
// чужой топик (или сразу на `*`) может кто угодно. Текст уведомлений через хаб
// не ходит, наружу видно только «кому-то что-то пришло».
type Pinger struct {
	hubURL string
	token  string
}

// NewPinger возвращает nil, если хаб не настроен. Ping у nil — пустая операция:
// колокольчик тогда живёт на поллинге фронта.
func NewPinger(hubURL, jwtSecret string) *Pinger {
	hubURL = strings.TrimSpace(hubURL)
	if hubURL == "" || jwtSecret == "" {
		slog.Warn("MERCURE_URL или MERCURE_JWT_SECRET не заданы, realtime-пинги колокольчика отключены")
		return nil
	}

	// Токен без exp, как у канбана, поэтому подписываем один раз, а не на каждый пинг.
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"mercure": map[string]any{"publish": []string{"*"}},
	}).SignedString([]byte(jwtSecret))
	if err != nil {
		slog.Error("не удалось подписать JWT для Mercure, realtime-пинги колокольчика отключены", "error", err)
		return nil
	}

	return &Pinger{hubURL: hubURL, token: token}
}

// Ping уходит в фоне: ни консьюмер, ни HTTP-ответ хаб не ждут. Ошибка — только
// в лог: уведомление уже в базе, а фронт догонит поллингом.
func (p *Pinger) Ping(userIDs ...int64) {
	if p == nil || len(userIDs) == 0 {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
		defer cancel()

		if err := p.publish(ctx, userIDs); err != nil {
			slog.Warn("не удалось отправить пинг колокольчика в Mercure",
				"user_ids", userIDs, "error", err)
		}
	}()
}

// publish отправляет одно обновление на всех получателей: Mercure доставит его
// подписчику любого из топиков. Список топиков в SSE-событие не попадает, так
// что получатели друг друга не видят.
func (p *Pinger) publish(ctx context.Context, userIDs []int64) error {
	// Пустой data EventSource молча не доставляет. Текста уведомления здесь нет:
	// хаб anonymous, подписаться на чужой топик может кто угодно.
	form := url.Values{"data": {`{"ping":true}`}}
	for _, id := range userIDs {
		form.Add("topic", "/notifications/user/"+strconv.FormatInt(id, 10))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.hubURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+p.token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("status %d: %s", resp.StatusCode, bytes.TrimSpace(body))
	}

	return nil
}
