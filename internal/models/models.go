package models

import (
	"time"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type AdminUserRole string
const (
	RoleSuperAdmin     AdminUserRole = "SUPERADMIN"
	RoleAdmin          AdminUserRole = "ADMIN"
	RoleSeniorUser     AdminUserRole = "SENIORUSER"
)

type UserStatus string
const (
	StatusActive   UserStatus = "Active"
	StatusInactive UserStatus = "Inactive"
)

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

func HashPassword(pw string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(pw), 14)
	return string(bytes), err
}

type EligibleEntry struct {
	MSISDN string
	Points int
}

type WeightedEntry struct {
	MSISDN string
	Weight int
	CumSum int
}

type PrizeStructure struct {
	ID           uuid.UUID      `gorm:"type:uuid;default:uuid_generate_v4();primaryKey"`
	Name         string         `gorm:"not null"`
	Effective    time.Time      `gorm:"not null;index"`
	EligibleDays pq.StringArray `gorm:"type:text[]"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Tiers        []PrizeTier `gorm:"foreignKey:PrizeStructureID;constraint:OnDelete:CASCADE"`
}

type PrizeTier struct {
	ID               uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey"`
	PrizeStructureID uuid.UUID `gorm:"type:uuid;not null;index"`
	TierName         string    `gorm:"not null"`
	Amount           int       `gorm:"not null"`
	Quantity         int       `gorm:"not null;default:1"`
	RunnerUpCount    int       `gorm:"not null;default:0"`
	OrderIndex       int       `gorm:"not null;index"`
}

type Draw struct {
	ID               uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey"`
	DrawDate         time.Time `gorm:"not null;index"`
	AdminUserID      uuid.UUID `gorm:"type:uuid;not null"`
	AdminUser        AdminUser `gorm:"foreignKey:AdminUserID"`
	PrizeStructureID uuid.UUID `gorm:"type:uuid;not null"`
	TotalEntries     int       `gorm:"not null;default:0"`
	Source           string    `gorm:"not null;default:'PostHog'"`
	IsRerun          bool      `gorm:"not null;default:false"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
	Winners          []Winner `gorm:"foreignKey:DrawID;constraint:OnDelete:CASCADE"`
}

type Winner struct {
	ID          uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey"`
	DrawID      uuid.UUID `gorm:"type:uuid;not null;index"`
	PrizeTierID uuid.UUID `gorm:"type:uuid;not null;index"`
	PrizeTier   PrizeTier `gorm:"foreignKey:PrizeTierID"`
	MSISDN      string    `gorm:"not null"`
	Position    int       `gorm:"not null"`
	IsRunnerUp  bool      `gorm:"not null;default:false"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func Migrate(db *gorm.DB) {
	db.AutoMigrate(&AdminUser{}, &PrizeStructure{}, &PrizeTier{}, &Draw{}, &Winner{})
}