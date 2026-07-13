package main

import (
	"log/slog"
	"os"

	"notification_service_document_flow/internal/app"
	"notification_service_document_flow/internal/config"
	"notification_service_document_flow/internal/logger"
	"notification_service_document_flow/internal/validator"
)

func main() {
	cfg := config.Load()

	logger.Setup(cfg.Env)
	validator.Init()

	db, err := config.ConnectDB(cfg)
	if err != nil {
		slog.Error("Can't connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := config.ValidateSchema(db); err != nil {
		slog.Error("Database schema check failed", "error", err)
		slog.Error("Hint: make sure you ran 'goose up' (or apply migrations/00001_init_notification_schema.sql)")
		os.Exit(1)
	}

	application, err := app.NewApp(cfg, db)
	if err != nil {
		slog.Error("Can't initialize application", "error", err)
		os.Exit(1)
	}

	if err := application.Run(); err != nil {
		slog.Error("Application error", "error", err)
		os.Exit(1)
	}
}
