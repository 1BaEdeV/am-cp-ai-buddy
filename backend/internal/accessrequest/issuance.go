package accessrequest

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	accessservice "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/access"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/auth"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	issuanceLease          = time.Minute
	providerTimeout        = 30 * time.Second
	resourceChangedMessage = "Внешний ресурс изменился после подтверждения. Руководителю нужно проверить настройки ресурса."
)

// RetryAccessIssuance повторяет обработку существующей заявки после ошибки.
func (s *Service) RetryAccessIssuance(ctx context.Context, approverID, requestID string) (Request, error) {
	if strings.TrimSpace(requestID) == "" {
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
			return fmt.Errorf("загрузить заявку для повторной выдачи: %w", err)
		}
		if result.Status == models.AccessRequestIssuing || result.Status == models.AccessRequestGranted {
			return nil
		}
		if result.Status != models.AccessRequestFailed {
			return ErrInvalidRequestState
		}
		now := s.now().UTC()
		update := tx.Model(&models.AccessIssuanceJob{}).Where("request_id = ? AND completed_at IS NULL", result.ID).
			Updates(map[string]any{"available_at": now, "lease_until": nil, "lease_token": "", "updated_at": now})
		if update.Error != nil {
			return fmt.Errorf("повторно поставить выдачу в очередь: %w", update.Error)
		}
		if update.RowsAffected != 1 {
			return fmt.Errorf("повторить выдачу: у заявки отсутствует незавершённое задание")
		}
		if err := tx.Model(&models.AccessRequest{}).Where("id = ?", result.ID).
			Updates(map[string]any{"status": models.AccessRequestIssuing, "last_error": "", "updated_at": now}).Error; err != nil {
			return fmt.Errorf("сохранить повторную выдачу: %w", err)
		}
		result.Status, result.LastError, result.UpdatedAt = models.AccessRequestIssuing, "", now
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return Request{}, err
	}
	return requestView(result), nil
}

// ProcessNextIssuance — один шаг фонового обработчика. Возвращает false, если
// готовых заданий нет. Сетевые вызовы выполняются без удержания транзакции БД.
// После остановки процесса задание можно забрать заново по истечении аренды.
func (s *Service) ProcessNextIssuance(ctx context.Context, provider AccessProvider) (bool, error) {
	if provider == nil {
		return false, ErrInvalidInput
	}
	job, request, err := s.claimIssuance(ctx)
	if err != nil || job == nil {
		return false, err
	}
	target := GrantTarget{
		EmployeeID: request.EmployeeID, Service: request.Parameters.ResourceService,
		ExternalID: request.Parameters.ResourceExternalID, AccessLevel: request.Parameters.AccessLevel,
	}
	providerCtx, cancel := context.WithTimeout(ctx, providerTimeout)
	defer cancel()
	// Проверка обязательна на каждой попытке, особенно после неизвестного исхода
	// прошлой выдачи. Ключ внешней операции всегда равен ID исходной заявки.
	var actual ProviderAccess
	var issueErr error
	publicError := "Не удалось проверить доступ. Согласующий руководитель может повторить обработку заявки."
	if !issuanceResourceMatches(request.Resource, request.Parameters) {
		issueErr, publicError = ErrIssuanceResourceChanged, resourceChangedMessage
	} else {
		actual, issueErr = provider.CheckAccess(providerCtx, target)
		if issueErr == nil && !providerHasRequiredAccess(actual, target) {
			publicError = "Не удалось выдать доступ. Руководитель может повторить обработку; сервис сначала проверит предыдущую попытку."
			issueErr = provider.GrantAccess(providerCtx, request.ID, target)
			if issueErr == nil {
				publicError = "Сервис пока не подтвердил выдачу доступа. Согласующий руководитель может повторить проверку."
				actual, issueErr = provider.CheckAccess(providerCtx, target)
				if issueErr == nil && !providerHasRequiredAccess(actual, target) {
					issueErr = ErrIssuanceUnconfirmed
				}
			}
		}
	}
	// Даже при отмене сетевого контекста сохраняем понятное состояние заявки.
	// Если процесс завершится до записи, задание восстановится по истечении аренды.
	saveCtx, cancelSave := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancelSave()
	if err := s.finishIssuance(saveCtx, *job, request, actual, issueErr, publicError); err != nil {
		return true, errors.Join(issueErr, err)
	}
	if issueErr != nil {
		return true, fmt.Errorf("обработать выдачу доступа: %w", issueErr)
	}
	return true, nil
}

