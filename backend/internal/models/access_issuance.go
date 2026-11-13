package models

import "time"

// AccessIssuanceJob — сохранённое задание на выдачу одного одобренного доступа.
type AccessIssuanceJob struct {
	RequestID   string `gorm:"primaryKey"`
	Request     AccessRequest
	Attempts    int        `gorm:"not null;default:0"`
	AvailableAt time.Time  `gorm:"not null;index"`
	LeaseUntil  *time.Time `gorm:"index"`
	LeaseToken  string     `gorm:"not null;default:''"`
	CompletedAt *time.Time
	CreatedAt   time.Time `gorm:"not null"`
	UpdatedAt   time.Time `gorm:"not null"`
}
