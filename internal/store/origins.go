package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ErrLastOrigin is returned when deleting the only origin of a subscription.
var ErrLastOrigin = errors.New("store: last origin of subscription")

// AddOrigin attaches a new origin to a subscription owned by the user.
func (s *Store) AddOrigin(ctx context.Context, userID, subID int64, o *Origin) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("store: begin add origin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var owned int64
	err = tx.QueryRow(ctx, `SELECT id FROM subscriptions WHERE user_id = $1 AND id = $2`, userID, subID).Scan(&owned)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("store: check subscription owner: %w", err)
	}

	o.SubscriptionID = subID
	if err := insertOrigin(ctx, tx, o); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("store: commit add origin: %w", err)
	}
	return nil
}

// DeleteOrigin removes an origin owned by the user and returns the id of its
// subscription. The last origin of a subscription cannot be removed.
func (s *Store) DeleteOrigin(ctx context.Context, userID, originID int64) (int64, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("store: begin delete origin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var subID int64
	err = tx.QueryRow(ctx, `
		SELECT o.subscription_id
		FROM subscription_origins o
		JOIN subscriptions s ON s.id = o.subscription_id
		WHERE o.id = $1 AND s.user_id = $2`, originID, userID).Scan(&subID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("store: find origin: %w", err)
	}

	// Locking the subscription serializes concurrent deletes of its origins.
	if _, err := tx.Exec(ctx, `SELECT id FROM subscriptions WHERE id = $1 FOR UPDATE`, subID); err != nil {
		return 0, fmt.Errorf("store: lock subscription: %w", err)
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM subscription_origins WHERE subscription_id = $1`, subID).Scan(&count); err != nil {
		return 0, fmt.Errorf("store: count origins: %w", err)
	}
	if count <= 1 {
		return 0, ErrLastOrigin
	}
	if _, err := tx.Exec(ctx, `DELETE FROM subscription_origins WHERE id = $1`, originID); err != nil {
		return 0, fmt.Errorf("store: delete origin: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("store: commit delete origin: %w", err)
	}
	return subID, nil
}

// insertOrigin stores an origin and fills its id and creation time.
func insertOrigin(ctx context.Context, tx pgx.Tx, o *Origin) error {
	err := tx.QueryRow(ctx, `
		INSERT INTO subscription_origins (subscription_id, origin_url, hwid, hwid_mode, hwid_param)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at`,
		o.SubscriptionID, o.URL, o.HWID, o.HWIDMode, o.HWIDParam).
		Scan(&o.ID, &o.CreatedAt)
	if err != nil {
		return fmt.Errorf("store: insert origin: %w", err)
	}
	return nil
}

// attachOrigins loads the origins of the given subscriptions in one query,
// keeping their creation order.
func (s *Store) attachOrigins(ctx context.Context, subs []Subscription) error {
	if len(subs) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(subs))
	index := make(map[int64]int, len(subs))
	for i, sub := range subs {
		ids = append(ids, sub.ID)
		index[sub.ID] = i
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, subscription_id, origin_url, hwid, hwid_mode, hwid_param, created_at
		FROM subscription_origins
		WHERE subscription_id = ANY($1)
		ORDER BY id`, ids)
	if err != nil {
		return fmt.Errorf("store: list origins: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var o Origin
		if err := rows.Scan(&o.ID, &o.SubscriptionID, &o.URL, &o.HWID, &o.HWIDMode, &o.HWIDParam, &o.CreatedAt); err != nil {
			return fmt.Errorf("store: scan origin: %w", err)
		}
		i := index[o.SubscriptionID]
		subs[i].Origins = append(subs[i].Origins, o)
	}
	return rows.Err()
}
