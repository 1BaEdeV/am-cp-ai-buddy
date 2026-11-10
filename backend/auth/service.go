package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// Login ищет сотрудника в демо-справочнике и возвращает профиль или данные для формы.
func (s *Service) Login(ctx context.Context, identifier string) (LoginResult, error) {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" || utf8.RuneCountInString(identifier) > 254 {
		return LoginResult{}, ErrInvalidInput
	}

	var result LoginResult
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var employee models.Employee
		err := tx.Preload("Position").Take(&employee, "id = ?", identifier).Error
		if errors.Is(err, gorm.ErrRecordNotFound) && strings.Contains(identifier, "@") {
			err = tx.Preload("Position").Take(&employee, "lower(email) = lower(?)", identifier).Error
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrEmployeeNotFound
		}
		if err != nil {
			return fmt.Errorf("найти сотрудника: %w", err)
		}

		profile, err := findProfile(tx, employee.ID)
		switch {
		case err == nil:
			view := profileView(profile, employee)
			result = LoginResult{Status: LoginReady, Profile: &view}
			return nil
		case errors.Is(err, gorm.ErrRecordNotFound):
			result, err = registrationForm(tx, employee)
			return err
		default:
			return fmt.Errorf("найти профиль: %w", err)
		}
	}, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return LoginResult{}, err
	}
	return result, nil
}

// Register сохраняет профиль и возвращает признак создания. При повторе возвращает сохранённый профиль.
func (s *Service) Register(ctx context.Context, employeeID string, input RegistrationInput) (Profile, bool, error) {
	employeeID = strings.TrimSpace(employeeID)
	if employeeID == "" {
		return Profile{}, false, ErrEmployeeNotFound
	}
	input.TeamID = strings.TrimSpace(input.TeamID)
	input.PositionID = strings.TrimSpace(input.PositionID)
	input.MentorID = strings.TrimSpace(input.MentorID)
	input.InternshipStartDate = strings.TrimSpace(input.InternshipStartDate)
	if input.TeamID == "" {
		return Profile{}, false, fmt.Errorf("%w: выбери команду", ErrInvalidInput)
	}

	var result Profile
	var created bool
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var employee models.Employee
		// Строка сотрудника существует ещё до профиля. Параллельный запрос дождётся её освобождения.
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Preload("Position").
			Take(&employee, "id = ?", employeeID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrEmployeeNotFound
		}
		if err != nil {
			return fmt.Errorf("загрузить сотрудника для регистрации: %w", err)
		}

		startDate, err := registrationDate(employee.Role, input)
		if err != nil {
			return err
		}
		positionID := input.PositionID
		if positionID == "" {
			positionID = employee.PositionID
		}

		profile, err := findProfile(tx, employee.ID)
		switch {
		case err == nil:
			if profile.TeamID != input.TeamID ||
				mentorID(profile) != input.MentorID ||
				dateString(profile.InternshipStartDate) != input.InternshipStartDate ||
				employee.PositionID != positionID {
				return ErrProfileAlreadyExists
			}
			result = profileView(profile, employee)
			return nil
		case !errors.Is(err, gorm.ErrRecordNotFound):
			return fmt.Errorf("проверить существующий профиль: %w", err)
		}

		result, err = createProfile(tx, employee, input, positionID, startDate)
		created = err == nil
		return err
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return Profile{}, false, err
	}
	return result, created, nil
}

func registrationForm(tx *gorm.DB, employee models.Employee) (LoginResult, error) {
	var teams []models.Team
	query := tx.Preload("Mentors", func(db *gorm.DB) *gorm.DB {
		return db.Order("first_name, second_name, id")
	}).Order("name, id")
	if employee.Role == models.RoleIntern {
		query = query.Where("EXISTS (SELECT 1 FROM mentors WHERE mentors.team_id = teams.id)")
	}
	if err := query.Find(&teams).Error; err != nil {
		return LoginResult{}, fmt.Errorf("загрузить команды: %w", err)
	}

	var positions []models.Position
	if err := tx.Order("name, id").Find(&positions).Error; err != nil {
		return LoginResult{}, fmt.Errorf("загрузить должности: %w", err)
	}
	if len(teams) == 0 || len(positions) == 0 {
		return LoginResult{}, ErrRegistrationUnavailable
	}

	result := LoginResult{
		Status:          LoginNeedsRegistration,
		EmployeeID:      employee.ID,
		FullName:        employee.FirstName + " " + employee.SecondName,
		Email:           employee.Email,
		Role:            employee.Role,
		PositionID:      employee.PositionID,
		TeamOptions:     make([]TeamOption, 0, len(teams)),
		PositionOptions: make([]Option, 0, len(positions)),
	}
	for _, team := range teams {
		option := TeamOption{
			Option:  Option{ID: team.ID, Name: team.Name},
			Mentors: make([]MentorOption, 0, len(team.Mentors)),
		}
		for _, mentor := range team.Mentors {
			option.Mentors = append(option.Mentors, MentorOption{
				ID: mentor.ID, FullName: mentor.FirstName + " " + mentor.SecondName,
			})
		}
		result.TeamOptions = append(result.TeamOptions, option)
	}
	for _, position := range positions {
		result.PositionOptions = append(result.PositionOptions, Option{ID: position.ID, Name: position.Name})
	}
	return result, nil
}