func (s *Service) claimIssuance(ctx context.Context) (*models.AccessIssuanceJob, models.AccessRequest, error) {
	var job models.AccessIssuanceJob
	var request models.AccessRequest
	found := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := s.now().UTC()
		issuing := tx.Model(&models.AccessRequest{}).Select("id").Where("status = ?", models.AccessRequestIssuing)
		result := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("completed_at IS NULL AND available_at <= ? AND (lease_until IS NULL OR lease_until <= ?)", now, now).
			Where("request_id IN (?)", issuing).Order("available_at, request_id").Limit(1).Find(&job)
		if result.Error != nil {
			return fmt.Errorf("взять задание на выдачу доступа: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return nil
		}
		until := now.Add(issuanceLease)
		job.LeaseToken, job.LeaseUntil, job.Attempts = rand.Text(), &until, job.Attempts+1
		if err := tx.Model(&models.AccessIssuanceJob{}).Where("request_id = ?", job.RequestID).
			Updates(map[string]any{"lease_token": job.LeaseToken, "lease_until": until, "attempts": job.Attempts, "updated_at": now}).Error; err != nil {
			return fmt.Errorf("закрепить задание за обработчиком: %w", err)
		}
		if err := tx.Preload("Resource").Take(&request, "id = ?", job.RequestID).Error; err != nil {
			return fmt.Errorf("загрузить заявку для выдачи: %w", err)
		}
		found = true
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil || !found {
		return nil, models.AccessRequest{}, err
	}
	return &job, request, nil
}

func (s *Service) finishIssuance(ctx context.Context, claimed models.AccessIssuanceJob, request models.AccessRequest, actual ProviderAccess, issueErr error, publicError string) error {
	var outcomeErr error
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := requestEmployee(tx, request.EmployeeID, "UPDATE"); err != nil {
			return err
		}
		var saved models.AccessRequest
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Take(&saved, "id = ?", request.ID).Error; err != nil {
			return fmt.Errorf("загрузить заявку для результата выдачи: %w", err)
		}
		var job models.AccessIssuanceJob
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Take(&job, "request_id = ?", request.ID).Error; err != nil {
			return fmt.Errorf("загрузить задание для результата выдачи: %w", err)
		}
		now := s.now().UTC()
		if job.LeaseToken != claimed.LeaseToken || job.LeaseUntil == nil || !now.Before(*job.LeaseUntil) || job.CompletedAt != nil {
			return ErrIssuanceLeaseLost
		}
		if saved.Status != models.AccessRequestIssuing {
			return ErrInvalidRequestState
		}
		if issueErr == nil {
			// Назначение могло измениться, пока выполнялся сетевой вызов.
			// Удерживаем ресурс до коммита, чтобы не засчитать права другого ресурса.
			var resource models.Resource
			if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Take(&resource, "id = ?", saved.ResourceID).Error; err != nil {
				return fmt.Errorf("проверить ресурс перед сохранением доступа: %w", err)
			}
			if !issuanceResourceMatches(resource, saved.Parameters) {
				issueErr, publicError = ErrIssuanceResourceChanged, resourceChangedMessage
				outcomeErr = ErrIssuanceResourceChanged
			}
		}
		requestUpdate := map[string]any{"updated_at": now}
		jobUpdate := map[string]any{"lease_until": nil, "lease_token": "", "updated_at": now}
		if issueErr == nil {
			access := models.EmployeeAccess{
				ID: rand.Text(), EmployeeID: saved.EmployeeID, ResourceID: saved.ResourceID,
				AccessLevel: actual.AccessLevel, VerifiedAt: now,
			}
			if err := tx.Omit(clause.Associations).Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "employee_id"}, {Name: "resource_id"}},
				DoUpdates: clause.AssignmentColumns([]string{"access_level", "verified_at"}),
			}).Create(&access).Error; err != nil {
				return fmt.Errorf("сохранить подтверждённый доступ: %w", err)
			}
			requestUpdate["status"], requestUpdate["granted_at"], requestUpdate["last_error"] = models.AccessRequestGranted, now, ""
			jobUpdate["completed_at"] = now
		} else {
			requestUpdate["status"], requestUpdate["last_error"] = models.AccessRequestFailed, publicError
		}
		if err := tx.Model(&models.AccessRequest{}).Where("id = ?", saved.ID).Updates(requestUpdate).Error; err != nil {
			return fmt.Errorf("сохранить результат выдачи: %w", err)
		}
		if err := tx.Model(&models.AccessIssuanceJob{}).Where("request_id = ?", job.RequestID).Updates(jobUpdate).Error; err != nil {
			return fmt.Errorf("завершить попытку выдачи: %w", err)
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	return outcomeErr
}

func issuanceResourceMatches(resource models.Resource, parameters models.AccessRequestParameters) bool {
	return resource.ID != "" && resource.Service == parameters.ResourceService && resource.ExternalID == parameters.ResourceExternalID
}

func providerHasRequiredAccess(actual ProviderAccess, target GrantTarget) bool {
	return actual.Granted && accessservice.HasRequiredLevel(target.Service, actual.AccessLevel, target.AccessLevel)
}
