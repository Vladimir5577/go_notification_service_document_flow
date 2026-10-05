package handler

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"notification_service_document_flow/internal/apperr"
	"notification_service_document_flow/internal/helper"
	"notification_service_document_flow/internal/middleware"
	"notification_service_document_flow/internal/repository"
)

func (h *NotificationHandler) PushPublicKey(w http.ResponseWriter, r *http.Request) {
	if _, ok := middleware.GetUser(r.Context()); !ok {
		helper.WriteError(w, apperr.ErrUnauthorized)
		return
	}
	if h.vapidPublic == "" {
		helper.WriteError(w, apperr.ErrNotFound)
		return
	}
	helper.WriteJSON(w, http.StatusOK, map[string]string{"publicKey": h.vapidPublic})
}

func (h *NotificationHandler) SavePushSubscription(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUser(r.Context())
	if !ok {
		helper.WriteError(w, apperr.ErrUnauthorized)
		return
	}

	sub, err := decodePushSubscription(w, r)
	if err != nil {
		helper.WriteError(w, err)
		return
	}
	if err := h.repo.UpsertPushSubscription(r.Context(), user.ID, sub); err != nil {
		helper.WriteError(w, err)
		return
	}
	helper.WriteJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (h *NotificationHandler) DeletePushSubscription(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUser(r.Context())
	if !ok {
		helper.WriteError(w, apperr.ErrUnauthorized)
		return
	}

	var body struct {
		Endpoint string `json:"endpoint"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		helper.WriteError(w, apperr.ErrValidation)
		return
	}
	endpoint := strings.TrimSpace(body.Endpoint)
	if !validPushEndpoint(endpoint) {
		helper.WriteError(w, apperr.ErrValidation)
		return
	}
	if err := h.repo.DeletePushSubscription(r.Context(), user.ID, endpoint); err != nil {
		helper.WriteError(w, err)
		return
	}
	helper.WriteJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func decodePushSubscription(w http.ResponseWriter, r *http.Request) (repository.PushSubscription, error) {
	var body struct {
		Endpoint string `json:"endpoint"`
		P256dh   string `json:"p256dh"`
		Auth     string `json:"auth"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&body); err != nil {
		return repository.PushSubscription{}, apperr.ErrValidation
	}
	sub := repository.PushSubscription{
		Endpoint: strings.TrimSpace(body.Endpoint),
		P256dh:   strings.TrimSpace(body.P256dh),
		Auth:     strings.TrimSpace(body.Auth),
	}
	if !validPushEndpoint(sub.Endpoint) || !validPushKey(sub.P256dh) || !validPushKey(sub.Auth) {
		return repository.PushSubscription{}, apperr.ErrValidation
	}
	return sub, nil
}

func validPushEndpoint(raw string) bool {
	if raw == "" || utf8.RuneCountInString(raw) > 2048 {
		return false
	}
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host != ""
}

func validPushKey(raw string) bool {
	if raw == "" || utf8.RuneCountInString(raw) > 200 {
		return false
	}
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '=':
		default:
			return false
		}
	}
	return true
}
