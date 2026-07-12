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
