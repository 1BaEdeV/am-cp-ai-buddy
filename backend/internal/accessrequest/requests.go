package accessrequest

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	accessservice "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/access"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/auth"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var activeRequestStatuses = accessservice.ActiveRequestStatuses()

func (s *Service) PrepareAccessRequest(ctx context.Context, employeeID, resourceID string) (Preparation, error) {
	resourceID = strings.TrimSpace(resourceID)
	if resourceID == "" {
		return Preparation{}, ErrInvalidInput
	}
	var result Preparation
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		employee, err := requestEmployee(tx, employeeID, "UPDATE")
		if err != nil {
			return err
		}
		if err := requireInternProfile(tx, employee); err != nil {
			return err
		}
		requirement, err := resourceRequirement(tx, employee, resourceID)
		if err != nil {
			return err
		}
		granted, err := sufficientAccess(tx, employee.ID, requirement)
		if err != nil {
			return err
		}
		if granted {
			result.Status = PreparationAlreadyGranted
			return nil
		}
		active, err := activeRequest(tx, employee.ID, resourceID)
		if err != nil {
			return err
		}
		if active != nil {
			view := requestView(*active)
			result = Preparation{Status: PreparationRequestExists, Request: &view}
			return nil
		}
		now := s.now().UTC()
		draft := models.AccessDraft{
			ID: rand.Text(), EmployeeID: employee.ID, TeamID: employee.TeamID, PositionID: employee.PositionID,
			ResourceID: resourceID, ApproverID: requirement.ApproverID, Parameters: requirementParameters(requirement),
			CreatedAt: now, ExpiresAt: now.Add(DraftTTL),
		}
		if err := tx.Omit(clause.Associations).Create(&draft).Error; err != nil {
			return fmt.Errorf("сохранить черновик доступа: %w", err)
		}
		view := draftView(draft)
		result = Preparation{Status: PreparationReady, Draft: &view}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return Preparation{}, err
	}
	return result, nil
}

// ConfirmAccessRequest вызывается только после явного согласия владельца на
func (s *Service) ConfirmAccessRequest(ctx context.Context, employeeID, draftID string) (Request, bool, error) {
	draftID = strings.TrimSpace(draftID)
	if draftID == "" {
		return Request{}, false, ErrInvalidInput
	}
	var result models.AccessRequest
	var created bool
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		employee, err := requestEmployee(tx, employeeID, "UPDATE")
		if err != nil {
			return err
		}
		if employee.Role != models.RoleIntern {
			return auth.ErrForbidden
		}
		var draft models.AccessDraft
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Take(&draft, "id = ? AND employee_id = ?", draftID, employee.ID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrDraftNotFound
		}
		if err != nil {
			return fmt.Errorf("загрузить черновик доступа: %w", err)
		}
		err = tx.Take(&result, "draft_id = ? AND employee_id = ?", draft.ID, employee.ID).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("проверить результат подтверждения: %w", err)
		}
		if err := requireInternProfile(tx, employee); err != nil {
			return err
		}
		now := s.now().UTC()
		if !now.Before(draft.ExpiresAt) {
			return ErrDraftExpired
		}
		if draft.ConfirmedAt != nil || employee.TeamID != draft.TeamID || employee.PositionID != draft.PositionID {
			return ErrDraftChanged
		}
		requirement, err := resourceRequirement(tx, employee, draft.ResourceID)
		if err != nil {
			if errors.Is(err, ErrResourceNotRequired) || errors.Is(err, accessservice.ErrInvalidRequirement) {
				return ErrDraftChanged
			}
			return err
		}
		if draft.ApproverID != requirement.ApproverID || draft.Parameters != requirementParameters(requirement) {
			return ErrDraftChanged
		}
		granted, err := sufficientAccess(tx, employee.ID, requirement)
		if err != nil {
			return err
		}
		if granted {
			return ErrAccessAlreadyGranted
		}
		active, err := activeRequest(tx, employee.ID, draft.ResourceID)
		if err != nil {
			return err
		}
		if active != nil {
			return ErrActiveRequestExists
		}
		result = models.AccessRequest{
			ID: rand.Text(), DraftID: draft.ID, EmployeeID: employee.ID,
			TeamID: draft.TeamID, PositionID: draft.PositionID, ResourceID: draft.ResourceID,
			ApproverID: draft.ApproverID, Parameters: draft.Parameters, Status: models.AccessRequestPending,
			CreatedAt: now, UpdatedAt: now,
		}
		if err := tx.Omit(clause.Associations).Create(&result).Error; err != nil {
			return fmt.Errorf("создать заявку на доступ: %w", err)
		}
		if err := tx.Model(&models.AccessDraft{}).Where("id = ?", draft.ID).Update("confirmed_at", now).Error; err != nil {
			return fmt.Errorf("сохранить подтверждение черновика: %w", err)
		}
		created = true
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if isUniqueViolation(err) {
		var saved models.AccessRequest
		lookupErr := s.db.WithContext(ctx).Take(&saved, "draft_id = ? AND employee_id = ?", draftID, strings.TrimSpace(employeeID)).Error
		if lookupErr == nil {
			return requestView(saved), false, nil
		}
		if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
			return Request{}, false, fmt.Errorf("прочитать результат повторного подтверждения: %w", lookupErr)
		}
		active, lookupErr := activeRequest(s.db.WithContext(ctx), strings.TrimSpace(employeeID), result.ResourceID)
		if lookupErr != nil {
			return Request{}, false, lookupErr
		}
		if active != nil {
			return Request{}, false, ErrActiveRequestExists
		}
	}
	if err != nil {
		return Request{}, false, err
	}
	return requestView(result), created, nil
}

