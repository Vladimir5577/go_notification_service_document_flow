package repository

import "context"

// PushSubscription — одна подписка браузера на Web Push.
type PushSubscription struct {
	Endpoint string
	P256dh   string
	Auth     string
}

func (r *NotificationRepository) UpsertPushSubscription(ctx context.Context, userID int64, sub PushSubscription) error {
	_, err := r.Db.Exec(ctx, `
		INSERT INTO push_subscription (user_id, endpoint, p256dh, auth)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (endpoint) DO UPDATE
		SET user_id = EXCLUDED.user_id,
		    p256dh = EXCLUDED.p256dh,
		    auth = EXCLUDED.auth
	`, userID, sub.Endpoint, sub.P256dh, sub.Auth)
	return err
}

func (r *NotificationRepository) DeletePushSubscription(ctx context.Context, userID int64, endpoint string) error {
	_, err := r.Db.Exec(ctx, `
		DELETE FROM push_subscription WHERE user_id = $1 AND endpoint = $2
	`, userID, endpoint)
	return err
}

func (r *NotificationRepository) DeletePushSubscriptionByEndpoint(ctx context.Context, endpoint string) error {
	_, err := r.Db.Exec(ctx, `DELETE FROM push_subscription WHERE endpoint = $1`, endpoint)
	return err
}

func (r *NotificationRepository) ListPushSubscriptions(ctx context.Context, userID int64) ([]PushSubscription, error) {
	rows, err := r.Db.Query(ctx, `
		SELECT endpoint, p256dh, auth FROM push_subscription WHERE user_id = $1
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subs []PushSubscription
	for rows.Next() {
		var sub PushSubscription
		if err := rows.Scan(&sub.Endpoint, &sub.P256dh, &sub.Auth); err != nil {
			return nil, err
		}
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}
