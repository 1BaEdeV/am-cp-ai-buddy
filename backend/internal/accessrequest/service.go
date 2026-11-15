// Package accessrequest управляет черновиками, согласованием заявок и выдачей доступа.
package accessrequest

import (
	"time"

	"gorm.io/gorm"
)

type Service struct {
	db  *gorm.DB
	now func() time.Time
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db, now: time.Now}
}
