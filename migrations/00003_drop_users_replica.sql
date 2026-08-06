-- +goose Up

DROP TABLE IF EXISTS users;

-- +goose Down

-- Схема восстанавливается, данные — нет: их источник в монолите, и повторное
-- наполнение возможно только полным ресинком (команды для него не существует).
CREATE TABLE IF NOT EXISTS users (
    id          BIGINT PRIMARY KEY,
    login       VARCHAR(50) NOT NULL UNIQUE,
    lastname    VARCHAR(50) NOT NULL,
    firstname   VARCHAR(50) NOT NULL,
    patronymic  VARCHAR(50),
    avatar_name VARCHAR(255),
    deleted_at  TIMESTAMPTZ(0),
    synced_at   TIMESTAMPTZ(0) NOT NULL DEFAULT NOW()
);
