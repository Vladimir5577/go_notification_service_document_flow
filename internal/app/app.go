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
	"notification_service_document_flow/internal/messaging/usersync"
	"notification_service_document_flow/internal/middleware"
	"notification_service_document_flow/internal/repository"
	"notification_service_document_flow/internal/service"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type App struct {
	router                  *chi.Mux
	cfg                     *config.Config
	userSyncConsumer        *usersync.Consumer
	kanbanNotificationConsumer *notifications.Consumer
}

func NewApp(cfg *config.Config, db *pgxpool.Pool) (*App, error) {
	authMw, err := middleware.NewAuthMiddleware(cfg.JWTPublicKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to init auth middleware: %w", err)
	}

	userRepo := repository.NewUserRepository(db)
	notificationRepo := repository.NewNotificationRepository(db)

	notificationHandler := handler.NewNotificationHandler(notificationRepo)

	notificationSvc := service.NewNotificationService(notificationRepo)
	kanbanNotifConsumer := notifications.NewConsumer(cfg, notificationSvc)

	r := setupRouter(notificationHandler, authMw)

	return &App{
		router:                     r,
		cfg:                        cfg,
		userSyncConsumer:           usersync.NewConsumer(cfg, userRepo),
		kanbanNotificationConsumer: kanbanNotifConsumer,
	}, nil
}

func (a *App) Run() error {
	addr := fmt.Sprintf(":%s", a.cfg.Port)
	appCtx, stopBackground := context.WithCancel(context.Background())
	var backgroundWG sync.WaitGroup

	// Start user sync consumer
	if a.userSyncConsumer != nil {
		backgroundWG.Add(1)
		go func() {
			defer backgroundWG.Done()
			if err := a.userSyncConsumer.Run(appCtx); err != nil {
				slog.Error("Ошибка RabbitMQ consumer синхронизации пользователей", "error", err)
			}
		}()
	}

	// Start Kanban notification events consumer
	if a.kanbanNotificationConsumer != nil {
		backgroundWG.Add(1)
		go func() {
			defer backgroundWG.Done()
			if err := a.kanbanNotificationConsumer.Run(appCtx); err != nil {
				slog.Error("Ошибка RabbitMQ kanban notification consumer", "error", err)
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
