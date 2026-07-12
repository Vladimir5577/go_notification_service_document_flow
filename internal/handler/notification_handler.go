package handler

import (
	"net/http"
	"strconv"

	"notification_service_document_flow/internal/apperr"
	"notification_service_document_flow/internal/dto"
	"notification_service_document_flow/internal/helper"
	"notification_service_document_flow/internal/middleware"
	"notification_service_document_flow/internal/repository"
)

const (
	defaultPageSize = 20
	maxPageSize     = 100
	defaultLimit    = 15
	unreadCap       = 100
)

type NotificationHandler struct {
	repo *repository.NotificationRepository
}

func NewNotificationHandler(repo *repository.NotificationRepository) *NotificationHandler {
	return &NotificationHandler{repo: repo}
}

func (h *NotificationHandler) List(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUser(r.Context())
	if !ok {
		helper.WriteError(w, apperr.ErrUnauthorized)
		return
	}

	page := 1
	if p := r.URL.Query().Get("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}

	pageSize := defaultPageSize
	if ps := r.URL.Query().Get("page_size"); ps != "" {
		if v, err := strconv.Atoi(ps); err == nil {
			if v > 0 && v <= maxPageSize {
				pageSize = v
			}
		}
	}

	notifications, err := h.repo.FindAllForUser(r.Context(), user.ID, page, pageSize)
	if err != nil {
		helper.WriteError(w, err)
		return
	}

	total, _ := h.repo.CountAllForUser(r.Context(), user.ID)
	unread, _ := h.repo.CountUnreadForUser(r.Context(), user.ID)

	items := make([]dto.NotificationResponse, len(notifications))
	for i, n := range notifications {
		items[i] = dto.ToNotificationResponse(n)
	}

	resp := dto.ListResponse{
		Items:                items,
		Page:                 page,
		PageSize:             pageSize,
		Total:                total,
		UnreadCount:          unread,
		UnreadDocumentsCount: 0, // TODO: implement when document events arrive
	}

	helper.WriteJSON(w, http.StatusOK, resp)
}

func (h *NotificationHandler) Latest(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUser(r.Context())
	if !ok {
		helper.WriteError(w, apperr.ErrUnauthorized)
		return
	}

	limit := defaultLimit
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= maxPageSize {
			limit = v
		}
	}

	notifications, err := h.repo.FindLatestForUser(r.Context(), user.ID, limit)
	if err != nil {
		helper.WriteError(w, err)
		return
	}

	unread, _ := h.repo.CountUnreadForUser(r.Context(), user.ID)

	items := make([]dto.NotificationResponse, len(notifications))
	for i, n := range notifications {
		items[i] = dto.ToNotificationResponse(n)
	}

	resp := dto.LatestResponse{
		UnreadCount:          unread,
		UnreadDocumentsCount: 0,
		Notifications:        items,
	}

	helper.WriteJSON(w, http.StatusOK, resp)
}

func (h *NotificationHandler) MarkAsRead(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUser(r.Context())
	if !ok {
		helper.WriteError(w, apperr.ErrUnauthorized)
		return
	}

	id, err := helper.IDParam(r, "id")
	if err != nil {
		helper.WriteError(w, err)
		return
	}

	if err := h.repo.MarkAsRead(r.Context(), id, user.ID); err != nil {
		helper.WriteError(w, err)
		return
	}

	helper.WriteJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (h *NotificationHandler) MarkAllAsRead(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUser(r.Context())
	if !ok {
		helper.WriteError(w, apperr.ErrUnauthorized)
		return
	}

	if err := h.repo.MarkAllAsReadForUser(r.Context(), user.ID); err != nil {
		helper.WriteError(w, err)
		return
	}

	helper.WriteJSON(w, http.StatusOK, map[string]bool{"success": true})
}
