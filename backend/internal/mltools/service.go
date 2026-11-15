// Package mltools предоставляет операции агенту в контексте текущего сотрудника.
package mltools

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/access"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/accessrequest"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/auth"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"gorm.io/gorm"
)

var ErrResourceChanged = errors.New("параметры ресурса изменились во время проверки")

type employeeKey struct{}

// WithEmployeeID вызывается серверным адаптером, а не на основании аргументов LLM.
func WithEmployeeID(ctx context.Context, employeeID string) context.Context {
	return context.WithValue(ctx, employeeKey{}, strings.TrimSpace(employeeID))
}

// EmployeeID читает установленный серверным адаптером контекст сотрудника.
func EmployeeID(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	id, _ := ctx.Value(employeeKey{}).(string)
	if id == "" {
		return "", auth.ErrEmployeeNotFound
	}
	return id, nil
}

type Resources interface {
	ListRequiredAccesses(context.Context, string) (access.WorkingAccesses, error)
}

type Requests interface {
	PrepareAccessRequest(context.Context, string, string) (accessrequest.Preparation, error)
	GetAccessRequest(context.Context, string, string) (accessrequest.Request, error)
}

type Service struct {
	db        *gorm.DB
	resources Resources
	requests  Requests
	provider  accessrequest.AccessProvider
}

func NewService(db *gorm.DB, resources Resources, requests Requests, provider accessrequest.AccessProvider) *Service {
	return &Service{db: db, resources: resources, requests: requests, provider: provider}
}

func (s *Service) GetRoleTools(ctx context.Context) (access.WorkingAccesses, error) {
	id, err := EmployeeID(ctx)
	if err != nil {
		return access.WorkingAccesses{}, err
	}
	return s.resources.ListRequiredAccesses(ctx, id)
}

type ToolAccess struct {
	ResourceID          string `json:"resourceId"`
	RequiredAccessLevel string `json:"requiredAccessLevel"`
	Granted             bool   `json:"granted"`
	AccessLevel         string `json:"accessLevel"`
	Sufficient          bool   `json:"sufficient"`
}

// CheckToolAccess проверяет фактические права только на обязательный ресурс.
// Сетевой вызов выполняется после проверок роли и профиля, без транзакции БД.
// Операция не выдаёт права, не создаёт заявки и не изменяет EmployeeAccess.
func (s *Service) CheckToolAccess(ctx context.Context, resourceID string) (ToolAccess, error) {
	id, err := EmployeeID(ctx)
	if err != nil {
		return ToolAccess{}, err
	}
	resourceID = strings.TrimSpace(resourceID)
	if resourceID == "" {
		return ToolAccess{}, accessrequest.ErrInvalidInput
	}
	required, err := s.resources.ListRequiredAccesses(ctx, id)
	if err != nil {
		return ToolAccess{}, err
	}
	index := slices.IndexFunc(required.Resources, func(r access.RequiredResource) bool { return r.ID == resourceID })
	if index < 0 {
		return ToolAccess{}, accessrequest.ErrResourceNotRequired
	}
	rule := required.Resources[index]
	var resource models.Resource
	err = s.db.WithContext(ctx).Take(&resource, "id = ?", resourceID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ToolAccess{}, accessrequest.ErrResourceNotRequired
	}
	if err != nil {
		return ToolAccess{}, fmt.Errorf("прочитать ресурс инструмента агента: %w", err)
	}
	if resource.Service != rule.Service || !slices.Contains(resource.AccessLevels, rule.AccessLevel) {
		return ToolAccess{}, ErrResourceChanged
	}
	actual, err := s.provider.CheckAccess(ctx, accessrequest.GrantTarget{EmployeeID: id, Service: resource.Service, ExternalID: resource.ExternalID, AccessLevel: rule.AccessLevel})
	if err != nil {
		return ToolAccess{}, err
	}
	return ToolAccess{ResourceID: resourceID, RequiredAccessLevel: rule.AccessLevel, Granted: actual.Granted, AccessLevel: actual.AccessLevel,
		Sufficient: actual.Granted && access.HasRequiredLevel(resource.Service, actual.AccessLevel, rule.AccessLevel)}, nil
}

func (s *Service) PrepareAccessRequest(ctx context.Context, resourceID string) (accessrequest.Preparation, error) {
	id, err := EmployeeID(ctx)
	if err != nil {
		return accessrequest.Preparation{}, err
	}
	return s.requests.PrepareAccessRequest(ctx, id, resourceID)
}

func (s *Service) GetAccessRequestStatus(ctx context.Context, requestID string) (accessrequest.Request, error) {
	id, err := EmployeeID(ctx)
	if err != nil {
		return accessrequest.Request{}, err
	}
	return s.requests.GetAccessRequest(ctx, id, requestID)
}
