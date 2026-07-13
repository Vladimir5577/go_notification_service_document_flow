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
