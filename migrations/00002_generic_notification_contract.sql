-- +goose NO TRANSACTION
-- +goose Up

-- ============================================================================
-- ПЕРЕХОД НА ОБЩИЙ КОНТРАКТ УВЕДОМЛЕНИЙ
--
-- NO TRANSACTION — из-за CREATE INDEX CONCURRENTLY, его нельзя выполнять
-- внутри транзакции, а goose по умолчанию оборачивает миграцию в неё.
-- Таблица живая и непустая, поэтому обе колонки nullable: backfill под
-- event_id не нужен и NOT NULL не понадобится даже потом (см. индекс ниже).
-- ============================================================================

-- Дедупликация. Id генерирует продюсер, один на событие (не на получателя),
-- и переиспользует при повторной отправке. RabbitMQ гарантирует at-least-once:
-- при ретрае, переподключении или nack сообщение придёт снова.
-- ADD COLUMN без DEFAULT в PG11+ — только метаданные, без переписывания таблицы.
ALTER TABLE notification ADD COLUMN IF NOT EXISTS event_id UUID;

-- Подпись категории для списка уведомлений. Раньше её собирал switch
-- mapTypeToLabel в dto — по одному case на каждый тип каждого модуля.
-- Теперь приходит от продюсера вместе с заголовком.
ALTER TABLE notification ADD COLUMN IF NOT EXISTS type_label TEXT;

-- Старым строкам проставляем подписи по прежнему словарю — иначе уже
-- существующие уведомления остались бы в списке без категории.
-- Если таблица окажется крупной, разбить на батчи по id.
UPDATE notification
SET type_label = CASE
    WHEN type = 'DOCUMENT_SENT'                              THEN 'Документ отправлен'
    WHEN type = 'NEW_INCOMING_DOCUMENT'                      THEN 'Новый входящий документ'
    WHEN type IN ('KANBAN_TASK_ASSIGNED_TO_USER', 'TASK_ASSIGNED') THEN 'Назначена задача'
    WHEN type = 'KANBAN_CARD_CREATED'                        THEN 'Создана задача'
    WHEN type = 'USER_ADDED_TO_KANBAN_PROJECT'               THEN 'Добавлен в проект'
    WHEN type = 'USER_REMOVED_FROM_KANBAN_PROJECT'           THEN 'Исключён из проекта'
    WHEN type = 'TASK_MOVED'                                 THEN 'Задача перемещена'
    WHEN type = 'TASK_COMMENT_ADDED'                         THEN 'Новый комментарий в задаче'
    WHEN type = 'DOCUMENT_COMMENT_ADDED'                     THEN 'Новый комментарий к документу'
    ELSE 'Уведомление'
END
WHERE type_label IS NULL;

-- Индекс по ПАРЕ (event_id, user_id): из одного события сервис создаёт по
-- строке на получателя, и уникальность по одному event_id уронила бы его
-- собственную вторую вставку.
--
-- Частичный: NULL в уникальном индексе Postgres не равен другому NULL, но
-- держать в индексе тысячи легаси-строк незачем.
--
-- CONCURRENTLY не блокирует запись. Если построение сорвётся, останется
-- невалидный индекс — снести руками и повторить миграцию (IF NOT EXISTS
-- не даст упасть на занятом имени).
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS notification_event_recipient_uniq
    ON notification (event_id, user_id)
    WHERE event_id IS NOT NULL;

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS notification_event_recipient_uniq;
ALTER TABLE notification DROP COLUMN IF EXISTS type_label;
ALTER TABLE notification DROP COLUMN IF EXISTS event_id;
