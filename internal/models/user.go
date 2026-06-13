package models

import "time"

const (
	RoleOwner = "owner"
	RoleAdmin = "admin"
	RoleUser  = "user"
)

type User struct {
	ID            string    `bson:"_id"`
	Balance       int64     `bson:"balance"`
	DebtLimit     int64     `bson:"debt_limit"`
	DebtEnabled   bool      `bson:"debt_enabled"`
	DebtRoleID    string    `bson:"debt_role_id"`
	DebtUpdatedAt time.Time `bson:"debt_updated_at"`
	DebtUpdatedBy string    `bson:"debt_updated_by"`
	Role          string    `bson:"role"`
	CreatedAt     time.Time `bson:"created_at"`
	UpdatedAt     time.Time `bson:"updated_at"`
}
