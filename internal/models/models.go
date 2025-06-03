// internal/models/models.go

package models

import (
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// -----------------------------------------------------------
// 1) AdminUser
// -----------------------------------------------------------

// AdminUserRole enumerates allowed roles.
type AdminUserRole string

const (
	RoleSuperAdmin     AdminUserRole = "SUPERADMIN"
	RoleAdmin          AdminUserRole = "ADMIN"
	RoleSeniorUser     AdminUserRole = "SENIORUSER"
	RoleWinnerReports  AdminUserRole = "WINNERREPORTS"
	RoleAllReportsUser AdminUserRole = "ALLREPORTS"
)

// UserStatus enumerates user account states.
type UserStatus string

const (
	StatusActive   UserStatus = "Active"
	StatusInactive UserStatus = "Inactive"
	StatusLocked   UserStatus = "Locked"
)

// AdminUser is the user model for admins.
type AdminUser struct {
	ID           uuid.UUID     `gorm:"type:uuid;default:uuid_generate_v4();primaryKey"`
	Username     string        `gorm:"uniqueIndex;not null"`
	Email        string        `gorm:"uniqueIndex;not null"`
	PasswordHash string        `gorm:"not null"`
	Role         AdminUserRole `gorm:"not null"`
	Status       UserStatus    `gorm:"not null;default:'Active'"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// HashPassword hashes a plaintext password using bcrypt.
func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}

// -----------------------------------------------------------
// 2) EligibleEntry & WeightedEntry (for RNG logic)
// -----------------------------------------------------------

// EligibleEntry represents one MSISDN’s total points in a given draw window.
type EligibleEntry struct {
	MSISDN string `json:"msisdn"`
	Points int    `json:"points"`
}

// WeightedEntry is an internal struct used by the RNG engine.
//  - MSISDN: the phone number string.
//  - Weight: the number of “tickets” (points).
//  - CumSum: cumulative sum up to this index.
type WeightedEntry struct {
	MSISDN string
	Weight int
	CumSum int
}

// -----------------------------------------------------------
// 3) PrizeStructure & PrizeTier
// -----------------------------------------------------------

// PrizeStructure represents one active set of prizes (effective for a range of dates).
// We assume one structure per day; extend with EffectiveFrom/EffectiveTo if needed later.
type PrizeStructure struct {
	ID        uuid.UUID   `gorm:"type:uuid;default:uuid_generate_v4();primaryKey"`
	Effective time.Time   `gorm:"not null;index"` // e.g. “2025-06-01” for that day’s structure
	CreatedAt time.Time
	UpdatedAt time.Time

	Name  string      `gorm:"not null"`                           // e.g. "Daily Draw – June 2025"
	Tiers []PrizeTier `gorm:"foreignKey:PrizeStructureID;constraint:OnDelete:CASCADE"`
}

// PrizeTier defines one tier within a PrizeStructure (e.g. “Jackpot”, “1st Consolation”).
type PrizeTier struct {
	ID               uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey"`
	PrizeStructureID uuid.UUID `gorm:"type:uuid;not null;index"`
	TierName         string    `gorm:"not null"`                     // "Jackpot", "Consolation", etc.
	Amount           int       `gorm:"not null"`                     // prize value in Naira
	Quantity         int       `gorm:"not null;default:1"`           // how many main winners
	RunnerUpCount    int       `gorm:"not null;default:0"`           // how many runner‐ups
	OrderIndex       int       `gorm:"not null;index"`               // lower = higher priority (1=Jackpot)
}

// -----------------------------------------------------------
// 4) Draw & Winner
// -----------------------------------------------------------

// Draw represents one execution of a draw (daily Mon–Sat).
type Draw struct {
	ID               uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey"`
	DrawDate         time.Time `gorm:"not null;index"`       // e.g. “2025-06-01T00:00:00Z”
	AdminUserID      uuid.UUID `gorm:"type:uuid;not null"`   // which admin triggered
	PrizeStructureID uuid.UUID `gorm:"type:uuid;not null"`   // which structure applied
	TotalEntries     int       `gorm:"not null;default:0"`   // total “tickets” in pool
	IsRerun          bool      `gorm:"not null;default:false"`
	CreatedAt        time.Time
	UpdatedAt        time.Time

	// Winners: one‐to‐many, keyed by DrawID
	Winners []Winner `gorm:"foreignKey:DrawID;constraint:OnDelete:CASCADE"`
}

// Winner stores each MSISDN that actually won (main winners + runner‐ups).
type Winner struct {
	ID          uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey"`
	DrawID      uuid.UUID `gorm:"type:uuid;not null;index"`     // parent draw
	PrizeTierID uuid.UUID `gorm:"type:uuid;not null;index"`     // FK to PrizeTier

	// PrizeTier relationship so we can Preload and get TierName
	PrizeTier   PrizeTier `gorm:"foreignKey:PrizeTierID"`

	MSISDN     string    `gorm:"not null"` // full MSISDN (audit)
	Position   int       `gorm:"not null"` // 1=first main winner, 2=second, then runner‐ups
	IsRunnerUp bool      `gorm:"not null;default:false"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// -----------------------------------------------------------
// 5) Auto‐Migrate helper
// -----------------------------------------------------------

// Migrate will create/update your tables in the correct order.
func Migrate(db *gorm.DB) {
	db.AutoMigrate(
		&AdminUser{},
		&PrizeStructure{},
		&PrizeTier{},
		&Draw{},
		&Winner{},
	)
}
