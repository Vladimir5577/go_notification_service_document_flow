package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"notification_service_document_flow/internal/config"
	"notification_service_document_flow/internal/handler"
	"notification_service_document_flow/internal/messaging/notifications"
	"notification_service_document_flow/internal/middleware"
	"notification_service_document_flow/internal/repository"
	"notification_service_document_flow/internal/service"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type App struct {
	router               *chi.Mux
	cfg                  *config.Config
	notificationConsumer *notifications.Consumer
}

func NewApp(cfg *config.Config, db *pgxpool.Pool) (*App, error) {
	authMw, err := middleware.NewAuthMiddleware(cfg.JWTPublicKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to init auth middleware: %w", err)
	}

	notificationRepo := repository.NewNotificationRepository(db)

	notificationHandler := handler.NewNotificationHandler(notificationRepo)

	notificationSvc := service.NewNotificationService(notificationRepo, cfg.Clock)
	notifConsumer := notifications.NewConsumer(cfg, notificationSvc)

	r := setupRouter(notificationHandler, authMw)

	return &App{
		router:               r,
		cfg:                  cfg,
		notificationConsumer: notifConsumer,
	}, nil
}

func (a *App) Run() error {
	addr := fmt.Sprintf(":%s", a.cfg.Port)
	appCtx, stopBackground := context.WithCancel(context.Background())
	var backgroundWG sync.WaitGroup

	// Consumer уведомлений: одна очередь на все модули-источники (*.notification.#)
	if a.notificationConsumer != nil {
		backgroundWG.Add(1)
		go func() {
			defer backgroundWG.Done()
			if err := a.notificationConsumer.Run(appCtx); err != nil {
				slog.Warn("Notification consumer stopped", "error", err)
			}
		}()
	}

	srv := &http.Server{
		Addr:         addr,
		Handler:      a.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 20 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		slog.Info("Notification service starting", "port", a.cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Ошибка старта HTTP-сервера", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Shutting down server...")

	stopBackground()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("Server forced to shutdown", "error", err)
	}

	backgroundWG.Wait()
	slog.Info("Server exited properly")
	return nil
}
