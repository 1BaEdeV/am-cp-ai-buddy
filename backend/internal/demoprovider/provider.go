// Package demoprovider имитирует проверку и выдачу прав корпоративными сервисами.
package demoprovider

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/access"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/accessrequest"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrInvalidInput      = errors.New("некорректные параметры демо-провайдера")
	ErrEmployeeNotFound  = errors.New("сотрудник демо-провайдера не найден")
	ErrResourceNotFound  = errors.New("ресурс демо-провайдера не найден")
	ErrUnsupportedLevel  = errors.New("уровень доступа не поддерживается ресурсом")
	ErrOperationConflict = errors.New("ID операции уже использован для других параметров выдачи")
	ErrCheckFailed       = errors.New("демо-сбой проверки доступа")
	ErrGrantFailed       = errors.New("демо-сбой выдачи доступа")
)

type FailureMode string

const (
	FailureNone       FailureMode = ""
	FailureCheck      FailureMode = "check_failed"
	FailureGrant      FailureMode = "grant_failed"
	FailureAfterGrant FailureMode = "timeout_after_grant"
)

type Provider struct {
	db  *gorm.DB
	now func() time.Time
}

var _ accessrequest.AccessProvider = (*Provider)(nil)

func NewProvider(db *gorm.DB) *Provider {
	return &Provider{db: db, now: time.Now}
}

// CheckAccess читает фактический уровень, даже если он ниже запрошенного.
// Отсутствие прав — обычный результат Granted=false, а не ошибка.
func (p *Provider) CheckAccess(ctx context.Context, target accessrequest.GrantTarget) (accessrequest.ProviderAccess, error) {
	if err := validateInput(ctx, target); err != nil {
		return accessrequest.ProviderAccess{}, err
	}
	tx := p.db.WithContext(ctx)
	if err := validateTarget(tx, target); err != nil {
		return accessrequest.ProviderAccess{}, err
	}
	var saved models.DemoProviderAccess
	err := tx.Take(&saved, "employee_id = ? AND service = ? AND external_id = ?", target.EmployeeID, target.Service, target.ExternalID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return accessrequest.ProviderAccess{}, nil
	}
	if err != nil {
		return accessrequest.ProviderAccess{}, fmt.Errorf("прочитать права демо-провайдера: %w", err)
	}
	if FailureMode(saved.FailureMode) == FailureCheck {
		return accessrequest.ProviderAccess{}, ErrCheckFailed
	}
	return accessrequest.ProviderAccess{Granted: saved.AccessLevel != "", AccessLevel: saved.AccessLevel}, nil
}

// GrantAccess атомарно сохраняет права и ID успешной операции. PostgreSQL
// сериализует конкурирующие операции по строке прав и уникальному operationID.
func (p *Provider) GrantAccess(ctx context.Context, operationID string, target accessrequest.GrantTarget) error {
	if err := validateInput(ctx, target); err != nil {
		return err
	}
	if strings.TrimSpace(operationID) == "" {
		return ErrInvalidInput
	}
	lostResponse := false
	err := p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var saved models.DemoProviderOperation
		err := tx.Take(&saved, "id = ?", operationID).Error
		if err == nil {
			return sameOperation(saved, target)
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("прочитать операцию демо-провайдера: %w", err)
		}
		if err := validateTarget(tx, target); err != nil {
			return err
		}
		operation := models.DemoProviderOperation{
			ID: operationID, EmployeeID: target.EmployeeID, Service: target.Service,
			ExternalID: target.ExternalID, AccessLevel: target.AccessLevel, CreatedAt: p.now().UTC(),
		}
		insert := tx.Omit(clause.Associations).Clauses(clause.OnConflict{DoNothing: true}).Create(&operation)
		if insert.Error != nil {
			return fmt.Errorf("сохранить операцию демо-провайдера: %w", insert.Error)
		}
		if insert.RowsAffected == 0 {
			// Другой процесс мог завершить ту же операцию после первого чтения.
			if err := tx.Take(&saved, "id = ?", operationID).Error; err != nil {
				return fmt.Errorf("прочитать параллельную операцию демо-провайдера: %w", err)
			}
			return sameOperation(saved, target)
		}
		state, err := p.lockAccess(tx, target)
		if err != nil {
			return err
		}
		if FailureMode(state.FailureMode) == FailureGrant {
			return ErrGrantFailed
		}
		if err := p.saveLevel(tx, state, target.AccessLevel); err != nil {
			return err
		}
		lostResponse = FailureMode(state.FailureMode) == FailureAfterGrant
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	if lostResponse {
		// Имитируем потерю ответа после коммита без ожидания реального тайм-аута.
		// Проверка прав и повтор operationID обнаружат успешно выполненную выдачу.
		return context.DeadlineExceeded
	}
	return nil
}

