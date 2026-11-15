package accessrequest

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"gorm.io/gorm"
)

// GetAccessRequest даёт стажёру только свою заявку, руководителю — назначенную ему.
func (s *Service) GetAccessRequest(ctx context.Context, employeeID, requestID string) (Request, error) {
	if strings.TrimSpace(requestID) == "" {
		return Request{}, ErrInvalidInput
	}
	var result models.AccessRequest
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		employee, err := requestEmployee(tx, employeeID, "")
		if err != nil {
			return err
		}
		err = visibleRequests(tx, employee).Take(&result, "id = ?", strings.TrimSpace(requestID)).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrRequestNotFound
		}
		if err != nil {
			return fmt.Errorf("прочитать заявку на доступ: %w", err)
		}
		return nil
	}, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return Request{}, err
	}
	return requestView(result), nil
}

// ListAccessRequests возвращает стажёру свои заявки с фильтрами, а руководителю —
// только назначенные ему pending. Страница и счётчики читаются из одного снимка БД.
func (s *Service) ListAccessRequests(ctx context.Context, employeeID string, filter RequestFilter) (RequestList, error) {
	if filter.Limit == 0 {
		filter.Limit = 20
	}
	filter.Group = strings.TrimSpace(filter.Group)
	filter.ResourceID = strings.TrimSpace(filter.ResourceID)
	if filter.Limit < 1 || filter.Limit > 100 || filter.Offset < 0 {
		return RequestList{}, ErrInvalidInput
	}
	result := RequestList{Items: []Request{}, Limit: filter.Limit, Offset: filter.Offset}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		employee, err := requestEmployee(tx, employeeID, "")
		if err != nil {
			return err
		}
		if employee.Role == models.RoleManager {
			filter.Group, filter.Status, filter.ResourceID = "", "", ""
		} else if !slices.Contains([]string{"", "all", "active", "completed"}, filter.Group) ||
			(filter.Status != "" && !slices.Contains([]models.AccessRequestStatus{
				models.AccessRequestPending, models.AccessRequestIssuing, models.AccessRequestGranted,
				models.AccessRequestRejected, models.AccessRequestFailed,
			}, filter.Status)) {
			return ErrInvalidInput
		}
		var groups []struct {
			Status models.AccessRequestStatus
			Count  int64
		}
		if err := requestsForList(tx, employee).Select("status, count(*) AS count").Group("status").Scan(&groups).Error; err != nil {
			return fmt.Errorf("посчитать заявки на доступ: %w", err)
		}
		for _, group := range groups {
			result.Counts.Total += group.Count
			switch group.Status {
			case models.AccessRequestPending:
				result.Counts.Pending = group.Count
			case models.AccessRequestIssuing:
				result.Counts.Issuing = group.Count
			case models.AccessRequestGranted:
				result.Counts.Granted = group.Count
			case models.AccessRequestRejected:
				result.Counts.Rejected = group.Count
			case models.AccessRequestFailed:
				result.Counts.Failed = group.Count
			}
		}
		result.Counts.Active = result.Counts.Pending + result.Counts.Issuing + result.Counts.Failed
		result.Counts.Completed = result.Counts.Granted + result.Counts.Rejected
		query := requestsForList(tx, employee)
		switch filter.Group {
		case "active":
			query = query.Where("status IN ?", activeRequestStatuses)
		case "completed":
			query = query.Where("status IN ?", []models.AccessRequestStatus{models.AccessRequestGranted, models.AccessRequestRejected})
		}
		if filter.Status != "" {
			query = query.Where("status = ?", filter.Status)
		}
		if filter.ResourceID != "" {
			query = query.Where("resource_id = ?", filter.ResourceID)
		}
		if err := query.Count(&result.Total).Error; err != nil {
			return fmt.Errorf("посчитать отфильтрованные заявки: %w", err)
		}
		var requests []models.AccessRequest
		if err := query.Order("created_at DESC, id DESC").Limit(filter.Limit).Offset(filter.Offset).Find(&requests).Error; err != nil {
			return fmt.Errorf("загрузить список заявок: %w", err)
		}
		for _, request := range requests {
			result.Items = append(result.Items, requestView(request))
		}
		return nil
	}, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return RequestList{}, err
	}
	return result, nil
}

func requestsForList(tx *gorm.DB, employee models.Employee) *gorm.DB {
	query := visibleRequests(tx, employee)
	if employee.Role == models.RoleManager {
		return query.Where("status = ?", models.AccessRequestPending)
	}
	return query
}

func visibleRequests(tx *gorm.DB, employee models.Employee) *gorm.DB {
	query := tx.Model(&models.AccessRequest{})
	if employee.Role == models.RoleManager {
		return query.Where("approver_id = ?", employee.ID)
	}
	return query.Where("employee_id = ?", employee.ID)
}
