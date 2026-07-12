package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"notification_service_document_flow/internal/messaging/events"
	"notification_service_document_flow/internal/model"
	"notification_service_document_flow/internal/repository"
)

// NotificationService handles creation of notifications from events.
type NotificationService struct {
	repo *repository.NotificationRepository
}

func NewNotificationService(repo *repository.NotificationRepository) *NotificationService {
	return &NotificationService{repo: repo}
}

// CreateFromKanbanEvent creates notifications for each recipient based on the event.
// Titles/messages are built here to match Symfony's NotificationService logic.
//
// Returns an error if any notification failed to be created (consumer will requeue).
func (s *NotificationService) CreateFromKanbanEvent(ctx context.Context, evt events.KanbanNotificationEvent) error {
	if len(evt.Recipients) == 0 {
		return nil
	}

	var firstErr error

	for _, recipientID := range evt.Recipients {
		title, message, link := s.buildTitleMessageLink(evt)

		notification := &model.Notification{
			Type:      mapEventTypeToDB(evt.Type),
			Title:     title,
			Message:   message,
			Link:      link,
			CreatedAt: time.Now().UTC(),
			ReadAt:    nil,
			UserID:    recipientID,
		}

		// Store extra from event data if present (for future use / frontend)
		if len(evt.Data) > 0 {
			if extraBytes, err := json.Marshal(evt.Data); err == nil {
				notification.Extra = extraBytes
			}
		}

		if _, err := s.repo.Create(ctx, notification); err != nil {
			slog.Error("Failed to create notification", "user_id", recipientID, "type", evt.Type, "error", err)
			if firstErr == nil {
				firstErr = err
			}
		} else {
			slog.Info("Notification created", "user_id", recipientID, "type", evt.Type, "title", title)
		}
	}

	return firstErr
}

func (s *NotificationService) buildTitleMessageLink(evt events.KanbanNotificationEvent) (string, *string, *string) {
	data := evt.Data
	authorName := getString(data, "authorName", "")
	taskTitle := getString(data, "taskTitle", getString(data, "cardTitle", ""))
	boardTitle := getString(data, "boardTitle", "")
	fromColumn := getString(data, "fromColumnTitle", getString(data, "fromColumn", ""))
	toColumn := getString(data, "toColumnTitle", getString(data, "toColumn", ""))
	projectName := getString(data, "projectName", "")

	var title string
	var msg *string

	switch evt.Type {
	case "card_created":
		title = fmt.Sprintf("Новая задача «%s» на доске «%s»", taskTitle, boardTitle)
	case "task_assigned":
		if isSubtask(data) {
			title = fmt.Sprintf("Вам назначена подзадача: %s", taskTitle)
		} else {
			title = fmt.Sprintf("Вам назначена задача: %s", taskTitle)
		}
	case "task_moved":
		title = fmt.Sprintf("%s переместил задачу %s из колонки «%s» в колонку «%s»",
			authorName, taskTitle, fromColumn, toColumn)
	case "comment_added":
		title = fmt.Sprintf("%s оставил комментарий к задаче %s", authorName, taskTitle)
	case "subtask_assigned":
		title = fmt.Sprintf("Вам назначена подзадача: %s", taskTitle)
	case "project_user_added":
		title = fmt.Sprintf("Вас добавили в проект «%s»", projectName)
	case "project_user_removed":
		title = fmt.Sprintf("Вас исключили из проекта «%s»", projectName)
	default:
		title = getString(data, "title", "Уведомление")
	}

	linkStr := getString(data, "link", "")
	if linkStr != "" {
		return title, msg, normalizeLink(linkStr)
	}
	return title, msg, nil
}

func mapEventTypeToDB(eventType string) string {
	switch eventType {
	case "card_created":
		return "KANBAN_CARD_CREATED"
	case "task_assigned":
		return "KANBAN_TASK_ASSIGNED_TO_USER"
	case "task_moved":
		return "TASK_MOVED"
	case "comment_added":
		return "TASK_COMMENT_ADDED"
	case "subtask_assigned":
		return "KANBAN_TASK_ASSIGNED_TO_USER"
	case "project_user_added":
		return "USER_ADDED_TO_KANBAN_PROJECT"
	case "project_user_removed":
		return "USER_REMOVED_FROM_KANBAN_PROJECT"
	default:
		return strings.ToUpper(eventType)
	}
}

func normalizeLink(raw string) *string {
	if raw == "" || raw == "#" {
		return nil
	}

	// Document links (legacy Symfony -> SPA)
	if link := mapDocumentViewLink(raw, "/view_incoming_document", "/document-in"); link != nil {
		return link
	}
	if link := mapDocumentViewLink(raw, "/view_outgoing_document", "/document-out"); link != nil {
		return link
	}

	// Kanban project legacy link
	if strings.HasPrefix(raw, "/kanban_project/") {
		idStr := strings.TrimPrefix(raw, "/kanban_project/")
		if id, err := strconv.Atoi(idStr); err == nil && id > 0 {
			s := fmt.Sprintf("/projects/%d/edit", id)
			return &s
		}
		return &raw
	}

	// Kanban board links (support both /kanban/board/ and legacy /kanban_board/)
	if strings.Contains(raw, "/kanban/board/") || strings.Contains(raw, "/kanban_board/") {
		// Extract board id
		boardID := 0
		if idx := strings.LastIndex(raw, "/board/"); idx != -1 {
			if id, err := strconv.Atoi(raw[idx+7:]); err == nil {
				boardID = id
			}
		} else if idx := strings.LastIndex(raw, "/kanban_board/"); idx != -1 {
			if id, err := strconv.Atoi(raw[idx+14:]); err == nil {
				boardID = id
			}
		}

		if boardID > 0 {
			// We don't have easy access to project ID here without DB lookup.
			// Modern events from Kanban service usually send good /projects/... links already.
			// Keep board param for SPA.
			s := fmt.Sprintf("/projects?board=%d", boardID)

			// Preserve card if present in query or fragment
			if u, err := url.Parse(raw); err == nil {
				if card := u.Query().Get("card"); card != "" {
					s += "&card=" + card
				}
				if u.Fragment != "" {
					s += "#" + u.Fragment
				}
			}
			return &s
		}
	}

	// General kanban prefix cleanup (fallback)
	if strings.Contains(raw, "/kanban/") {
		normalized := strings.Replace(raw, "/kanban/", "/projects/", 1)
		return &normalized
	}

	return &raw
}

func mapDocumentViewLink(raw, legacySegment, spaBase string) *string {
	if !strings.Contains(raw, legacySegment) {
		return nil
	}

	parts := strings.Split(raw, legacySegment+"/")
	if len(parts) < 2 {
		return nil
	}

	docPart := parts[1]
	docID := strings.Split(docPart, "?")[0]
	docID = strings.Split(docID, "#")[0]

	if docID == "" {
		return nil
	}

	result := spaBase + "?doc=" + docID

	// preserve fragment if any
	if idx := strings.Index(raw, "#"); idx != -1 {
		result += raw[idx:]
	}

	return &result
}

func getString(m map[string]any, key string, fallback string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return fallback
}

func isSubtask(data map[string]any) bool {
	if v, ok := data["isSubtask"]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}