// SeedAccess задаёт начальные права; повтор не понижает уже выданный уровень.
// EmployeeAccess и заявки эта служебная операция не изменяет.
func (p *Provider) SeedAccess(ctx context.Context, target accessrequest.GrantTarget) error {
	return p.changeAccess(ctx, target, func(tx *gorm.DB, state models.DemoProviderAccess) error {
		return p.saveLevel(tx, state, target.AccessLevel)
	})
}

// SetFailure задаёт воспроизводимый сбой для сотрудника и внешнего ресурса.
// FailureNone отключает сбой, не изменяя уже выданные права.
func (p *Provider) SetFailure(ctx context.Context, target accessrequest.GrantTarget, mode FailureMode) error {
	if !slices.Contains([]FailureMode{FailureNone, FailureCheck, FailureGrant, FailureAfterGrant}, mode) {
		return ErrInvalidInput
	}
	return p.changeAccess(ctx, target, func(tx *gorm.DB, state models.DemoProviderAccess) error {
		return tx.Model(&state).Updates(map[string]any{"failure_mode": string(mode), "updated_at": p.now().UTC()}).Error
	})
}

func (p *Provider) changeAccess(ctx context.Context, target accessrequest.GrantTarget, change func(*gorm.DB, models.DemoProviderAccess) error) error {
	if err := validateInput(ctx, target); err != nil {
		return err
	}
	return p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := validateTarget(tx, target); err != nil {
			return err
		}
		state, err := p.lockAccess(tx, target)
		if err != nil {
			return err
		}
		if err := change(tx, state); err != nil {
			return fmt.Errorf("изменить состояние демо-провайдера: %w", err)
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
}

func validateInput(ctx context.Context, target accessrequest.GrantTarget) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, value := range []string{target.EmployeeID, target.Service, target.ExternalID, target.AccessLevel} {
		if strings.TrimSpace(value) == "" {
			return ErrInvalidInput
		}
	}
	return nil
}

func validateTarget(tx *gorm.DB, target accessrequest.GrantTarget) error {
	var employee models.Employee
	err := tx.Select("id").Take(&employee, "id = ?", target.EmployeeID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrEmployeeNotFound
	}
	if err != nil {
		return fmt.Errorf("проверить сотрудника демо-провайдера: %w", err)
	}
	var resource models.Resource
	err = tx.Take(&resource, "service = ? AND external_id = ?", target.Service, target.ExternalID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrResourceNotFound
	}
	if err != nil {
		return fmt.Errorf("проверить ресурс демо-провайдера: %w", err)
	}
	if !slices.Contains(resource.AccessLevels, target.AccessLevel) {
		return ErrUnsupportedLevel
	}
	return nil
}

func sameOperation(saved models.DemoProviderOperation, target accessrequest.GrantTarget) error {
	if saved.EmployeeID != target.EmployeeID || saved.Service != target.Service || saved.ExternalID != target.ExternalID || saved.AccessLevel != target.AccessLevel {
		return ErrOperationConflict
	}
	return nil
}

func (p *Provider) lockAccess(tx *gorm.DB, target accessrequest.GrantTarget) (models.DemoProviderAccess, error) {
	state := models.DemoProviderAccess{EmployeeID: target.EmployeeID, Service: target.Service, ExternalID: target.ExternalID, CreatedAt: p.now().UTC(), UpdatedAt: p.now().UTC()}
	if err := tx.Omit(clause.Associations).Clauses(clause.OnConflict{DoNothing: true}).Create(&state).Error; err != nil {
		return state, fmt.Errorf("создать состояние демо-провайдера: %w", err)
	}
	var saved models.DemoProviderAccess
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Take(&saved,
		"employee_id = ? AND service = ? AND external_id = ?", target.EmployeeID, target.Service, target.ExternalID).Error; err != nil {
		return state, fmt.Errorf("заблокировать состояние демо-провайдера: %w", err)
	}
	return saved, nil
}

func (p *Provider) saveLevel(tx *gorm.DB, state models.DemoProviderAccess, level string) error {
	if state.AccessLevel != "" && access.HasRequiredLevel(state.Service, state.AccessLevel, level) {
		return nil
	}
	if err := tx.Model(&state).Updates(map[string]any{"access_level": level, "updated_at": p.now().UTC()}).Error; err != nil {
		return fmt.Errorf("сохранить права демо-провайдера: %w", err)
	}
	return nil
}
