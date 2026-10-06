// Package store provides Postgres persistence for users and subscriptions.
package store

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// ErrNotFound is returned when the requested row does not exist.
var ErrNotFound = errors.New("store: not found")

// HWID delivery modes.
const (
	HWIDModeHeader = "header"
	HWIDModeQuery  = "query"
)

// User is a Telegram user of the bot.
type User struct {
	TgID int64
	Lang string
}

// Subscription is a provider subscription proxied through our domain.
type Subscription struct {
	ID        int64
	UserID    int64
	Name      string
	OriginURL string
	HWID      string
	HWIDMode  string
	HWIDParam string
	Token     string
	CreatedAt time.Time
}

// Store wraps a pgx connection pool.
type Store struct {
	pool *pgxpool.Pool
}

// New creates a Store on top of an existing pool.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Migrate applies embedded SQL migrations in order.
func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return fmt.Errorf("store: create schema_migrations: %w", err)
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("store: read migrations: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		var exists bool
		if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE name = $1)`, name).Scan(&exists); err != nil {
			return fmt.Errorf("store: check migration %s: %w", name, err)
		}
		if exists {
			continue
		}
		raw, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("store: read migration %s: %w", name, err)
		}
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("store: begin migration %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, string(raw)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("store: apply migration %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (name) VALUES ($1)`, name); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("store: record migration %s: %w", name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("store: commit migration %s: %w", name, err)
		}
		slog.Info("store: migration applied", "name", name)
	}
	return nil
}

// Ping checks database connectivity.
func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

// UpsertUser creates the user or updates nothing if it already exists.
func (s *Store) UpsertUser(ctx context.Context, tgID int64, lang string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO users (tg_id, lang) VALUES ($1, $2)
		ON CONFLICT (tg_id) DO NOTHING`, tgID, lang)
	if err != nil {
		return fmt.Errorf("store: upsert user: %w", err)
	}
	return nil
}

// GetUser returns the user row.
func (s *Store) GetUser(ctx context.Context, tgID int64) (User, error) {
	var u User
	err := s.pool.QueryRow(ctx, `SELECT tg_id, lang FROM users WHERE tg_id = $1`, tgID).
		Scan(&u.TgID, &u.Lang)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("store: get user: %w", err)
	}
	return u, nil
}

// SetLang saves the interface language of the user.
func (s *Store) SetLang(ctx context.Context, tgID int64, lang string) error {
	if _, err := s.pool.Exec(ctx, `UPDATE users SET lang = $2 WHERE tg_id = $1`, tgID, lang); err != nil {
		return fmt.Errorf("store: set lang: %w", err)
	}
	return nil
}

// CreateSubscription inserts a new subscription with a fresh public token.
func (s *Store) CreateSubscription(ctx context.Context, sub *Subscription) error {
	token, err := newToken()
	if err != nil {
		return err
	}
	sub.Token = token
	err = s.pool.QueryRow(ctx, `
		INSERT INTO subscriptions (user_id, name, origin_url, hwid, hwid_mode, hwid_param, token)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id`, sub.UserID, sub.Name, sub.OriginURL, sub.HWID, sub.HWIDMode, sub.HWIDParam, sub.Token).
		Scan(&sub.ID)
	if err != nil {
		return fmt.Errorf("store: create subscription: %w", err)
	}
	return nil
}

// ListSubscriptions returns all subscriptions of the user, newest first.
func (s *Store) ListSubscriptions(ctx context.Context, userID int64) ([]Subscription, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, name, origin_url, hwid, hwid_mode, hwid_param, token, created_at
		FROM subscriptions WHERE user_id = $1
		ORDER BY id DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("store: list subscriptions: %w", err)
	}
	defer rows.Close()

	var out []Subscription
	for rows.Next() {
		var sub Subscription
		if err := rows.Scan(&sub.ID, &sub.UserID, &sub.Name, &sub.OriginURL, &sub.HWID, &sub.HWIDMode, &sub.HWIDParam, &sub.Token, &sub.CreatedAt); err != nil {
			return nil, fmt.Errorf("store: scan subscription: %w", err)
		}
		out = append(out, sub)
	}
	return out, rows.Err()
}

// GetSubscription returns one subscription owned by the user.
func (s *Store) GetSubscription(ctx context.Context, userID, id int64) (Subscription, error) {
	var sub Subscription
	err := s.pool.QueryRow(ctx, `
		SELECT id, user_id, name, origin_url, hwid, hwid_mode, hwid_param, token, created_at
		FROM subscriptions WHERE user_id = $1 AND id = $2`, userID, id).
		Scan(&sub.ID, &sub.UserID, &sub.Name, &sub.OriginURL, &sub.HWID, &sub.HWIDMode, &sub.HWIDParam, &sub.Token, &sub.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Subscription{}, ErrNotFound
	}
	if err != nil {
		return Subscription{}, fmt.Errorf("store: get subscription: %w", err)
	}
	return sub, nil
}

// GetByToken returns a subscription by its public token (no ownership check,
// used by the HTTP endpoint).
func (s *Store) GetByToken(ctx context.Context, token string) (Subscription, error) {
	var sub Subscription
	err := s.pool.QueryRow(ctx, `
		SELECT id, user_id, name, origin_url, hwid, hwid_mode, hwid_param, token
		FROM subscriptions WHERE token = $1`, token).
		Scan(&sub.ID, &sub.UserID, &sub.Name, &sub.OriginURL, &sub.HWID, &sub.HWIDMode, &sub.HWIDParam, &sub.Token)
	if errors.Is(err, pgx.ErrNoRows) {
		return Subscription{}, ErrNotFound
	}
	if err != nil {
		return Subscription{}, fmt.Errorf("store: get by token: %w", err)
	}
	return sub, nil
}

// DeleteSubscription removes a subscription owned by the user.
func (s *Store) DeleteSubscription(ctx context.Context, userID, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM subscriptions WHERE user_id = $1 AND id = $2`, userID, id)
	if err != nil {
		return fmt.Errorf("store: delete subscription: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RenameSubscription changes the display name of a subscription.
func (s *Store) RenameSubscription(ctx context.Context, userID, id int64, name string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE subscriptions SET name = $3, updated_at = now()
		WHERE user_id = $1 AND id = $2`, userID, id, name)
	if err != nil {
		return fmt.Errorf("store: rename subscription: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetHWID replaces the stored HWID of a subscription.
func (s *Store) SetHWID(ctx context.Context, userID, id int64, hwid string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE subscriptions SET hwid = $3, updated_at = now()
		WHERE user_id = $1 AND id = $2`, userID, id, hwid)
	if err != nil {
		return fmt.Errorf("store: set hwid: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetHWIDMode switches the HWID delivery mode of a subscription.
func (s *Store) SetHWIDMode(ctx context.Context, userID, id int64, mode, param string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE subscriptions SET hwid_mode = $3, hwid_param = $4, updated_at = now()
		WHERE user_id = $1 AND id = $2`, userID, id, mode, param)
	if err != nil {
		return fmt.Errorf("store: set hwid mode: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// newToken generates a short URL-safe public token for a subscription link.
func newToken() (string, error) {
	var b [18]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("store: generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}
