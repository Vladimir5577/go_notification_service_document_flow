-- +goose Up

-- Браузерные push-подписки. endpoint уникален: один браузер — одна строка,
-- при смене пользователя на том же устройстве строка переезжает к нему.
CREATE TABLE push_subscription (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL,
    endpoint   TEXT NOT NULL UNIQUE,
    p256dh     TEXT NOT NULL,
    auth       TEXT NOT NULL,
    created_at TIMESTAMPTZ(0) NOT NULL DEFAULT NOW()
);

CREATE INDEX push_subscription_user_id_idx ON push_subscription (user_id);

-- +goose Down

DROP TABLE IF EXISTS push_subscription;
