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

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// Me загружает профиль по внутреннему ID из заголовка демо-авторизации.
func (s *Service) Me(ctx context.Context, employeeID string) (Profile, error) {
	employeeID = strings.TrimSpace(employeeID)
	if employeeID == "" {
		return Profile{}, ErrEmployeeNotFound
	}

	var result Profile
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var employee models.Employee
		err := tx.Preload("Position").Preload("Team").Take(&employee, "id = ?", employeeID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrEmployeeNotFound
		}
		if err != nil {
			return fmt.Errorf("найти сотрудника: %w", err)
		}
		if employee.Role != models.RoleIntern && employee.Role != models.RoleManager {
			return ErrForbidden
		}
		profile, err := findProfile(tx, employee.ID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrProfileRequired
		}
		if err != nil {
			return fmt.Errorf("найти профиль: %w", err)
		}
		result = profileView(profile, employee)
		return nil
	}, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return Profile{}, err
	}
	return result, nil
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
		err := tx.Preload("Position").Preload("Team").Take(&employee, "id = ?", identifier).Error
		if errors.Is(err, gorm.ErrRecordNotFound) && strings.Contains(identifier, "@") {
			err = tx.Preload("Position").Preload("Team").Take(&employee, "lower(email) = lower(?)", identifier).Error
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

	var result Profile
	var created bool
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var employee models.Employee
		// Строка сотрудника существует ещё до профиля. Параллельный запрос дождётся её освобождения.
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Preload("Position").Preload("Team").
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
		name, err := registrationName(input.FullName, employee)
		if err != nil {
			return err
		}
		teamID := input.TeamID
		if teamID == "" {
			teamID = employee.TeamID
		}
		if employee.Role == models.RoleManager && teamID != employee.TeamID {
			return fmt.Errorf("%w: руководитель не может менять команду при регистрации", ErrInvalidInput)
		}
		positionID := input.PositionID
		if positionID == "" {
			positionID = employee.PositionID
		}

		profile, err := findProfile(tx, employee.ID)
		switch {
		case err == nil:
			if mentorID(profile) != input.MentorID ||
				dateString(profile.InternshipStartDate) != input.InternshipStartDate ||
				employee.PositionID != positionID || employee.TeamID != teamID ||
				employee.FirstName != name.first || employee.SecondName != name.second || employee.MiddleName != name.middle {
				return ErrProfileAlreadyExists
			}
			result = profileView(profile, employee)
			return nil
		case !errors.Is(err, gorm.ErrRecordNotFound):
			return fmt.Errorf("проверить существующий профиль: %w", err)
		}

		employee.FirstName, employee.SecondName, employee.MiddleName = name.first, name.second, name.middle
		result, err = createProfile(tx, employee, input, teamID, positionID, startDate)
		created = err == nil
		return err
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return Profile{}, false, err
	}
	return result, created, nil
}

func createProfile(tx *gorm.DB, employee models.Employee, input RegistrationInput, teamID, positionID string, startDate *time.Time) (Profile, error) {
	// Справочники могут измениться после открытия формы. Сверяем выбор и удерживаем найденные записи до коммита.
	var position models.Position
	if err := loadReference(tx, &position, "должность", "id = ?", positionID); err != nil {
		return Profile{}, err
	}
	var team models.Team
	if err := loadReference(tx, &team, "выбранная команда", "id = ?", teamID); err != nil {
		return Profile{}, err
	}
	if employee.Role == models.RoleIntern {
		// Наличие должности в общем справочнике не означает, что для неё готов план команды.
		var requirement models.AccessRequirement
		if err := loadReference(tx, &requirement, "правила доступов для команды и должности",
			"team_id = ? AND position_id = ?", team.ID, position.ID); err != nil {
			return Profile{}, err
		}
	}

	profile := models.Profile{
		ID:                  rand.Text(),
		EmployeeID:          employee.ID,
		InternshipStartDate: startDate,
	}
	var mentor *models.Mentor
	if input.MentorID != "" {
		var selected models.Mentor
		if err := loadReference(tx, &selected, "наставник выбранной команды", "id = ? AND team_id = ?", input.MentorID, team.ID); err != nil {
			return Profile{}, err
		}
		profile.MentorID = &selected.ID
		mentor = &selected
	}
	if err := tx.Model(&models.Employee{}).Where("id = ?", employee.ID).
		Updates(map[string]any{"position_id": position.ID, "team_id": team.ID,
			"first_name": employee.FirstName, "second_name": employee.SecondName, "middle_name": employee.MiddleName}).Error; err != nil {
		return Profile{}, fmt.Errorf("сохранить ФИО, команду и должность: %w", err)
	}
	employee.TeamID, employee.PositionID = team.ID, position.ID
	if err := tx.Create(&profile).Error; err != nil {
		return Profile{}, fmt.Errorf("создать профиль: %w", err)
	}
	employee.Position = position
	employee.Team = team
	profile.Mentor = mentor
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
			return nil, fmt.Errorf("%w: для руководителя достаточно должности", ErrInvalidInput)
		}
		return nil, nil
	default:
		return nil, ErrForbidden
	}
}

func findProfile(tx *gorm.DB, employeeID string) (models.Profile, error) {
	var profile models.Profile
	err := tx.Preload("Mentor").Take(&profile, "employee_id = ?", employeeID).Error
	return profile, err
}

func profileView(profile models.Profile, employee models.Employee) Profile {
	view := Profile{
		ID:                  profile.ID,
		EmployeeID:          employee.ID,
		FirstName:           employee.FirstName,
		SecondName:          employee.SecondName,
		MiddleName:          employee.MiddleName,
		Email:               employee.Email,
		Role:                employee.Role,
		Position:            employee.Position.Name,
		Team:                Option{ID: employee.Team.ID, Name: employee.Team.Name},
		InternshipStartDate: dateString(profile.InternshipStartDate),
	}
	if profile.Mentor != nil {
		view.Mentor = &MentorOption{
			ID:         profile.Mentor.ID,
			FirstName:  profile.Mentor.FirstName,
			SecondName: profile.Mentor.SecondName,
		}
	}
	return view
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
