package dto

import (
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
	createdAt := n.CreatedAt.Format("2006-01-02T15:04:05")
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
	case "PURCHASE_SUBMITTED":
		return "Заявка на закупку"
	case "PURCHASE_APPROVED":
		return "Закупка согласована"
	case "PURCHASE_REJECTED":
		return "Закупка возвращена на доработку"
	case "PURCHASE_TAKEN":
		return "Закупка взята в работу"
	case "PURCHASE_STATUS_CHANGED":
		return "Статус закупки изменён"
	case "PURCHASE_DELIVERED":
		return "Закупка доставлена"
	case "PURCHASE_CONFIRMED":
		return "Получение подтверждено"
	case "PURCHASE_CANCELLED":
		return "Закупка отменена"
	case "PURCHASE_COMMENT_ADDED":
		return "Новый комментарий к закупке"
	default:
		return "Уведомление"
	}
}
