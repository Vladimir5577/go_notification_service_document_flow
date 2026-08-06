package dto

import (
	"time"

	"notification_service_document_flow/internal/model"
)

type NotificationResponse struct {
	ID        int64   `json:"id"`
	Type      string  `json:"type"`
	TypeLabel string  `json:"typeLabel"`
	Title     string  `json:"title"`
	Message   *string `json:"message"`
	Link      *string `json:"link"`
	IsRead    bool    `json:"isRead"`
	CreatedAt string  `json:"createdAt"`
}

type ListResponse struct {
	Items       []NotificationResponse `json:"items"`
	Page        int                    `json:"page"`
	PageSize    int                    `json:"pageSize"`
	Total       int                    `json:"total"`
	UnreadCount int                    `json:"unreadCount"`
}

type LatestResponse struct {
	UnreadCount   int                    `json:"unreadCount"`
	Notifications []NotificationResponse `json:"notifications"`
}

func ToNotificationResponse(n model.Notification) NotificationResponse {
	// Use RFC3339 so it serializes as ...Z (UTC)
	createdAt := n.CreatedAt.UTC().Format(time.RFC3339)
	isRead := n.ReadAt != nil

	// TypeLabel приходит от продюсера вместе с текстом и лежит в строке.
	// Раньше здесь стоял switch по типам — по case на каждое событие каждого
	// модуля; строкам, созданным до перехода, подписи проставила миграция
	// 00002 по тому же словарю.
	return NotificationResponse{
		ID:        n.ID,
		Type:      n.Type,
		TypeLabel: n.TypeLabel,
		Title:     n.Title,
		Message:   n.Message,
		Link:      n.Link,
		IsRead:    isRead,
		CreatedAt: createdAt,
	}
}
