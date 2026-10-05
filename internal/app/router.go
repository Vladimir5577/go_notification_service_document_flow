package app

import (
	"net/http"
	"time"

	"notification_service_document_flow/internal/handler"
	"notification_service_document_flow/internal/middleware"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
)

func setupRouter(notificationHandler *handler.NotificationHandler, authMw *middleware.AuthMiddleware) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.RequestLogger())
	r.Use(chiMiddleware.Recoverer)
	r.Use(chiMiddleware.Timeout(15 * time.Second))

	// Health check
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status": "ok", "service": "notification"}`))
	})

	// Protected routes
	r.Group(func(r chi.Router) {
		r.Use(authMw.Handler)

		// SPA Notifications API (matches frontend expectations)
		r.Route("/spa/api/notifications", func(r chi.Router) {
			r.Get("/", notificationHandler.List)
			r.Get("/latest", notificationHandler.Latest)
			r.Get("/push/public-key", notificationHandler.PushPublicKey)
			r.Put("/push", notificationHandler.SavePushSubscription)
			r.Delete("/push", notificationHandler.DeletePushSubscription)
			r.Post("/read-all", notificationHandler.MarkAllAsRead)
			r.Post("/{id}/read", notificationHandler.MarkAsRead)
		})
	})

	return r
}
