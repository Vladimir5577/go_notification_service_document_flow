-- +goose Up

-- ============================================================================
-- НАЧАЛЬНАЯ СХЕМА БД NOTIFICATION SERVICE
-- users — реплика из Symfony (синхронизируется через RabbitMQ, без FK).
-- notification — собственная таблица сервиса.
-- Используем BIGINT для ID, чтобы избежать преобразований int32<->int64 в Go.
-- ============================================================================

-- Реплика пользователей (синхронизируется через RabbitMQ, без FK)
CREATE TABLE users (
    id          BIGINT PRIMARY KEY,
    login       VARCHAR(50) NOT NULL UNIQUE,
    lastname    VARCHAR(50) NOT NULL,
    firstname   VARCHAR(50) NOT NULL,
    patronymic  VARCHAR(50),
    avatar_name VARCHAR(255),
    deleted_at  TIMESTAMPTZ(0),
    synced_at   TIMESTAMPTZ(0) NOT NULL DEFAULT NOW()
);

-- Таблица уведомлений
CREATE TABLE notification (
    id          BIGSERIAL PRIMARY KEY,
    type        VARCHAR(50) NOT NULL,
    -- title/link — генерируемый из событий текст (имена задач, колонок, ссылки).
    -- Держим TEXT, а не VARCHAR(n): у Postgres нет разницы в скорости, но нет и
    -- класса ошибок "value too long", который ронял сервис (SQLSTATE 22001).
    title       TEXT NOT NULL,
    message     TEXT,
    link        TEXT,
    created_at  TIMESTAMPTZ(0) NOT NULL DEFAULT NOW(),
    read_at     TIMESTAMPTZ(0),
    extra       JSON,
    user_id     BIGINT NOT NULL
);

-- Индексы для эффективных запросов (как в продакшене)
CREATE INDEX idx_notification_user_read ON notification (user_id, read_at);
CREATE INDEX idx_notification_user_created ON notification (user_id, created_at);

-- +goose Down
DROP INDEX IF EXISTS idx_notification_user_created;
DROP INDEX IF EXISTS idx_notification_user_read;
DROP TABLE IF EXISTS notification;
DROP TABLE IF EXISTS users;
