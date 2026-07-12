package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"notification_service_document_flow/internal/model"
	"notification_service_document_flow/internal/repository/dbgen"
)

type NotificationRepository struct {
	Db *pgxpool.Pool
}

func NewNotificationRepository(db *pgxpool.Pool) *NotificationRepository {
	return &NotificationRepository{Db: db}
}

func (r *NotificationRepository) FindAllForUser(ctx context.Context, userID int64, page, pageSize int) ([]model.Notification, error) {
	queries := dbgen.New(r.Db)

	dbNotifs, err := queries.FindAllForUser(ctx, dbgen.FindAllForUserParams{
		UserID: userID,
		Limit:  int32(pageSize),
		Offset: int32((page - 1) * pageSize),
	})
	if err != nil {
		return nil, err
	}

	return mapDBNotifications(dbNotifs), nil
}

func (r *NotificationRepository) FindLatestForUser(ctx context.Context, userID int64, limit int) ([]model.Notification, error) {
	queries := dbgen.New(r.Db)

	dbNotifs, err := queries.FindLatestForUser(ctx, dbgen.FindLatestForUserParams{
		UserID: userID,
		Limit:  int32(limit),
	})
	if err != nil {
		return nil, err
	}

	return mapDBNotifications(dbNotifs), nil
}

func (r *NotificationRepository) CountAllForUser(ctx context.Context, userID int64) (int, error) {
	queries := dbgen.New(r.Db)
	count, err := queries.CountAllForUser(ctx, userID)
	return int(count), err
}

func (r *NotificationRepository) CountUnreadForUser(ctx context.Context, userID int64) (int, error) {
	queries := dbgen.New(r.Db)
	count, err := queries.CountUnreadForUser(ctx, userID)
	if count > 100 {
		count = 100
	}
	return int(count), err
}

func (r *NotificationRepository) MarkAsRead(ctx context.Context, id, userID int64) error {
	queries := dbgen.New(r.Db)
	return queries.MarkAsRead(ctx, dbgen.MarkAsReadParams{
		ID:     id,
		UserID: userID,
	})
}

func (r *NotificationRepository) MarkAllAsReadForUser(ctx context.Context, userID int64) error {
	queries := dbgen.New(r.Db)
	return queries.MarkAllAsReadForUser(ctx, userID)
}

func (r *NotificationRepository) Create(ctx context.Context, n *model.Notification) (*model.Notification, error) {
	queries := dbgen.New(r.Db)

	createdAt := n.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	var msg pgtype.Text
	if n.Message != nil {
		msg = pgtype.Text{String: *n.Message, Valid: true}
	}

	var link pgtype.Text
	if n.Link != nil {
		link = pgtype.Text{String: *n.Link, Valid: true}
	}

	var readAt pgtype.Timestamp
	if n.ReadAt != nil {
		readAt = pgtype.Timestamp{Time: *n.ReadAt, Valid: true}
	}

	dbNotif, err := queries.CreateNotification(ctx, dbgen.CreateNotificationParams{
		Type:      n.Type,
		Title:     n.Title,
		Message:   msg,
		Link:      link,
		CreatedAt: pgtype.Timestamp{Time: createdAt, Valid: true},
		ReadAt:    readAt,
		Extra:     n.Extra,
		UserID:    n.UserID,
	})
	if err != nil {
		return nil, err
	}

	return mapDBNotification(&dbNotif), nil
}

// mappers convert dbgen types (pgtype) to clean model types
func mapDBNotifications(dbNotifs []dbgen.Notification) []model.Notification {
	res := make([]model.Notification, len(dbNotifs))
	for i := range dbNotifs {
		res[i] = *mapDBNotification(&dbNotifs[i])
	}
	return res
}

func mapDBNotification(db *dbgen.Notification) *model.Notification {
	n := &model.Notification{
		ID:     db.ID,
		Type:   db.Type,
		Title:  db.Title,
		Extra:  db.Extra,
		UserID: db.UserID,
	}

	if db.Message.Valid {
		n.Message = &db.Message.String
	}
	if db.Link.Valid {
		n.Link = &db.Link.String
	}
	if db.CreatedAt.Valid {
		n.CreatedAt = db.CreatedAt.Time
	}
	if db.ReadAt.Valid {
		t := db.ReadAt.Time
		n.ReadAt = &t
	}

	return n
}
