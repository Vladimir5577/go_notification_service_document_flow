package model

import (
	"encoding/json"
	"time"
)

// Notification — доменная модель уведомления.
type Notification struct {
	ID int64 `json:"id"`
	// EventID — id исходного события от продюсера, общий на всех его получателей.
	// Пустой у строк, созданных до перехода на общий контракт.
	EventID string `json:"event_id,omitempty"`
	Type    string `json:"type"`
	// TypeLabel — подпись категории для списка уведомлений, приходит от продюсера.
	TypeLabel string          `json:"type_label,omitempty"`
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
