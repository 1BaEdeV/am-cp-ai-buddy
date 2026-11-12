package models

import "time"

// TeamIntroduction хранит отметку стажёра о знакомстве с командой.
// EmployeeID одновременно является первичным ключом,
// поэтому для одного сотрудника может быть только одна отметка.
type TeamIntroduction struct {
	EmployeeID  string    `gorm:"primaryKey"`
	Employee    Employee  `gorm:"foreignKey:EmployeeID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	CompletedAt time.Time `gorm:"not null"`
}