// DecideAccessRequest принимает решение только назначенного руководителя.
func (s *Service) DecideAccessRequest(ctx context.Context, approverID, requestID string, input DecisionInput) (Request, error) {
	input.Reason = strings.TrimSpace(input.Reason)
	if strings.TrimSpace(requestID) == "" ||
		(input.Decision != DecisionApprove && input.Decision != DecisionReject) ||
		(input.Decision == DecisionReject && input.Reason == "") || utf8.RuneCountInString(input.Reason) > 2000 {
		return Request{}, ErrInvalidInput
	}
	var result models.AccessRequest
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		actor, err := requestEmployee(tx, approverID, "SHARE")
		if err != nil {
			return err
		}
		if actor.Role != models.RoleManager {
			return auth.ErrForbidden
		}
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Take(&result, "id = ? AND approver_id = ?", strings.TrimSpace(requestID), actor.ID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrRequestNotFound
		}
		if err != nil {
			return fmt.Errorf("загрузить заявку для решения: %w", err)
		}
		if result.Status != models.AccessRequestPending {
			sameDecision := (input.Decision == DecisionReject && result.Status == models.AccessRequestRejected) ||
				(input.Decision == DecisionApprove && slices.Contains([]models.AccessRequestStatus{
					models.AccessRequestIssuing, models.AccessRequestGranted, models.AccessRequestFailed,
				}, result.Status))
			if sameDecision && result.DecidedAt != nil && result.DecisionReason == input.Reason {
				return nil
			}
			return ErrRequestAlreadyDecided
		}
		now := s.now().UTC()
		status := models.AccessRequestRejected
		if input.Decision == DecisionApprove {
			status = models.AccessRequestIssuing
		}
		update := tx.Model(&models.AccessRequest{}).Where("id = ? AND status = ?", result.ID, models.AccessRequestPending).
			Updates(map[string]any{"status": status, "decided_at": now, "decision_reason": input.Reason, "updated_at": now})
		if update.Error != nil {
			return fmt.Errorf("сохранить решение руководителя: %w", update.Error)
		}
		if update.RowsAffected != 1 {
			return ErrRequestAlreadyDecided
		}
		if input.Decision == DecisionApprove {
			job := models.AccessIssuanceJob{RequestID: result.ID, AvailableAt: now, CreatedAt: now, UpdatedAt: now}
			if err := tx.Omit(clause.Associations).Create(&job).Error; err != nil {
				return fmt.Errorf("сохранить задание на выдачу доступа: %w", err)
			}
		}
		result.Status, result.DecidedAt, result.DecisionReason, result.UpdatedAt = status, &now, input.Reason, now
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return Request{}, err
	}
	return requestView(result), nil
}

