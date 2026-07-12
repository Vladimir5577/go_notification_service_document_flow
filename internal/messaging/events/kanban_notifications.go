package events

// KanbanNotificationEvent is the payload published to RabbitMQ when a Kanban
// action that should generate a user notification occurs.
//
// The notification service consumes these events
// (routing keys starting with "kanban.notification.") and creates records
// in its own database.
type KanbanNotificationEvent struct {
	Type       string         `json:"type"`
	ActorID    int64          `json:"actorId"`
	ProjectID  int64          `json:"projectId"`
	BoardID    *int64         `json:"boardId,omitempty"`
	CardID     *int64         `json:"cardId,omitempty"`
	Recipients []int64        `json:"recipients"`
	Data       map[string]any `json:"data"`
}

// Routing key constants (must match what Kanban publishes)
const (
	RoutingKanbanNotificationCardCreated     = "kanban.notification.card_created"
	RoutingKanbanNotificationTaskAssigned    = "kanban.notification.task_assigned"
	RoutingKanbanNotificationTaskMoved       = "kanban.notification.task_moved"
	RoutingKanbanNotificationCommentAdded    = "kanban.notification.comment_added"
	RoutingKanbanNotificationSubtaskAssigned = "kanban.notification.subtask_assigned"
	RoutingKanbanNotificationProjectUserAdded   = "kanban.notification.project_user_added"
	RoutingKanbanNotificationProjectUserRemoved = "kanban.notification.project_user_removed"
)
