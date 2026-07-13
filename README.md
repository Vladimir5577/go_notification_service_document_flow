# Notification Service for Document Flow

## Основные возможности
- Синхронизация пользователей через RabbitMQ (`user.upserted`, `user.deleted`)
- Получение событий уведомлений от Kanban (`kanban.notification.*`)
- Хранение уведомлений в собственной БД
- SPA API под `/spa/api/notifications` (совместим с frontend analytics_platform)
- JWT авторизация (RS256)

## Быстрый старт

### 1. Подготовка
Скопируйте файл конфигурации:
```bash
cp .env.example .env
```

### 2. Запуск
```bash
docker compose build
docker compose up -d
```

### 3. Apply migrations
```bash
~/go/bin/goose up
```

## Разработка

### Локальная сборка (Hot Reload)
Соберите бинарный файл под Linux и перезапустите контейнер:
```bash
GOOS=linux GOARCH=amd64 go build -o main cmd/main.go
docker compose restart notification_service
```

### База данных и миграции
Проект использует [sqlc](https://sqlc.dev/). 
- SQL-запросы находятся в `internal/repository/query.sql`
- Миграции в папке `migrations/`
- Сгенерированный код БД лежит в `internal/repository/dbgen/` (не редактировать вручную!)

Установка sqlc:
```bash
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
```

Полезные команды БД:

**Применить миграции**:
```bash
~/go/bin/goose up
```

**Генерация кода (sqlc)** (запускать после изменения структуры БД или запросов):
```bash
sqlc generate
```

**DBGate (запуск веб-клиента БД)**:
```bash
docker compose -f docker-compose.dbgate.yml up -d
```

**DBGate (остановка)**:
```bash
docker compose -f docker-compose.dbgate.yml down
```

### RabbitMQ
Сервис подписывается на exchange `events`:
- `user.upserted`, `user.deleted` — синхронизация пользователей (Очередь: `notification.user_sync`)
- `kanban.notification.*` — создание уведомлений

## Работа со временем

Проект хранит **фактическое московское время** (wall time) в колонках `TIMESTAMP(0)`:

- `created_at`, `read_at` в таблице `notification`
- `deleted_at`, `synced_at` в таблице `users`

### Почему именно так

- Требование: дата в базе и в API должна быть именно той, которую видят пользователи (московское гражданское время).
- Используются колонки без таймзоны (`TIMESTAMP`, а не `TIMESTAMPTZ`).
- Все операции записи проходят через `helper.Clock` (расположен в `internal/helper/clock.go`):
  - `Clock.Now()` — текущее московское время
  - `Clock.ToWall(t)` — подготовка времени перед записью в БД
  - `Clock.FromDB(t)` — правильная интерпретация чисел при чтении из БД
- `NOW()` в SQL тоже даёт московское время благодаря параметру `timezone=Europe/Moscow` в строке подключения и `TZ` в контейнере.

### Важные правила

- Никогда не используй `time.Now().UTC()` или голый `time.Now()` при записи в БД.
- При чтении из базы всегда используй хелперы из `helper.Clock`, чтобы у `time.Time` была правильная локация.
- Вся логика централизована в `internal/helper/clock.go` (там подробная документация).

См. также:
- `internal/config/config.go` (поле `Clock`)
- `internal/helper/clock.go`
- Комментарии в `notification_repository.go` и `service/notification_service.go`

## Полезные команды

**Логи сервиса**:
```bash
docker compose logs -f notification_service
```

**Статус контейнеров**:
```bash
docker compose ps
```

**Полная пересборка**:
```bash
docker compose build --no-cache
```
