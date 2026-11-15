package demoseed

import (
	"slices"
	"strings"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/access"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
)

func index[T any](name string, rows []T, id func(T) string) (map[string]T, error) {
	result := make(map[string]T, len(rows))
	for _, row := range rows {
		key := id(row)
		if strings.TrimSpace(key) == "" || key != strings.TrimSpace(key) {
			return nil, invalid("%s: пустой ID или пробелы вокруг ID", name)
		}
		if _, found := result[key]; found {
			return nil, invalid("%s: повторный ID %s", name, key)
		}
		result[key] = row
	}
	return result, nil
}

// Validate проверяет ссылки в самом наборе файлов, не полагаясь на существующую БД.
func (d Data) Validate() error {
	teams, err := index("teams", d.Teams, func(r models.Team) string { return r.ID })
	if err != nil {
		return err
	}
	positions, err := index("positions", d.Positions, func(r models.Position) string { return r.ID })
	if err != nil {
		return err
	}
	mentors, err := index("mentors", d.Mentors, func(r models.Mentor) string { return r.ID })
	if err != nil {
		return err
	}
	employees, err := index("employees", d.Employees, func(r models.Employee) string { return r.ID })
	if err != nil {
		return err
	}
	_, err = index("profiles", d.Profiles, func(r models.Profile) string { return r.ID })
	if err != nil {
		return err
	}
	resources, err := index("resources", d.Resources, func(r models.Resource) string { return r.ID })
	if err != nil {
		return err
	}
	_, err = index("access_requirements", d.Requirements, func(r models.AccessRequirement) string { return r.ID })
	if err != nil {
		return err
	}
	_, err = index("employee_accesses", d.Accesses, func(r models.EmployeeAccess) string { return r.ID })
	if err != nil {
		return err
	}
	if len(teams) == 0 || len(positions) == 0 || len(employees) == 0 || len(resources) == 0 {
		return invalid("нужны команды, должности, сотрудники и ресурсы")
	}
	for _, r := range d.Teams {
		if strings.TrimSpace(r.Name) == "" {
			return invalid("teams: нет названия %s", r.ID)
		}
	}
	for _, r := range d.Positions {
		if strings.TrimSpace(r.Name) == "" {
			return invalid("positions: нет названия %s", r.ID)
		}
	}
	for _, r := range d.Mentors {
		if teams[r.TeamID].ID == "" || strings.TrimSpace(r.FirstName) == "" || strings.TrimSpace(r.SecondName) == "" {
			return invalid("mentors: неверная команда или имя %s", r.ID)
		}
	}
	emails := map[string]bool{}
	for _, r := range d.Employees {
		email := strings.ToLower(strings.TrimSpace(r.Email))
		if teams[r.TeamID].ID == "" || positions[r.PositionID].ID == "" || !slices.Contains([]models.EmployeeRole{models.RoleIntern, models.RoleManager}, r.Role) ||
			email == "" || emails[email] || strings.TrimSpace(r.FirstName) == "" || strings.TrimSpace(r.SecondName) == "" {
			return invalid("employees: неверные данные или повторная почта %s", r.ID)
		}
		emails[email] = true
	}
	profileOwners := map[string]bool{}
	for _, r := range d.Profiles {
		e := employees[r.EmployeeID]
		if e.ID == "" || profileOwners[e.ID] {
			return invalid("profiles: неизвестный сотрудник или повторный профиль %s", r.ID)
		}
		profileOwners[e.ID] = true
		if e.Role == models.RoleManager {
			if r.MentorID != nil || r.InternshipStartDate != nil {
				return invalid("profiles: руководителю не нужны параметры стажировки %s", r.ID)
			}
		} else if r.MentorID == nil || mentors[*r.MentorID].ID == "" || mentors[*r.MentorID].TeamID != e.TeamID || r.InternshipStartDate == nil {
			return invalid("profiles: стажёру нужны наставник своей команды и дата %s", r.ID)
		}
	}
	externalKeys := map[[2]string]bool{}
	for _, r := range d.Resources {
		key := [2]string{r.Service, r.ExternalID}
		if strings.TrimSpace(r.Name) == "" || strings.TrimSpace(r.Service) == "" || strings.TrimSpace(r.ExternalID) == "" || len(r.AccessLevels) == 0 || externalKeys[key] {
			return invalid("resources: неверные данные или повторный внешний ресурс %s", r.ID)
		}
		externalKeys[key] = true
		levels := map[string]bool{}
		for _, level := range r.AccessLevels {
			if strings.TrimSpace(level) == "" || levels[level] {
				return invalid("resources: пустой или повторный уровень %s", r.ID)
			}
			levels[level] = true
		}
	}
	requirementKeys := map[[3]string]bool{}
	for _, r := range d.Requirements {
		key := [3]string{r.TeamID, r.PositionID, r.ResourceID}
		r.Resource, r.Approver = resources[r.ResourceID], employees[r.ApproverID]
		if teams[r.TeamID].ID == "" || positions[r.PositionID].ID == "" || !access.ValidRequirement(r, r.TeamID) || requirementKeys[key] {
			return invalid("access_requirements: неверное правило или дубль %s", r.ID)
		}
		requirementKeys[key] = true
	}
	accessKeys := map[[2]string]bool{}
	for _, r := range d.Accesses {
		key := [2]string{r.EmployeeID, r.ResourceID}
		if employees[r.EmployeeID].ID == "" || resources[r.ResourceID].ID == "" || !slices.Contains(resources[r.ResourceID].AccessLevels, r.AccessLevel) || r.VerifiedAt.IsZero() || accessKeys[key] {
			return invalid("employee_accesses: неверный доступ или дубль %s", r.ID)
		}
		accessKeys[key] = true
	}
	return nil
}
