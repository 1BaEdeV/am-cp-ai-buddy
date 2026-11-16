package auth

import (
	"fmt"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"gorm.io/gorm"
)

func registrationForm(tx *gorm.DB, employee models.Employee) (LoginResult, error) {
	query := tx.Preload("Mentors", func(db *gorm.DB) *gorm.DB {
		return db.Order("first_name, second_name, id")
	}).Order("name, id")
	if employee.Role == models.RoleManager {
		query = query.Where("id = ?", employee.TeamID)
	} else if employee.Role != models.RoleIntern {
		return LoginResult{}, ErrForbidden
	}
	var teams []models.Team
	if err := query.Find(&teams).Error; err != nil {
		return LoginResult{}, fmt.Errorf("загрузить команды: %w", err)
	}
	options := make([]TeamOption, 0, len(teams))
	for _, team := range teams {
		if employee.Role == models.RoleIntern && len(team.Mentors) == 0 {
			continue
		}
		var positions []models.Position
		positionsQuery := tx.Order("name, id")
		if employee.Role == models.RoleIntern {
			positionsQuery = positionsQuery.Where("id IN (?)", tx.Model(&models.AccessRequirement{}).
				Select("position_id").Where("team_id = ?", team.ID))
		}
		if err := positionsQuery.Find(&positions).Error; err != nil {
			return LoginResult{}, fmt.Errorf("загрузить должности команды: %w", err)
		}
		if len(positions) == 0 {
			continue
		}
		option := TeamOption{Option: Option{ID: team.ID, Name: team.Name},
			Mentors: make([]MentorOption, 0, len(team.Mentors)), PositionOptions: make([]Option, 0, len(positions))}
		for _, mentor := range team.Mentors {
			option.Mentors = append(option.Mentors, MentorOption{ID: mentor.ID, FirstName: mentor.FirstName, SecondName: mentor.SecondName})
		}
		for _, position := range positions {
			option.PositionOptions = append(option.PositionOptions, Option{ID: position.ID, Name: position.Name})
		}
		options = append(options, option)
	}
	if len(options) == 0 {
		return LoginResult{}, ErrRegistrationUnavailable
	}
	selected := options[0]
	for _, option := range options {
		if option.ID == employee.TeamID {
			selected = option
			break
		}
	}
	positionID := selected.PositionOptions[0].ID
	for _, position := range selected.PositionOptions {
		if position.ID == employee.PositionID {
			positionID = position.ID
		}
	}
	return LoginResult{
		Status: LoginNeedsRegistration, EmployeeID: employee.ID, FirstName: employee.FirstName,
		SecondName: employee.SecondName, Email: employee.Email, Role: employee.Role,
		Team: &selected, TeamOptions: options, PositionID: positionID, PositionOptions: selected.PositionOptions,
	}, nil
}
