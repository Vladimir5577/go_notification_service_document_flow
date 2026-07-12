package model

import (
	"encoding/json"
	"time"
)

// Notification — доменная модель уведомления.
type Notification struct {
	ID        int64           `json:"id"`
	Type      string          `json:"type"`
	Title     string          `json:"title"`
	Message   *string         `json:"message"`
	Link      *string         `json:"link"`
	CreatedAt time.Time       `json:"created_at"`
	ReadAt    *time.Time      `json:"read_at"`
	Extra     json.RawMessage `json:"extra,omitempty"`
	UserID    int64           `json:"user_id"`
}

// IsRead возвращает true, если уведомление прочитано.
func (n *Notification) IsRead() bool {
	return n.ReadAt != nil
}
