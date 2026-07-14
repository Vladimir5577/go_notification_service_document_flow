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
	Items              []NotificationResponse `json:"items"`
	Page               int                    `json:"page"`
	PageSize           int                    `json:"pageSize"`
	Total              int                    `json:"total"`
	UnreadCount        int                    `json:"unreadCount"`
	UnreadDocumentsCount int                  `json:"unreadDocumentsCount"`
}

type LatestResponse struct {
	UnreadCount          int                    `json:"unreadCount"`
	UnreadDocumentsCount int                    `json:"unreadDocumentsCount"`
	Notifications        []NotificationResponse `json:"notifications"`
}

func ToNotificationResponse(n model.Notification) NotificationResponse {
	// Use RFC3339 so it serializes as ...Z (UTC)
	createdAt := n.CreatedAt.UTC().Format(time.RFC3339)
	isRead := n.ReadAt != nil

	return NotificationResponse{
		ID:        n.ID,
		Type:      n.Type,
		TypeLabel: mapTypeToLabel(n.Type),
		Title:     n.Title,
		Message:   n.Message,
		Link:      n.Link,
		IsRead:    isRead,
		CreatedAt: createdAt,
	}
}

func mapTypeToLabel(typ string) string {
	switch typ {
	case "DOCUMENT_SENT":
		return "Документ отправлен"
	case "NEW_INCOMING_DOCUMENT":
		return "Новый входящий документ"
	case "KANBAN_TASK_ASSIGNED_TO_USER", "TASK_ASSIGNED":
		return "Назначена задача"
	case "KANBAN_CARD_CREATED":
		return "Создана задача"
	case "USER_ADDED_TO_KANBAN_PROJECT":
		return "Добавлен в проект"
	case "USER_REMOVED_FROM_KANBAN_PROJECT":
		return "Исключён из проекта"
	case "TASK_MOVED":
		return "Задача перемещена"
	case "TASK_COMMENT_ADDED":
		return "Новый комментарий в задаче"
	case "DOCUMENT_COMMENT_ADDED":
		return "Новый комментарий к документу"
	default:
		return "Уведомление"
	}
}
