package models

import "time"

// Resource — рабочий ресурс, например репозиторий GitLab или проект YouTrack.
type Resource struct {
	ID           string   `gorm:"primaryKey"`
	Name         string   `gorm:"not null"`
	Service      string   `gorm:"not null;uniqueIndex:ux_resources_service_external"` // Сервис, через адаптер которого проверяется и выдаётся доступ.
	ExternalID   string   `gorm:"not null;uniqueIndex:ux_resources_service_external"` // ID ресурса в тестовом или корпоративном сервисе.
	AccessLevels []string `gorm:"serializer:json;type:jsonb;not null"`
}

// AccessRequirement — обязательный доступ для стажёра определённой команды и должности.
type AccessRequirement struct {
	ID          string `gorm:"primaryKey"`
	TeamID      string `gorm:"not null;uniqueIndex:ux_access_requirements_team_position_resource"`
	Team        Team
	PositionID  string `gorm:"not null;uniqueIndex:ux_access_requirements_team_position_resource;index"`
	Position    Position
	ResourceID  string `gorm:"not null;uniqueIndex:ux_access_requirements_team_position_resource;index"`
	Resource    Resource
	AccessLevel string `gorm:"not null"`
	Purpose     string `gorm:"not null"`
	ApproverID  string `gorm:"not null;index"`
	Approver    Employee
}

// EmployeeAccess — подтверждённый доступ сотрудника к конкретному ресурсу.
type EmployeeAccess struct {
	ID          string `gorm:"primaryKey"`
	EmployeeID  string `gorm:"not null;uniqueIndex:ux_employee_accesses_employee_resource"`
	Employee    Employee
	ResourceID  string `gorm:"not null;uniqueIndex:ux_employee_accesses_employee_resource;index"`
	Resource    Resource
	AccessLevel string    `gorm:"not null"`
	VerifiedAt  time.Time `gorm:"not null"`
}
