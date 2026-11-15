package onboarding

import (
	"context"
	"errors"
	"fmt"
	"gorm.io/gorm/clause"
	"strings"
	"time"

	accessservice "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/access"
	accessrequest "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/accessrequest"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/auth"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"gorm.io/gorm"
)

var (
	ErrInternNotFound  = errors.New("стажёр не найден")
	ErrForbidden       = errors.New("действие доступно только стажёру")
	ErrProfileNotFound = errors.New("профиль стажёра не найден")
	ErrManagerNotFound = errors.New("руководитель команды не найден")
	ErrMentorNotFound  = errors.New("у стажёра не выбран наставник")
	ErrInvalidTask     = errors.New("неизвестный этап адаптации")
)

type Service struct {
	db             *gorm.DB
	accesses       RequiredAccessesReader
	accessRequests AccessRequestsReader
}

// RequiredAccessesReader supplies the current access state for a trainee.
type RequiredAccessesReader interface {
	ListRequiredAccesses(context.Context, string) (accessservice.WorkingAccesses, error)
}

// AccessRequestsReader supplies the trainee's request counters.
type AccessRequestsReader interface {
	ListAccessRequests(context.Context, string, accessrequest.RequestFilter) (accessrequest.RequestList, error)
}

func NewService(db *gorm.DB) *Service {
	return NewServiceWithReaders(db, accessservice.NewService(db), accessrequest.NewService(db))
}

// NewServiceWithReaders wires the onboarding plan to the same access services
// used by the access endpoints.
func NewServiceWithReaders(db *gorm.DB, accesses RequiredAccessesReader, requests AccessRequestsReader) *Service {
	return &Service{db: db, accesses: accesses, accessRequests: requests}
}

func (s *Service) GetTeamIntro(
	ctx context.Context,
	employeeID string,
) (TaskDetails, error) {
	intern, profile, manager, err := s.loadTeamIntroPeople(ctx, employeeID)
	if err != nil {
		return TaskDetails{}, err
	}

	if profile.Mentor == nil {
		return TaskDetails{}, ErrMentorNotFound
	}

	status := "not_started"

	var completion models.TeamIntroduction
	err = s.db.WithContext(ctx).
		Take(&completion, "employee_id = ?", intern.ID).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		// Стажёр ещё не отметил знакомство
	case err != nil:
		return TaskDetails{}, fmt.Errorf("проверить отметку знакомства: %w", err)
	default:
		status = "completed"
	}

	return TaskDetails{
		ID:          "team-intro",
		Title:       "Знакомство с командой",
		Subtitle:    "Команда, руководитель и наставник",
		Status:      status,
		Description: "Узнай, чем занимается команда и к кому обращаться.",
		ChatEnabled: true,
		TeamIntro: &TeamIntro{
			Team: TeamRef{
				ID:   intern.Team.ID,
				Name: intern.Team.Name,
			},
			Description: intern.Team.Description,
			Manager: Contact{
				FullName: fullName(manager.FirstName, manager.SecondName),
				Position: manager.Position.Name,
			},
			Mentor: Contact{
				FullName: fullName(profile.Mentor.FirstName, profile.Mentor.SecondName),
				Position: "Наставник",
				Email:    profile.Mentor.Contact,
			},
		},
	}, nil
}

