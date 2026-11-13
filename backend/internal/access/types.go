package access

import (
	"errors"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
)

var (
	ErrRequirementsNotConfigured = errors.New("для команды и должности не настроены обязательные доступы")
	ErrInvalidRequirement        = errors.New("правило доступа настроено некорректно")
)

type ResourceStatus string

const (
	StatusRequestable ResourceStatus = "requestable"
	StatusGranted     ResourceStatus = "granted"
	StatusPending     ResourceStatus = "pending"
	StatusIssuing     ResourceStatus = "issuing"
	StatusFailed      ResourceStatus = "failed"
)

// WorkingAccesses содержит обязательные ресурсы стажёра и прогресс получения доступов.
type WorkingAccesses struct {
	Resources []RequiredResource `json:"resources"`
	Progress  Progress           `json:"progress"`
}

// Progress считает подтверждённые ресурсы из общего числа обязательных.
type Progress struct {
	Completed  int `json:"completed"`
	Total      int `json:"total"`
	InProgress int `json:"inProgress"`
	Failed     int `json:"failed"`
}

// RequiredResource описывает требуемый ресурс для карточки «Рабочие доступы».
type RequiredResource struct {
	ID          string          `json:"id"`
	Service     string          `json:"service"`
	Name        string          `json:"name"`
	AccessLevel string          `json:"accessLevel"`
	Purpose     string          `json:"purpose"`
	Status      ResourceStatus  `json:"status"`
	CanRequest  bool            `json:"canRequest"`
	Request     *RequestSummary `json:"request,omitempty"`
}

type RequestSummary struct {
	ID     string                     `json:"id"`
	Number int64                      `json:"number"`
	Status models.AccessRequestStatus `json:"status"`
}

// ActiveRequestStatuses возвращает состояния, блокирующие новую заявку на ресурс.
// Каждый вызов возвращает отдельный срез, чтобы вызывающий код не менял общие правила.
func ActiveRequestStatuses() []models.AccessRequestStatus {
	return []models.AccessRequestStatus{
		models.AccessRequestPending, models.AccessRequestIssuing, models.AccessRequestFailed,
	}
}
