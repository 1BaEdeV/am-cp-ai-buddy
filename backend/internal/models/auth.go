package models

import "time"

// EmployeeRole определяет права сотрудника в приложении.
type EmployeeRole string

const (
	RoleIntern  EmployeeRole = "intern"
	RoleManager EmployeeRole = "manager"
)

// Employee — запись в справочнике сотрудников. Она может существовать до регистрации.
type Employee struct {
	ID         string `gorm:"primaryKey"`
	Email      string `gorm:"not null;uniqueIndex:ux_employees_email,expression:lower(email)"`
	FirstName  string `gorm:"not null"`
	SecondName string `gorm:"not null"`
	PositionID string `gorm:"not null"`
	Position   Position
	TeamID     string `gorm:"not null"`
	Team       Team
	Role       EmployeeRole `gorm:"not null;check:employee_role_valid,role IN ('intern','manager')"`
}

// Position — должность из справочника. На неё могут ссылаться несколько сотрудников.
type Position struct {
	ID   string `gorm:"primaryKey"`
	Name string `gorm:"not null"`
}

// Team — назначенная сотруднику команда из справочника. По ней подбираются наставники.
type Team struct {
	ID          string `gorm:"primaryKey"`
	Name        string `gorm:"not null"`
	Description string `gorm:"not null;default:''"`
	Mentors     []Mentor
}

// Mentor — наставник, закреплённый за одной командой.
type Mentor struct {
	ID         string `gorm:"primaryKey"`
	TeamID     string `gorm:"not null;index"`
	FirstName  string `gorm:"not null"`
	SecondName string `gorm:"not null"`
	Contact    string `gorm:"not null;default:''"`
}

// Profile — профиль сотрудника в приложении. Для стажёра хранит наставника
// и дату начала стажировки. Команда и роль берутся из записи Employee.
type Profile struct {
	ID                  string `gorm:"primaryKey"`
	EmployeeID          string `gorm:"not null;uniqueIndex"`
	Employee            Employee
	MentorID            *string    // ID наставника в базе. nil, если наставника нет.
	Mentor              *Mentor    // Данные наставника. nil, если запись не загружена или наставника нет.
	InternshipStartDate *time.Time `gorm:"type:date"` // nil, если дата начала стажировки отсутствует.
}
