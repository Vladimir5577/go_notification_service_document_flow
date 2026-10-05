package service

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/SherClockHolmes/webpush-go"

	"notification_service_document_flow/internal/model"
	"notification_service_document_flow/internal/repository"
)

const pushTimeout = 15 * time.Second

// Pusher шлёт Web Push на подписки браузера. nil — ключи VAPID не заданы,
// Push ничего не делает: колокольчик живёт как раньше, на пингах и поллинге.
type Pusher struct {
	repo       *repository.NotificationRepository
	publicKey  string
	privateKey string
	subscriber string
}

func NewPusher(repo *repository.NotificationRepository, publicKey, privateKey, subscriber string) *Pusher {
	publicKey = strings.TrimSpace(publicKey)
	privateKey = strings.TrimSpace(privateKey)
	if publicKey == "" || privateKey == "" {
		slog.Warn("VAPID_PUBLIC_KEY или VAPID_PRIVATE_KEY не заданы, push-уведомления отключены")
		return nil
	}
	if strings.TrimSpace(subscriber) == "" {
		subscriber = "mailto:notifications@localhost"
	}
	return &Pusher{
		repo:       repo,
		publicKey:  publicKey,
		privateKey: privateKey,
		subscriber: subscriber,
	}
}

// Push уходит в фоне: консьюмер не ждёт браузерные сервисы Google и Mozilla.
// Уведомление уже в базе, ошибка доставки — только лог.
func (p *Pusher) Push(n *model.Notification) {
	if p == nil || n == nil {
		return
	}

	userID := n.UserID
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), pushTimeout)
		defer cancel()

		payload, err := json.Marshal(p.payload(ctx, n))
		if err != nil {
			return
		}
		subs, err := p.repo.ListPushSubscriptions(ctx, userID)
		if err != nil {
			slog.Warn("не удалось прочитать push-подписки", "user_id", userID, "error", err)
			return
		}
		for _, sub := range subs {
			p.send(ctx, sub, payload)
		}
	}()
}

func (p *Pusher) payload(ctx context.Context, n *model.Notification) map[string]any {
	count, err := p.repo.CountUnreadForUser(ctx, n.UserID)
	if err != nil || count < 1 {
		count = 1
	}
	if count > 1 {
		return map[string]any{
			"title": "Новые уведомления в портале",
			"link":  "/notifications",
			"count": count,
		}
	}

	body := ""
	if n.Message != nil {
		body = *n.Message
	}
	link := ""
	if n.Link != nil {
		link = *n.Link
	}
	return map[string]any{
		"title": n.Title,
		"body":  body,
		"link":  link,
		"id":    n.ID,
		"count": count,
	}
}

func (p *Pusher) send(ctx context.Context, sub repository.PushSubscription, payload []byte) {
	resp, err := webpush.SendNotificationWithContext(ctx, payload, &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys: webpush.Keys{
			P256dh: sub.P256dh,
			Auth:   sub.Auth,
		},
	}, &webpush.Options{
		Subscriber:      p.subscriber,
		VAPIDPublicKey:  p.publicKey,
		VAPIDPrivateKey: p.privateKey,
		TTL:             7 * 24 * 60 * 60,
		Urgency:         webpush.UrgencyHigh,
		// Одна тема на подписку: пока браузер спит, сервис доставки хранит только последний пуш.
		Topic: "portal",
	})
	if err != nil {
		slog.Warn("не удалось отправить push", "error", err)
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	// Браузер отозвал подписку. Повтор бессмысленен.
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		if err := p.repo.DeletePushSubscriptionByEndpoint(context.Background(), sub.Endpoint); err != nil {
			slog.Warn("не удалось удалить протухшую push-подписку", "error", err)
		}
		return
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		slog.Warn("push-сервис отклонил уведомление", "status", resp.StatusCode)
	}
}
