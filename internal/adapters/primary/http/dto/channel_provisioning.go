package dto

import "time"

type ChannelProvisioning struct {
	ID               UUID      `json:"provisioning_id"`
	BusinessID       UUID      `json:"business_id"`
	Provider         string    `json:"provider"`
	Channel          string    `json:"channel"`
	Status           string    `json:"status"`
	AuthorizationURL string    `json:"authorization_url,omitempty"`
	CreatedAt        time.Time `json:"created_at,omitempty"`
	UpdatedAt        time.Time `json:"updated_at,omitempty"`
}