func requestEmployee(tx *gorm.DB, employeeID, lock string) (models.Employee, error) {
	var employee models.Employee
	employeeID = strings.TrimSpace(employeeID)
	if employeeID == "" {
		return employee, auth.ErrEmployeeNotFound
	}
	query := tx
	if lock != "" {
		query = query.Clauses(clause.Locking{Strength: lock})
	}
	err := query.Take(&employee, "id = ?", employeeID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return employee, auth.ErrEmployeeNotFound
	}
	if err != nil {
		return employee, fmt.Errorf("загрузить сотрудника для заявки: %w", err)
	}
	if employee.Role != models.RoleIntern && employee.Role != models.RoleManager {
		return employee, auth.ErrForbidden
	}
	return employee, nil
}

func requireInternProfile(tx *gorm.DB, employee models.Employee) error {
	if employee.Role != models.RoleIntern {
		return auth.ErrForbidden
	}
	var profile models.Profile
	err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Select("id").Take(&profile, "employee_id = ?", employee.ID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return auth.ErrProfileRequired
	}
	if err != nil {
		return fmt.Errorf("проверить профиль для заявки: %w", err)
	}
	return nil
}

func resourceRequirement(tx *gorm.DB, employee models.Employee, resourceID string) (models.AccessRequirement, error) {
	var requirement models.AccessRequirement
	err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Take(&requirement,
		"team_id = ? AND position_id = ? AND resource_id = ?", employee.TeamID, employee.PositionID, resourceID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return requirement, ErrResourceNotRequired
	}
	if err != nil {
		return requirement, fmt.Errorf("загрузить правило доступа: %w", err)
	}
	if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Take(&requirement.Resource, "id = ?", resourceID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return requirement, accessservice.ErrInvalidRequirement
		}
		return requirement, fmt.Errorf("загрузить ресурс для заявки: %w", err)
	}
	if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Take(&requirement.Approver, "id = ?", requirement.ApproverID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return requirement, accessservice.ErrInvalidRequirement
		}
		return requirement, fmt.Errorf("загрузить согласующего для заявки: %w", err)
	}
	if !accessservice.ValidRequirement(requirement, employee.TeamID) {
		return requirement, accessservice.ErrInvalidRequirement
	}
	return requirement, nil
}

func sufficientAccess(tx *gorm.DB, employeeID string, r models.AccessRequirement) (bool, error) {
	var saved models.EmployeeAccess
	err := tx.Take(&saved, "employee_id = ? AND resource_id = ?", employeeID, r.ResourceID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("проверить сохранённый доступ: %w", err)
	}
	return !saved.VerifiedAt.IsZero() && accessservice.HasRequiredLevel(r.Resource.Service, saved.AccessLevel, r.AccessLevel), nil
}

func activeRequest(tx *gorm.DB, employeeID, resourceID string) (*models.AccessRequest, error) {
	var request models.AccessRequest
	err := tx.Where("employee_id = ? AND resource_id = ? AND status IN ?", employeeID, resourceID, activeRequestStatuses).Take(&request).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("проверить активную заявку: %w", err)
	}
	return &request, nil
}

func requirementParameters(r models.AccessRequirement) models.AccessRequestParameters {
	return models.AccessRequestParameters{
		ResourceName: r.Resource.Name, ResourceService: r.Resource.Service, ResourceExternalID: r.Resource.ExternalID,
		AccessLevel: r.AccessLevel, Purpose: r.Purpose,
		ApproverName: strings.TrimSpace(r.Approver.FirstName + " " + r.Approver.SecondName),
	}
}

func parametersView(p models.AccessRequestParameters) RequestParameters {
	return RequestParameters(p)
}

func draftView(d models.AccessDraft) Draft {
	return Draft{ID: d.ID, ResourceID: d.ResourceID, ApproverID: d.ApproverID, Parameters: parametersView(d.Parameters), CreatedAt: d.CreatedAt, ExpiresAt: d.ExpiresAt}
}

func requestView(r models.AccessRequest) Request {
	return Request{
		ID: r.ID, Number: r.Number, DraftID: r.DraftID, EmployeeID: r.EmployeeID, ResourceID: r.ResourceID,
		ApproverID: r.ApproverID, Parameters: parametersView(r.Parameters), Status: r.Status,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, DecidedAt: r.DecidedAt, DecisionReason: r.DecisionReason,
		GrantedAt: r.GrantedAt, LastError: r.LastError,
	}
}

func isUniqueViolation(err error) bool {
	var sqlState interface{ SQLState() string }
	return errors.Is(err, gorm.ErrDuplicatedKey) || (errors.As(err, &sqlState) && sqlState.SQLState() == "23505")
}
