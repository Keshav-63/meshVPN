package domain

import "time"

type User struct {
	UserID           string    `json:"user_id"`
	Email            string    `json:"email"`
	Provider         string    `json:"provider"`
	IsSubscriber     bool      `json:"is_subscriber"`
	SubscriptionTier string    `json:"subscription_tier,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}
