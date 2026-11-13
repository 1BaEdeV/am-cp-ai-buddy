package onboarding

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"gorm.io/gorm"
)

var (
	ErrInternNotFound  = errors.New("стажёр не найден")
	ErrForbidden       = errors.New("действие доступно только стажёру")
	ErrProfileNotFound = errors.New("профиль стажёра не найден")
	ErrManagerNotFound = errors.New("руководитель команды не найден")
	ErrMentorNotFound  = errors.New("у стажёра не выбран наставник")
)

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
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
