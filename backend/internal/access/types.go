package access

import "errors"

var (
	ErrRequirementsNotConfigured = errors.New("для команды и должности не настроены обязательные доступы")
	ErrInvalidRequirement        = errors.New("правило доступа настроено некорректно")
)

type ResourceStatus string

const (
	StatusRequestable ResourceStatus = "requestable"
	StatusGranted     ResourceStatus = "granted"
)

// WorkingAccesses содержит обязательные ресурсы стажёра и прогресс получения доступов.
type WorkingAccesses struct {
	Resources []RequiredResource `json:"resources"`
	Progress  Progress           `json:"progress"`
}

// Progress считает подтверждённые ресурсы из общего числа обязательных.
type Progress struct {
	Completed int `json:"completed"`
	Total     int `json:"total"`
}

// RequiredResource описывает требуемый ресурс для карточки «Рабочие доступы».
type RequiredResource struct {
	ID          string         `json:"id"`
	Service     string         `json:"service"`
	Name        string         `json:"name"`
	AccessLevel string         `json:"accessLevel"`
	Purpose     string         `json:"purpose"`
	Status      ResourceStatus `json:"status"`
	CanRequest  bool           `json:"canRequest"`
}
