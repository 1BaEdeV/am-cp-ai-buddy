// Package demoseed читает и атомарно загружает исходные данные демо.
package demoseed

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"time"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"gopkg.in/yaml.v3"
)

var ErrInvalidData = errors.New("некорректные демо-данные")

type Data struct {
	Teams        []models.Team
	Positions    []models.Position
	Mentors      []models.Mentor
	Employees    []models.Employee
	Profiles     []models.Profile
	Resources    []models.Resource
	Requirements []models.AccessRequirement
	Accesses     []models.EmployeeAccess
}

type reference struct {
	ID   string `yaml:"id"`
	Name string `yaml:"name"`
}

type mentor struct {
	ID         string `yaml:"id"`
	TeamID     string `yaml:"team_id"`
	FirstName  string `yaml:"first_name"`
	SecondName string `yaml:"second_name"`
}

type employee struct {
	ID         string              `yaml:"id"`
	Email      string              `yaml:"email"`
	FirstName  string              `yaml:"first_name"`
	SecondName string              `yaml:"second_name"`
	TeamID     string              `yaml:"team_id"`
	PositionID string              `yaml:"position_id"`
	Role       models.EmployeeRole `yaml:"role"`
}

type profile struct {
	ID         string  `yaml:"id"`
	EmployeeID string  `yaml:"employee_id"`
	MentorID   *string `yaml:"mentor_id"`
	StartDate  *string `yaml:"internship_start_date"`
}

type resource struct {
	ID           string   `yaml:"id"`
	Name         string   `yaml:"name"`
	Service      string   `yaml:"service"`
	ExternalID   string   `yaml:"external_id"`
	AccessLevels []string `yaml:"access_levels"`
}

type requirement struct {
	ID          string `yaml:"id"`
	TeamID      string `yaml:"team_id"`
	PositionID  string `yaml:"position_id"`
	ResourceID  string `yaml:"resource_id"`
	AccessLevel string `yaml:"access_level"`
	Purpose     string `yaml:"purpose"`
	ApproverID  string `yaml:"approver_id"`
}

type employeeAccess struct {
	ID          string `yaml:"id"`
	EmployeeID  string `yaml:"employee_id"`
	ResourceID  string `yaml:"resource_id"`
	AccessLevel string `yaml:"access_level"`
	VerifiedAt  string `yaml:"verified_at"`
}

// Read проверяет все файлы до первого обращения к БД. Неизвестные поля,
// повторные документы, дубли и неверные ссылки считаются ошибкой.
func Read(files fs.FS) (Data, error) {
	var data Data
	teams, err := decode[reference](files, "teams")
	if err != nil {
		return Data{}, err
	}
	for _, r := range teams {
		data.Teams = append(data.Teams, models.Team{ID: r.ID, Name: r.Name})
	}
	positions, err := decode[reference](files, "positions")
	if err != nil {
		return Data{}, err
	}
	for _, r := range positions {
		data.Positions = append(data.Positions, models.Position{ID: r.ID, Name: r.Name})
	}
	mentors, err := decode[mentor](files, "mentors")
	if err != nil {
		return Data{}, err
	}
	for _, r := range mentors {
		data.Mentors = append(data.Mentors, models.Mentor{ID: r.ID, TeamID: r.TeamID, FirstName: r.FirstName, SecondName: r.SecondName})
	}
	employees, err := decode[employee](files, "employees")
	if err != nil {
		return Data{}, err
	}
	for _, r := range employees {
		data.Employees = append(data.Employees, models.Employee{ID: r.ID, Email: r.Email, FirstName: r.FirstName, SecondName: r.SecondName, TeamID: r.TeamID, PositionID: r.PositionID, Role: r.Role})
	}
	profiles, err := decode[profile](files, "profiles")
	if err != nil {
		return Data{}, err
	}
	for _, r := range profiles {
		p := models.Profile{ID: r.ID, EmployeeID: r.EmployeeID, MentorID: r.MentorID}
		if r.StartDate != nil {
			date, err := time.Parse(time.DateOnly, *r.StartDate)
			if err != nil {
				return Data{}, invalid("profiles: дата профиля %s: %v", r.ID, err)
			}
			p.InternshipStartDate = &date
		}
		data.Profiles = append(data.Profiles, p)
	}
	resources, err := decode[resource](files, "resources")
	if err != nil {
		return Data{}, err
	}
	for _, r := range resources {
		data.Resources = append(data.Resources, models.Resource{ID: r.ID, Name: r.Name, Service: r.Service, ExternalID: r.ExternalID, AccessLevels: r.AccessLevels})
	}
	requirements, err := decode[requirement](files, "access_requirements")
	if err != nil {
		return Data{}, err
	}
	for _, r := range requirements {
		data.Requirements = append(data.Requirements, models.AccessRequirement{ID: r.ID, TeamID: r.TeamID, PositionID: r.PositionID, ResourceID: r.ResourceID, AccessLevel: r.AccessLevel, Purpose: r.Purpose, ApproverID: r.ApproverID})
	}
	accesses, err := decode[employeeAccess](files, "employee_accesses")
	if err != nil {
		return Data{}, err
	}
	for _, r := range accesses {
		verified, err := time.Parse(time.RFC3339, r.VerifiedAt)
		if err != nil {
			return Data{}, invalid("employee_accesses: время проверки %s: %v", r.ID, err)
		}
		data.Accesses = append(data.Accesses, models.EmployeeAccess{ID: r.ID, EmployeeID: r.EmployeeID, ResourceID: r.ResourceID, AccessLevel: r.AccessLevel, VerifiedAt: verified})
	}
	if err := data.Validate(); err != nil {
		return Data{}, err
	}
	return data, nil
}

func decode[T any](files fs.FS, key string) ([]T, error) {
	name := key + ".yaml"
	content, err := fs.ReadFile(files, name)
	if err != nil {
		return nil, fmt.Errorf("прочитать %s: %w", name, err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)
	var document map[string][]T
	if err := decoder.Decode(&document); err != nil {
		return nil, invalid("%s: %v", name, err)
	}
	records, ok := document[key]
	if !ok || len(document) != 1 || records == nil {
		return nil, invalid("%s: ожидается список %s", name, key)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, invalid("%s: допускается ровно один YAML-документ", name)
	}
	return records, nil
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidData, fmt.Sprintf(format, args...))
}
