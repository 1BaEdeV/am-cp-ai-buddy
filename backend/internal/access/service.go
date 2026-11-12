package access

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/auth"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"gorm.io/gorm"
)

// Service подбирает обязательные ресурсы по команде и должности стажёра.
type Service struct {
	db *gorm.DB
}

// NewService принимает общее подключение к базе приложения.
func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// ListRequiredAccesses возвращает требования и сохранённое состояние подтверждённых доступов.
func (s *Service) ListRequiredAccesses(ctx context.Context, employeeID string) (WorkingAccesses, error) {
	employeeID = strings.TrimSpace(employeeID)
	if employeeID == "" {
		return WorkingAccesses{}, auth.ErrEmployeeNotFound
	}
	var result WorkingAccesses
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var employee models.Employee
		err := tx.Take(&employee, "id = ?", employeeID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return auth.ErrEmployeeNotFound
		}
		if err != nil {
			return fmt.Errorf("найти сотрудника для рабочих доступов: %w", err)
		}
		if employee.Role != models.RoleIntern {
			return auth.ErrForbidden
		}

		var profile models.Profile
		err = tx.Select("id").Take(&profile, "employee_id = ?", employee.ID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return auth.ErrProfileRequired
		}
		if err != nil {
			return fmt.Errorf("проверить профиль стажёра: %w", err)
		}

		var requirements []models.AccessRequirement
		err = tx.Preload("Resource").Preload("Approver").
			Where("team_id = ? AND position_id = ?", employee.TeamID, employee.PositionID).
			Order("resource_id").Find(&requirements).Error
		if err != nil {
			return fmt.Errorf("загрузить обязательные доступы: %w", err)
		}
		if len(requirements) == 0 {
			return ErrRequirementsNotConfigured
		}

		resourceIDs := make([]string, 0, len(requirements))
		for _, requirement := range requirements {
			if requirement.Resource.ID == "" || requirement.AccessLevel == "" ||
				!slices.Contains(requirement.Resource.AccessLevels, requirement.AccessLevel) ||
				requirement.Approver.Role != models.RoleManager || requirement.Approver.TeamID != employee.TeamID {
				return fmt.Errorf("правило %s: %w", requirement.ID, ErrInvalidRequirement)
			}
			resourceIDs = append(resourceIDs, requirement.ResourceID)
		}

		var accesses []models.EmployeeAccess
		if err := tx.Where("employee_id = ? AND resource_id IN ?", employee.ID, resourceIDs).
			Find(&accesses).Error; err != nil {
			return fmt.Errorf("загрузить подтверждённые доступы: %w", err)
		}
		accessByResource := make(map[string]models.EmployeeAccess, len(accesses))
		for _, access := range accesses {
			accessByResource[access.ResourceID] = access
		}

		result.Resources = make([]RequiredResource, 0, len(requirements))
		result.Progress.Total = len(requirements)
		for _, requirement := range requirements {
			resource := RequiredResource{
				ID:          requirement.ResourceID,
				Service:     requirement.Resource.Service,
				Name:        requirement.Resource.Name,
				AccessLevel: requirement.AccessLevel,
				Purpose:     requirement.Purpose,
				Status:      StatusRequestable,
				CanRequest:  true,
			}
			access, found := accessByResource[requirement.ResourceID]
			if found && !access.VerifiedAt.IsZero() &&
				hasRequiredLevel(resource.Service, access.AccessLevel, requirement.AccessLevel) {
				resource.Status = StatusGranted
				resource.CanRequest = false
				result.Progress.Completed++
			}
			result.Resources = append(result.Resources, resource)
		}
		return nil
	}, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return WorkingAccesses{}, err
	}
	return result, nil
}

// hasRequiredLevel учитывает одинаковые уровни и включение прав в тестовых сервисах MVP.
func hasRequiredLevel(service, actual, required string) bool {
	if actual == required {
		return true
	}
	switch service {
	case "gitlab":
		return actual == "Developer" && required == "Reporter"
	case "figma":
		return actual == "Editor" && required == "Viewer"
	case "s3":
		return actual == "read_write" && required == "read"
	default:
		return false
	}
}
