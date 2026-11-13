package models

import "time"

// AccessRequestParameters — копия параметров, показанных сотруднику агентом.
type AccessRequestParameters struct {
	ResourceName       string `gorm:"not null"`
	ResourceService    string `gorm:"not null"`
	ResourceExternalID string `gorm:"not null"`
	AccessLevel        string `gorm:"not null"`
	Purpose            string `gorm:"not null"`
	ApproverName       string `gorm:"not null"`
}

// AccessDraft — подготовленный агентом черновик ровно одного доступа.
type AccessDraft struct {
	ID          string `gorm:"primaryKey"`
	EmployeeID  string `gorm:"not null;index"`
	Employee    Employee
	TeamID      string `gorm:"not null"`
	Team        Team
	PositionID  string `gorm:"not null"`
	Position    Position
	ResourceID  string `gorm:"not null;index"`
	Resource    Resource
	ApproverID  string `gorm:"not null;index"`
	Approver    Employee
	Parameters  AccessRequestParameters `gorm:"embedded;embeddedPrefix:snapshot_"`
	CreatedAt   time.Time               `gorm:"not null"`
	ExpiresAt   time.Time               `gorm:"not null;check:access_draft_expiry_valid,expires_at > created_at"`
	ConfirmedAt *time.Time
}

// AccessRequestStatus — состояние заявки после подтверждения сотрудником.
type AccessRequestStatus string

const (
	AccessRequestPending  AccessRequestStatus = "pending"
	AccessRequestIssuing  AccessRequestStatus = "issuing"
	AccessRequestGranted  AccessRequestStatus = "granted"
	AccessRequestRejected AccessRequestStatus = "rejected"
	AccessRequestFailed   AccessRequestStatus = "failed"
)

// AccessRequest — заявка ровно на один ресурс, созданная из подтверждённого черновика.
type AccessRequest struct {
	ID             string `gorm:"primaryKey"`
	Number         int64  `gorm:"autoIncrement;not null;uniqueIndex:ux_access_requests_number"`
	DraftID        string `gorm:"not null;uniqueIndex:ux_access_requests_draft"`
	Draft          AccessDraft
	EmployeeID     string `gorm:"not null;index;uniqueIndex:ux_access_requests_active_employee_resource,where:status = 'pending' OR status = 'issuing' OR status = 'failed'"`
	Employee       Employee
	TeamID         string `gorm:"not null"`
	Team           Team
	PositionID     string `gorm:"not null"`
	Position       Position
	ResourceID     string `gorm:"not null;index;uniqueIndex:ux_access_requests_active_employee_resource,where:status = 'pending' OR status = 'issuing' OR status = 'failed'"`
	Resource       Resource
	ApproverID     string `gorm:"not null;index"`
	Approver       Employee
	Parameters     AccessRequestParameters `gorm:"embedded;embeddedPrefix:snapshot_"`
	Status         AccessRequestStatus     `gorm:"not null;default:pending;index;check:access_request_status_valid,status IN ('pending','issuing','granted','rejected','failed')"`
	CreatedAt      time.Time               `gorm:"not null"`
	UpdatedAt      time.Time               `gorm:"not null"`
	DecidedAt      *time.Time
	DecisionReason string     `gorm:"not null;default:'';check:access_request_rejection_reason,status <> 'rejected' OR length(trim(decision_reason)) > 0"`
	GrantedAt      *time.Time // Время подтверждённой выдачи; до неё nil.
	LastError      string     `gorm:"not null;default:''"`
}
