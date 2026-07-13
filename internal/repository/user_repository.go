package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"notification_service_document_flow/internal/model"
)

// UserRepository uses raw SQL (not sqlc) because the user synchronization
// logic requires a specialized bulk upsert pattern via a temporary table.
// This approach ensures atomic batch inserts/updates and performs well
// when processing user.* events from RabbitMQ.
//
// sqlc is deliberately used only for the core notification queries
// (which are simple and stable). We avoid mixing query builders
// (such as Squirrel) at this stage to keep the project consistent
// with its original design.
type UserRepository struct {
	Db *pgxpool.Pool
}

func NewUserRepository(db *pgxpool.Pool) *UserRepository {
	return &UserRepository{Db: db}
}

func (r *UserRepository) GetUsersByIDs(ctx context.Context, ids []int64) ([]model.User, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	query := `SELECT id, login, lastname, firstname, patronymic, avatar_name, deleted_at 
	          FROM users WHERE id = ANY($1)`
	rows, err := r.Db.Query(ctx, query, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []model.User
	for rows.Next() {
		var u model.User
		err := rows.Scan(&u.ID, &u.Login, &u.Lastname, &u.Firstname, &u.Patronymic, &u.AvatarName, &u.DeletedAt)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return users, nil
}

func (r *UserRepository) UpsertUsers(ctx context.Context, users []model.User) error {
	if len(users) == 0 {
		return nil
	}

	tx, err := r.Db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		CREATE TEMP TABLE users_tmp (LIKE users INCLUDING ALL)
		ON COMMIT DROP
	`)
	if err != nil {
		return err
	}

	for _, u := range users {
		_, err = tx.Exec(ctx, `
			INSERT INTO users_tmp (id, login, lastname, firstname, patronymic, avatar_name, deleted_at, synced_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
			ON CONFLICT (id) DO UPDATE SET
				login = EXCLUDED.login,
				lastname = EXCLUDED.lastname,
				firstname = EXCLUDED.firstname,
				patronymic = EXCLUDED.patronymic,
				avatar_name = EXCLUDED.avatar_name,
				deleted_at = EXCLUDED.deleted_at,
				synced_at = EXCLUDED.synced_at
		`, u.ID, u.Login, u.Lastname, u.Firstname, u.Patronymic, u.AvatarName, u.DeletedAt)
		if err != nil {
			return err
		}
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO users 
		SELECT * FROM users_tmp
		ON CONFLICT (id) DO UPDATE SET
			login = EXCLUDED.login,
			lastname = EXCLUDED.lastname,
			firstname = EXCLUDED.firstname,
			patronymic = EXCLUDED.patronymic,
			avatar_name = EXCLUDED.avatar_name,
			deleted_at = EXCLUDED.deleted_at,
			synced_at = EXCLUDED.synced_at
	`)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *UserRepository) MarkUserDeleted(ctx context.Context, userID int64, deletedAt time.Time) error {
	// Note: caller must pass Moscow wall time (use helper.Clock.ToWall or cfg.ToLocal).
	// synced_at uses NOW() which produces Moscow time thanks to the "timezone=..." parameter
	// on the database connection string.
	_, err := r.Db.Exec(ctx, `
		UPDATE users
		SET deleted_at = $2,
		    synced_at = NOW()
		WHERE id = $1
	`, userID, deletedAt)
	return err
}