func (s *Service) loadTeamIntroPeople(
	ctx context.Context,
	employeeID string,
) (models.Employee, models.Profile, models.Employee, error) {
	var intern models.Employee

	err := s.db.WithContext(ctx).
		Preload("Team").
		Take(&intern, "id = ?", employeeID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return intern, models.Profile{}, models.Employee{}, ErrInternNotFound
	}
	if err != nil {
		return intern, models.Profile{}, models.Employee{}, fmt.Errorf("найти стажёра: %w", err)
	}
	if intern.Role != models.RoleIntern {
		return intern, models.Profile{}, models.Employee{}, ErrForbidden
	}

	var profile models.Profile
	err = s.db.WithContext(ctx).
		Preload("Mentor").
		Take(&profile, "employee_id = ?", intern.ID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return intern, profile, models.Employee{}, ErrProfileNotFound
	}
	if err != nil {
		return intern, profile, models.Employee{}, fmt.Errorf("найти профиль стажёра: %w", err)
	}

	var manager models.Employee
	err = s.db.WithContext(ctx).
		Preload("Position").
		Take(
			&manager,
			"team_id = ? AND role = ?",
			intern.TeamID,
			models.RoleManager,
		).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return intern, profile, manager, ErrManagerNotFound
	}
	if err != nil {
		return intern, profile, manager, fmt.Errorf("найти руководителя команды: %w", err)
	}

	return intern, profile, manager, nil
}

func fullName(firstName, secondName string) string {
	return strings.TrimSpace(firstName + " " + secondName)
}

func (s *Service) workingAccesses(ctx context.Context, employeeID string) (WorkingAccesses, error) {
	current, err := s.accesses.ListRequiredAccesses(ctx, employeeID)
	if err != nil {
		return WorkingAccesses{}, mapAccessError(err)
	}
	result := WorkingAccesses{
		Resources: make([]AccessResource, 0, len(current.Resources)),
		Progress: Progress{
			Completed:  current.Progress.Completed,
			Total:      current.Progress.Total,
			InProgress: current.Progress.InProgress,
			Failed:     current.Progress.Failed,
		},
	}
	for _, resource := range current.Resources {
		item := AccessResource{
			ID: resource.ID, Service: resource.Service, Name: resource.Name,
			AccessLevel: resource.AccessLevel, Purpose: resource.Purpose,
			Status: string(resource.Status), CanRequest: resource.CanRequest,
		}
		if resource.Request != nil {
			item.RequestID = resource.Request.ID
		}
		result.Resources = append(result.Resources, item)
	}
	return result, nil
}

func (s *Service) accessRequestSummary(ctx context.Context, employeeID string) (AccessRequestSummary, error) {
	requests, err := s.accessRequests.ListAccessRequests(ctx, employeeID, accessrequest.RequestFilter{})
	if err != nil {
		return AccessRequestSummary{}, mapAccessError(err)
	}
	return AccessRequestSummary{
		Pending:    int(requests.Counts.Pending),
		InProgress: int(requests.Counts.Pending + requests.Counts.Issuing),
		Processed:  int(requests.Counts.Granted + requests.Counts.Rejected + requests.Counts.Failed),
	}, nil
}

func mapAccessError(err error) error {
	switch {
	case errors.Is(err, auth.ErrEmployeeNotFound):
		return fmt.Errorf("%w: %w", ErrInternNotFound, err)
	case errors.Is(err, auth.ErrForbidden):
		return fmt.Errorf("%w: %w", ErrForbidden, err)
	case errors.Is(err, auth.ErrProfileRequired):
		return fmt.Errorf("%w: %w", ErrProfileNotFound, err)
	default:
		return err
	}
}

func (s *Service) CompleteTeamIntro(ctx context.Context, employeeID string) error {
	var intern models.Employee

	err := s.db.WithContext(ctx).
		Take(&intern, "id = ?", employeeID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrInternNotFound
	}
	if err != nil {
		return fmt.Errorf("найти стажёра: %w", err)
	}
	if intern.Role != models.RoleIntern {
		return ErrForbidden
	}

	var profile models.Profile
	err = s.db.WithContext(ctx).
		Take(&profile, "employee_id = ?", intern.ID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrProfileNotFound
	}
	if err != nil {
		return fmt.Errorf("найти профиль стажёра: %w", err)
	}

	completion := models.TeamIntroduction{
		EmployeeID:  intern.ID,
		CompletedAt: time.Now().UTC(),
	}

	err = s.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "employee_id"}},
			DoNothing: true,
		}).
		Create(&completion).Error
	if err != nil {
		return fmt.Errorf("сохранить отметку знакомства: %w", err)
	}

	return nil
}
