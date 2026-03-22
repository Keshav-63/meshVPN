package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"MeshVPN-slef-hosting/control-plane/internal/domain"
	"MeshVPN-slef-hosting/control-plane/internal/logs"
)

type PostgresUserRepository struct {
	db *sql.DB
}

func NewPostgresUserRepository(db *sql.DB) *PostgresUserRepository {
	return &PostgresUserRepository{db: db}
}

// GetByID retrieves a user by their user ID
func (r *PostgresUserRepository) GetByID(userID string) (domain.User, error) {
	const query = `
SELECT user_id, email, provider, is_subscriber, subscription_tier, created_at, updated_at
FROM users
WHERE user_id = $1
`

	var user domain.User
	var subscriptionTier sql.NullString
	var email sql.NullString

	err := r.db.QueryRow(query, userID).Scan(
		&user.UserID,
		&email,
		&user.Provider,
		&user.IsSubscriber,
		&subscriptionTier,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return domain.User{}, errors.New("user not found")
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("query user: %w", err)
	}

	if email.Valid {
		user.Email = email.String
	}
	if subscriptionTier.Valid {
		user.SubscriptionTier = subscriptionTier.String
	}

	return user, nil
}

// GetByEmail retrieves a user by their email address
func (r *PostgresUserRepository) GetByEmail(email string) (domain.User, error) {
	const query = `
SELECT user_id, email, provider, is_subscriber, subscription_tier, created_at, updated_at
FROM users
WHERE email = $1
`

	var user domain.User
	var subscriptionTier sql.NullString
	var emailVal sql.NullString

	err := r.db.QueryRow(query, email).Scan(
		&user.UserID,
		&emailVal,
		&user.Provider,
		&user.IsSubscriber,
		&subscriptionTier,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return domain.User{}, errors.New("user not found")
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("query user: %w", err)
	}

	if emailVal.Valid {
		user.Email = emailVal.String
	}
	if subscriptionTier.Valid {
		user.SubscriptionTier = subscriptionTier.String
	}

	return user, nil
}

// Upsert creates a new user or updates an existing one
func (r *PostgresUserRepository) Upsert(user domain.User) error {
	const stmt = `
INSERT INTO users (
	user_id, email, provider, is_subscriber, subscription_tier, created_at, updated_at
)
VALUES (
	$1, NULLIF($2, ''), NULLIF($3, ''), $4, NULLIF($5, ''), $6, $7
)
ON CONFLICT (user_id)
DO UPDATE SET
	email = EXCLUDED.email,
	provider = EXCLUDED.provider,
	is_subscriber = EXCLUDED.is_subscriber,
	subscription_tier = EXCLUDED.subscription_tier,
	updated_at = EXCLUDED.updated_at
`

	if user.CreatedAt.IsZero() {
		user.CreatedAt = time.Now().UTC()
	}
	user.UpdatedAt = time.Now().UTC()

	_, err := r.db.Exec(
		stmt,
		user.UserID,
		user.Email,
		user.Provider,
		user.IsSubscriber,
		user.SubscriptionTier,
		user.CreatedAt,
		user.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("upsert user: %w", err)
	}

	logs.Debugf("store-users", "upserted user user_id=%s email=%s is_subscriber=%t", user.UserID, user.Email, user.IsSubscriber)
	return nil
}

// UpdateSubscription updates the subscription status for a user
func (r *PostgresUserRepository) UpdateSubscription(userID string, isSubscriber bool, tier string) error {
	const stmt = `
UPDATE users
SET is_subscriber = $2, subscription_tier = NULLIF($3, ''), updated_at = $4
WHERE user_id = $1
`

	result, err := r.db.Exec(stmt, userID, isSubscriber, tier, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("update subscription: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return errors.New("user not found")
	}

	logs.Infof("store-users", "updated subscription user_id=%s is_subscriber=%t tier=%s", userID, isSubscriber, tier)
	return nil
}

// List returns all users (for admin purposes)
func (r *PostgresUserRepository) List() ([]domain.User, error) {
	const query = `
SELECT user_id, email, provider, is_subscriber, subscription_tier, created_at, updated_at
FROM users
ORDER BY created_at DESC
`

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query users: %w", err)
	}
	defer rows.Close()

	users := make([]domain.User, 0)
	for rows.Next() {
		var user domain.User
		var subscriptionTier sql.NullString
		var email sql.NullString

		err := rows.Scan(
			&user.UserID,
			&email,
			&user.Provider,
			&user.IsSubscriber,
			&subscriptionTier,
			&user.CreatedAt,
			&user.UpdatedAt,
		)
		if err != nil {
			logs.Errorf("store-users", "scan user row failed: %v", err)
			continue
		}

		if email.Valid {
			user.Email = email.String
		}
		if subscriptionTier.Valid {
			user.SubscriptionTier = subscriptionTier.String
		}

		users = append(users, user)
	}

	return users, nil
}
