package auth

import "time"

type Role string

const (
	RoleAdmin      Role = "admin"
	RoleContestant Role = "contestant"
)

type User struct {
	ID                     int64
	Username               string
	PasswordHash           string
	Role                   Role
	CreatedAt              time.Time
	FixedSlot              int
	PasswordChangeRequired bool
	Disabled               bool
}

type UserDTO struct {
	ID                     int64  `json:"id"`
	Username               string `json:"username"`
	Role                   Role   `json:"role"`
	FixedSlot              int    `json:"fixed_slot,omitempty"`
	PasswordChangeRequired bool   `json:"password_change_required,omitempty"`
	CreatedAt              string `json:"created_at"`
	Disabled               bool   `json:"disabled"`
}

func (u User) DTO() UserDTO {
	return UserDTO{ID: u.ID, Username: u.Username, Role: u.Role, FixedSlot: u.FixedSlot, PasswordChangeRequired: u.PasswordChangeRequired, CreatedAt: u.CreatedAt.UTC().Format(time.RFC3339), Disabled: u.Disabled}
}

type Session struct {
	ID        string
	UserID    int64
	Username  string
	Role      Role
	FixedSlot int
	IPAddress string
	UserAgent string
	CreatedAt time.Time
	LastSeen  time.Time
	ExpiresAt time.Time
}

type SessionDTO struct {
	ID        string `json:"id"`
	UserID    int64  `json:"user_id"`
	Username  string `json:"username"`
	Role      Role   `json:"role"`
	FixedSlot int    `json:"fixed_slot,omitempty"`
	IPAddress string `json:"ip_address"`
	UserAgent string `json:"user_agent"`
	CreatedAt string `json:"created_at"`
	LastSeen  string `json:"last_seen_at"`
	ExpiresAt string `json:"expires_at"`
	Online    bool   `json:"online"`
	Current   bool   `json:"current"`
}