func createProfile(tx *gorm.DB, employee models.Employee, input RegistrationInput, positionID string, startDate *time.Time) (Profile, error) {
	// Справочники могут измениться после открытия формы. Сверяем выбор и удерживаем найденные записи до коммита.
	var position models.Position
	if err := loadReference(tx, &position, "должность", "id = ?", positionID); err != nil {
		return Profile{}, err
	}
	var team models.Team
	if err := loadReference(tx, &team, "команда", "id = ?", input.TeamID); err != nil {
		return Profile{}, err
	}

	profile := models.Profile{
		ID:                  rand.Text(),
		EmployeeID:          employee.ID,
		TeamID:              team.ID,
		InternshipStartDate: startDate,
	}
	if input.MentorID != "" {
		var mentor models.Mentor
		if err := loadReference(tx, &mentor, "наставник выбранной команды", "id = ? AND team_id = ?", input.MentorID, team.ID); err != nil {
			return Profile{}, err
		}
		profile.MentorID = &mentor.ID
	}
	if employee.PositionID != position.ID {
		if err := tx.Model(&models.Employee{}).Where("id = ?", employee.ID).
			Update("position_id", position.ID).Error; err != nil {
			return Profile{}, fmt.Errorf("сохранить должность: %w", err)
		}
	}
	if err := tx.Create(&profile).Error; err != nil {
		return Profile{}, fmt.Errorf("создать профиль: %w", err)
	}
	employee.Position = position
	profile.Team = team
	return profileView(profile, employee), nil
}

func loadReference(tx *gorm.DB, dest any, label, query string, args ...any) error {
	err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where(query, args...).Take(dest).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("%w: %s не найден в справочнике", ErrInvalidInput, label)
	}
	if err != nil {
		return fmt.Errorf("загрузить запись справочника (%s): %w", label, err)
	}
	return nil
}

func registrationDate(role models.EmployeeRole, input RegistrationInput) (*time.Time, error) {
	switch role {
	case models.RoleIntern:
		date, err := time.Parse(time.DateOnly, input.InternshipStartDate)
		if input.MentorID == "" || err != nil || date.Year() < 1 {
			return nil, fmt.Errorf("%w: выбери наставника и укажи дату начала стажировки", ErrInvalidInput)
		}
		return &date, nil
	case models.RoleManager:
		if input.MentorID != "" || input.InternshipStartDate != "" {
			return nil, fmt.Errorf("%w: для руководителя достаточно команды и должности", ErrInvalidInput)
		}
		return nil, nil
	default:
		return nil, ErrForbidden
	}
}

func findProfile(tx *gorm.DB, employeeID string) (models.Profile, error) {
	var profile models.Profile
	err := tx.Preload("Team").Take(&profile, "employee_id = ?", employeeID).Error
	return profile, err
}

func profileView(profile models.Profile, employee models.Employee) Profile {
	return Profile{
		ID:                  profile.ID,
		EmployeeID:          employee.ID,
		FullName:            employee.FirstName + " " + employee.SecondName,
		Email:               employee.Email,
		Role:                employee.Role,
		Position:            employee.Position.Name,
		Team:                Option{ID: profile.Team.ID, Name: profile.Team.Name},
		InternshipStartDate: dateString(profile.InternshipStartDate),
	}
}

func mentorID(profile models.Profile) string {
	if profile.MentorID == nil {
		return ""
	}
	return *profile.MentorID
}

func dateString(date *time.Time) string {
	if date == nil {
		return ""
	}
	return date.Format(time.DateOnly)
}
