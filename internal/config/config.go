package config

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"notification_service_document_flow/internal/helper"
)

type Config struct {
	Env              string
	Port             string
	JWTPublicKeyPath string

	RabbitMQDSN      string
	RabbitMQExchange string
	UserSyncQueue    string

	// DB configuration
	DBHost     string
	DBPort     int
	DBUser     string
	DBPassword string
	DBName     string

	// DBTimezone controls the session timezone for Postgres (affects NOW() etc.).
	// We set it to Europe/Moscow so that "real" local time is used when storing
	// in TIMESTAMP(0) columns. Go side also uses the same location.
	DBTimezone string

	// TimezoneLocation is kept for backward compatibility during transition.
	// Prefer using the Clock field below.
	TimezoneLocation *time.Location

	// Clock provides helpers for storing and reading wall-clock (Moscow) time
	// in TIMESTAMP WITHOUT TIME ZONE columns.
	//
	// The implementation lives in internal/helper (clock.go).
	Clock helper.Clock
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		slog.Warn("Предупреждение: .env файл не найден, используются системные переменные окружения")
	}

	c := &Config{
		Env:              getEnv("ENV", "local"),
		Port:             getEnv("SERVER_PORT", "8086"),
		JWTPublicKeyPath: getEnv("JWT_PUBLIC_KEY_PATH", "../symfony_documents_flow/config/jwt/public.pem"),

		RabbitMQDSN:      getEnv("RABBITMQ_TRANSPORT_DSN", "amqp://guest:guest@rabbitmq:5672/"),
		RabbitMQExchange: getEnv("RABBITMQ_EVENTS_EXCHANGE", "events"),
		UserSyncQueue:    getEnv("RABBITMQ_USER_SYNC_QUEUE", "notification.user_sync"),

		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnvAsInt("DB_PORT", 5432),
		DBUser:     getEnv("DB_USER", ""),
		DBPassword: getEnv("DB_PASSWORD", ""),
		DBName:     getEnv("DB_NAME", ""),

		DBTimezone: getEnv("DB_TIMEZONE", "Europe/Moscow"),
	}

	// Load the location once. This is central to storing "real time" (Moscow wall time).
	loc, err := time.LoadLocation(c.DBTimezone)
	if err != nil {
		slog.Warn("Failed to load DBTimezone location, falling back to UTC", "timezone", c.DBTimezone, "err", err)
		loc = time.UTC
	}
	c.TimezoneLocation = loc
	c.Clock = helper.NewClock(loc)

	return c
}

func ConnectDB(conf *Config) (*pgxpool.Pool, error) {
	// Include timezone so that NOW() and timestamp literals use Moscow wall time on DB side.
	// Combined with container TZ and our clock.Clock helpers, this ensures we store
	// actual civil time (not UTC) in TIMESTAMP columns.
	connStr := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable timezone=%s",
		conf.DBHost,
		conf.DBPort,
		conf.DBUser,
		conf.DBPassword,
		conf.DBName,
		conf.DBTimezone,
	)

	const maxAttempts = 12 // ~60 seconds total with 5s interval

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		pool, err := pgxpool.New(context.Background(), connStr)
		if err == nil {
			if pingErr := pool.Ping(context.Background()); pingErr == nil {
				if attempt > 1 {
					slog.Info("Database connection established", "attempt", attempt)
				}
				return pool, nil
			} else {
				lastErr = pingErr
			}
			pool.Close()
		} else {
			lastErr = err
		}

		slog.Warn("Waiting for database to be ready", "attempt", attempt, "max", maxAttempts, "error", lastErr)
		time.Sleep(5 * time.Second)
	}

	return nil, fmt.Errorf("failed to connect to database after %d attempts: %w", maxAttempts, lastErr)
}

// ValidateSchema checks that the required tables exist in the database.
// If the tables are missing (migrations not applied), it returns an error.
// The application should exit immediately in this case instead of starting
// consumers that will spam retry errors.
func ValidateSchema(db *pgxpool.Pool) error {
	requiredTables := []string{"notification", "users"}

	for _, table := range requiredTables {
		var exists bool
		query := `SELECT EXISTS (
			SELECT 1 FROM information_schema.tables 
			WHERE table_schema = 'public' AND table_name = $1
		)`
		if err := db.QueryRow(context.Background(), query, table).Scan(&exists); err != nil {
			return fmt.Errorf("failed to check table existence: %w", err)
		}
		if !exists {
			return fmt.Errorf(
				"database schema is not initialized: table %q does not exist. "+
					"Run migrations first: goose up",
				table,
			)
		}
	}
	return nil
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

func getEnvAsInt(key string, fallback int) int {
	valueStr := getEnv(key, "")
	if value, err := strconv.Atoi(valueStr); err == nil {
		return value
	}
	return fallback
}

// Now returns current wall-clock time in the configured timezone (Europe/Moscow).
//
// Prefer using cfg.Clock.Now() for new code. This method is kept for compatibility.
func (c *Config) Now() time.Time {
	return c.Clock.Now()
}

// ToLocal converts the given time to wall time in our configured timezone.
//
// This is used for times coming from external sources (RabbitMQ messages,
// AMQP delivery timestamps, etc.) so that the numeric values stored in
// TIMESTAMP columns represent actual Moscow civil time.
//
// Prefer cfg.Clock.ToWall() in new code.
func (c *Config) ToLocal(t time.Time) time.Time {
	return c.Clock.ToWall(t)
}

