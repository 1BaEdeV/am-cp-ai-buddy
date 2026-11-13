package accessrequest

import (
	"context"
	"errors"
	"time"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
)

var (
	ErrInvalidInput            = errors.New("некорректные параметры операции с доступом")
	ErrResourceNotRequired     = errors.New("ресурс не предусмотрен для команды и должности сотрудника")
	ErrDraftNotFound           = errors.New("черновик не найден")
	ErrDraftExpired            = errors.New("срок действия черновика истёк")
	ErrDraftChanged            = errors.New("условия изменились: подготовь и подтверди новый черновик")
	ErrAccessAlreadyGranted    = errors.New("необходимый доступ уже есть")
	ErrActiveRequestExists     = errors.New("заявка на этот ресурс уже находится в работе")
	ErrRequestNotFound         = errors.New("заявка не найдена")
	ErrRequestAlreadyDecided   = errors.New("руководитель уже принял другое решение по заявке")
	ErrInvalidRequestState     = errors.New("действие недоступно в текущем состоянии заявки")
	ErrIssuanceLeaseLost       = errors.New("задание на выдачу уже передано другому обработчику")
	ErrIssuanceUnconfirmed     = errors.New("сервис не подтвердил необходимый уровень доступа")
	ErrIssuanceResourceChanged = errors.New("внешний ресурс изменился после подтверждения заявки")
)

const DraftTTL = 15 * time.Minute

type PreparationStatus string

const (
	PreparationReady          PreparationStatus = "draft"
	PreparationAlreadyGranted PreparationStatus = "already_granted"
	PreparationRequestExists  PreparationStatus = "request_exists"
)

// RequestParameters содержит именно показанные сотруднику значения, а не текущий справочник.
type RequestParameters struct {
	ResourceName       string `json:"resourceName"`
	ResourceService    string `json:"resourceService"`
	ResourceExternalID string `json:"resourceExternalId"`
	AccessLevel        string `json:"accessLevel"`
	Purpose            string `json:"purpose"`
	ApproverName       string `json:"approverName"`
}

type Draft struct {
	ID         string            `json:"id"`
	ResourceID string            `json:"resourceId"`
	ApproverID string            `json:"approverId"`
	Parameters RequestParameters `json:"parameters"`
	CreatedAt  time.Time         `json:"createdAt"`
	ExpiresAt  time.Time         `json:"expiresAt"`
}

// Preparation не создаёт черновик, если доступ уже есть или заявка находится в работе.
type Preparation struct {
	Status  PreparationStatus `json:"status"`
	Draft   *Draft            `json:"draft,omitempty"`
	Request *Request          `json:"request,omitempty"`
}

type Request struct {
	ID             string                     `json:"id"`
	Number         int64                      `json:"number"`
	DraftID        string                     `json:"draftId"`
	EmployeeID     string                     `json:"employeeId"`
	ResourceID     string                     `json:"resourceId"`
	ApproverID     string                     `json:"approverId"`
	Parameters     RequestParameters          `json:"parameters"`
	Status         models.AccessRequestStatus `json:"status"`
	CreatedAt      time.Time                  `json:"createdAt"`
	UpdatedAt      time.Time                  `json:"updatedAt"`
	DecidedAt      *time.Time                 `json:"decidedAt,omitempty"`
	DecisionReason string                     `json:"decisionReason,omitempty"`
	GrantedAt      *time.Time                 `json:"grantedAt,omitempty"`
	LastError      string                     `json:"lastError,omitempty"`
}

type Decision string

const (
	DecisionApprove Decision = "approve"
	DecisionReject  Decision = "reject"
)

type DecisionInput struct {
	Decision Decision `json:"decision"`
	Reason   string   `json:"reason"`
}

// RequestFilter: для стажёра Group — all, active или completed; Status уточняет фильтр.
// Для руководителя учитываются только Limit и Offset: очередь всегда содержит pending.
type RequestFilter struct {
	Group      string
	Status     models.AccessRequestStatus
	ResourceID string
	Limit      int
	Offset     int
}

// RequestCounts считает свои заявки стажёра независимо от фильтра списка.
// Для руководителя считает только назначенные ему pending из очереди согласования.
type RequestCounts struct {
	Total     int64 `json:"total"`
	Active    int64 `json:"active"`
	Completed int64 `json:"completed"`
	Pending   int64 `json:"pending"`
	Issuing   int64 `json:"issuing"`
	Granted   int64 `json:"granted"`
	Rejected  int64 `json:"rejected"`
	Failed    int64 `json:"failed"`
}

type RequestList struct {
	Items  []Request     `json:"items"`
	Total  int64         `json:"total"`
	Counts RequestCounts `json:"counts"`
	Limit  int           `json:"limit"`
	Offset int           `json:"offset"`
}

// GrantTarget использует утверждённый снимок внешнего ресурса.
type GrantTarget struct {
	EmployeeID  string
	Service     string
	ExternalID  string
	AccessLevel string
}

type ProviderAccess struct {
	Granted     bool
	AccessLevel string
}

// AccessProvider должен соблюдать контекст и идемпотентность по operationID:
type AccessProvider interface {
	CheckAccess(ctx context.Context, target GrantTarget) (ProviderAccess, error)
	GrantAccess(ctx context.Context, operationID string, target GrantTarget) error
}
