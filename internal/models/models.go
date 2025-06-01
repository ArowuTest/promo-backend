package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AdminUserRole enumerates allowed roles.
type AdminUserRole string

const (
	RoleSuperAdmin      AdminUserRole = "SUPERADMIN"
	RoleAdmin           AdminUserRole = "ADMIN"
	RoleSeniorUser      AdminUserRole = "SENIORUSER"
	RoleWinnerReports   AdminUserRole = "WINNERREPORTS"
	RoleAllReportsUser  AdminUserRole = "ALLREPORTS"
)

// UserStatus enumerates user account states.
type UserStatus string

const (
	StatusActive   UserStatus = "Active"
	StatusInactive UserStatus = "Inactive"
	StatusLocked   UserStatus = "Locked"
)

// AdminUser is your user model.
type AdminUser struct {
	ID           uuid.UUID      `gorm:"type:uuid;default:uuid_generate_v4();primaryKey"`
	Username     string         `gorm:"uniqueIndex;not null"`
	Email        string         `gorm:"uniqueIndex;not null"`
	PasswordHash string         `gorm:"not null"`
	Role         AdminUserRole  `gorm:"not null"`
	Status       UserStatus     `gorm:"not null;default:'Active'"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// PrizeStructure represents one “drawing configuration” effective on a given date.
type PrizeStructure struct {
	ID          uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey"`
	EffectiveOn time.Time `gorm:"not null;uniqueIndex"` // only one structure per calendar date

	Currency    string `gorm:"not null;default:'NGN'"` // e.g. “NGN”, “USD”
	// You could also add a “Name” field if needed (e.g. “Weekday Prize” vs “Saturday Prize”)

	Tiers       []PrizeTier `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// PrizeTier is one row in a PrizeStructure. E.g. “Jackpot”, “First Prize”, etc.
type PrizeTier struct {
	ID               uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey"`
	PrizeStructureID uuid.UUID `gorm:"type:uuid;not null;index"`
	TierName         string    `gorm:"not null"` // e.g. “Jackpot” or “First Prize”
	Quantity         int       `gorm:"not null"` // how many winners in this tier
	RunnerUpCount    int       `gorm:"not null"` // how many runner‐ups for this tier
	Amount           int       `gorm:"not null"` // amount in smallest unit, e.g. 10000 = ₦10,000

	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// A “Draw” record, one per date (Mon–Sat) whenever someone clicks “Execute Draw.”
type Draw struct {
	ID               uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey"`
	PrizeStructureID uuid.UUID `gorm:"type:uuid;not null"`
	DrawDate         time.Time `gorm:"not null;index"` // date/time of draw initiation
	EntryCount       int       `gorm:"not null"`      // total number of “entries” (sum of points)

	AdminUserID      uuid.UUID  `gorm:"type:uuid;not null;index"` // who ran the draw
	IsRerun          bool       `gorm:"not null;default:false"`   // true if this is a confirmed re‐run
	OriginalDrawID   *uuid.UUID `gorm:"type:uuid;default:null"`   // points to original Draw.ID if IsRerun==true

	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Winner is one row per winning MSISDN or runner‐up
type Winner struct {
	ID           uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey"`
	DrawID       uuid.UUID `gorm:"type:uuid;index;not null"`
	MSISDN       string    `gorm:"not null"` // store full MSISDN in DB
	MaskedMSISDN string    `gorm:"not null"` // “080XXXYYYZZ” (first 3/last 3)
	PrizeTier    string    `gorm:"not null"` // e.g. “Jackpot” or “First Prize”
	Position     string    `gorm:"not null"` // “Winner” vs “RunnerUp”
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// EligibleEntry is what the PostHog layer (or CSV fallback) returns:
// one row per MSISDN with a positive integer number of “entries” or points.
type EligibleEntry struct {
	MSISDN string
	Points int
}

// Migrate will create/update your tables
func Migrate(db *gorm.DB) {
	db.AutoMigrate(
		&AdminUser{},
		&PrizeStructure{},
		&PrizeTier{},
		&Draw{},
		&Winner{},
	)
}
