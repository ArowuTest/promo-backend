// internal/models/models.go

package models

import (
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// ────────────────────────────────────────────────────────────────────────────────
// AdminUser & Roles & Status
// ────────────────────────────────────────────────────────────────────────────────

// AdminUserRole enumerates allowed roles in the admin portal.
type AdminUserRole string

const (
	RoleSuperAdmin     AdminUserRole = "SUPERADMIN"
	RoleAdmin          AdminUserRole = "ADMIN"
	RoleSeniorUser     AdminUserRole = "SENIORUSER"
	RoleWinnerReports  AdminUserRole = "WINNERREPORTS"
	RoleAllReportsUser AdminUserRole = "ALLREPORTS"
)

// UserStatus enumerates admin‐user account states.
type UserStatus string

const (
	StatusActive   UserStatus = "Active"
	StatusInactive UserStatus = "Inactive"
	StatusLocked   UserStatus = "Locked"
)

// AdminUser is the GORM model for an admin‐portal user.
type AdminUser struct {
	ID           uuid.UUID      `gorm:"type:uuid;default:uuid_generate_v4();primaryKey" json:"id"`
	Username     string         `gorm:"uniqueIndex;not null"            json:"username"`
	Email        string         `gorm:"uniqueIndex;not null"            json:"email"`
	PasswordHash string         `gorm:"not null"                        json:"-"`
	Role         AdminUserRole  `gorm:"not null"                        json:"role"`
	Status       UserStatus     `gorm:"not null;default:'Active'"       json:"status"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

// BeforeCreate hook: generate a new UUID if missing
func (u *AdminUser) BeforeCreate(tx *gorm.DB) (err error) {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	return
}

// HashPassword is a helper to hash a plaintext password.
func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}

// ────────────────────────────────────────────────────────────────────────────────
// PrizeStructure & PrizeStructureItem
// ────────────────────────────────────────────────────────────────────────────────

// PrizeStructure represents one date’s set of prize‐tiers and runner‐ups.
type PrizeStructure struct {
	ID         uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey" json:"id"`
	Effective  time.Time `gorm:"not null"                            json:"effective"`   // date/time when this structure applies
	IsSaturday bool      `gorm:"not null"                            json:"is_saturday"` // true if a Saturday (weekly) structure
	Items      []PrizeStructureItem `gorm:"constraint:OnDelete:CASCADE" json:"items"`
	CreatedAt  time.Time            `json:"created_at"`
	UpdatedAt  time.Time            `json:"updated_at"`
}

// PrizeStructureItem is one row in the PrizeStructure (e.g. “Jackpot”, “First Prize”).
type PrizeStructureItem struct {
	ID                 uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey" json:"id"`
	PrizeStructureID   uuid.UUID `gorm:"type:uuid;not null;index"                    json:"prize_structure_id"`
	PrizeName          string    `gorm:"not null"                                    json:"prize_name"`
	Quantity           int       `gorm:"not null"                                    json:"quantity"`
	RunnerUpCount      int       `gorm:"not null"                                    json:"runner_up_count"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// BeforeCreate hook for PrizeStructure & PrizeStructureItem to assign UUID if missing
func (p *PrizeStructure) BeforeCreate(tx *gorm.DB) (err error) {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return
}
func (item *PrizeStructureItem) BeforeCreate(tx *gorm.DB) (err error) {
	if item.ID == uuid.Nil {
		item.ID = uuid.New()
	}
	return
}

// ────────────────────────────────────────────────────────────────────────────────
// Draw & Winner
// ────────────────────────────────────────────────────────────────────────────────

// Draw records one execution of a draw (daily or weekly).
// It keeps a snapshot of PrizeStructureID, the window, etc.
type Draw struct {
	ID               uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey" json:"id"`
	ExecutedAt       time.Time `gorm:"not null"                                        json:"executed_at"`
	PrizeStructureID uuid.UUID `gorm:"type:uuid;not null;index"                        json:"prize_structure_id"`
	EntryCount       int       `gorm:"not null"                                        json:"entry_count"`
	IsRerun          bool      `gorm:"not null;default:false"                          json:"is_rerun"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`

	Winners []Winner `gorm:"constraint:OnDelete:CASCADE" json:"winners"`
}

// Winner records one winner (or runner‐up) in a specific draw.
type Winner struct {
	ID           uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey" json:"id"`
	DrawID       uuid.UUID `gorm:"type:uuid;not null;index"                        json:"draw_id"`
	PrizeTier    string    `gorm:"not null"                                        json:"prize_tier"`
	Position     int       `gorm:"not null"                                        json:"position"`    // 1..Quantity+RunnerUps
	MaskFirst3   string    `gorm:"not null"                                        json:"mask_first3"`
	MaskLast3    string    `gorm:"not null"                                        json:"mask_last3"`
	MSISDN       string    `gorm:"not null"                                        json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// BeforeCreate hook to assign UUIDs if missing
func (d *Draw) BeforeCreate(tx *gorm.DB) (err error) {
	if d.ID == uuid.Nil {
		d.ID = uuid.New()
	}
	return
}
func (w *Winner) BeforeCreate(tx *gorm.DB) (err error) {
	if w.ID == uuid.Nil {
		w.ID = uuid.New()
	}
	return
}

// ────────────────────────────────────────────────────────────────────────────────
// EligibleEntry & WeightedEntry for RNG
// ────────────────────────────────────────────────────────────────────────────────

// EligibleEntry is one MSISDN + its total points in the eligibility window.
// This comes either from PostHog or from the CSV fallback.
type EligibleEntry struct {
	MSISDN string
	Points int
}

// WeightedEntry is used internally to build a running cumulative‐weight list for RNG.
type WeightedEntry struct {
	MSISDN     string
	Weight     int // the “points” for that MSISDN
	Cumulative int // the running total up to and including this entry
}

// ────────────────────────────────────────────────────────────────────────────────
// Migrate: run AutoMigrate on all tables
// ────────────────────────────────────────────────────────────────────────────────

func Migrate(db *gorm.DB) {
	db.AutoMigrate(
		&AdminUser{},
		&PrizeStructure{},
		&PrizeStructureItem{},
		&Draw{},
		&Winner{},
	)
}
