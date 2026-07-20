package events

// PurchaseNotificationEvent — событие уведомления модуля закупок.
// Публикуется Symfony (App\Message\PurchaseNotificationMessage) в topic
// exchange 'events' с routing key purchase.notification.{type}.
//
// На каждого получателя из Recipients создаётся отдельная запись notification.
type PurchaseNotificationEvent struct {
	// Тип события: submitted | approved | rejected | taken |
	// status_changed | delivered | confirmed | cancelled | comment_added
	Type string `json:"type"`

	// Кто совершил действие.
	ActorID int64 `json:"actorId"`

	// ID заявки на закупку (PurchaseRequest.id в Symfony).
	PurchaseID int64 `json:"purchaseId"`

	// Получатели уведомления.
	Recipients []int64 `json:"recipients"`

	// Данные для построения текста: purchaseTitle, actorName, status,
	// statusLabel, link, comment, resubmitted.
	Data map[string]any `json:"data"`
}

// Routing keys purchase-событий (публикует Symfony, справочно).
const (
	RoutingPurchaseNotificationSubmitted     = "purchase.notification.submitted"
	RoutingPurchaseNotificationApproved      = "purchase.notification.approved"
	RoutingPurchaseNotificationRejected      = "purchase.notification.rejected"
	RoutingPurchaseNotificationTaken         = "purchase.notification.taken"
	RoutingPurchaseNotificationStatusChanged = "purchase.notification.status_changed"
	RoutingPurchaseNotificationDelivered     = "purchase.notification.delivered"
	RoutingPurchaseNotificationConfirmed     = "purchase.notification.confirmed"
	RoutingPurchaseNotificationCancelled     = "purchase.notification.cancelled"
	RoutingPurchaseNotificationCommentAdded  = "purchase.notification.comment_added"
)
