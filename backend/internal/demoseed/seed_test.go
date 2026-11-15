package demoseed

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/db"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestSeedIsRepeatable(t *testing.T) {
	directory := t.TempDir()
	fixtures := map[string]string{
		"positions.yaml": `positions:
  - id: pos-intern
    name: Backend
  - id: pos-manager
    name: Lead
`,
		"teams.yaml": `teams:
  - id: team-search
    name: Поиск
    description: Поисковый сервис
`,
		"mentors.yaml": `mentors:
  - id: mentor-1
    team_id: team-search
    first_name: Катя
    second_name: Волкова
    contact: katya@example.test
`,
		"employees.yaml": `employees:
  - id: manager-1
    email: manager@example.test
    first_name: Дима
    second_name: Орлов
    position_id: pos-manager
    team_id: team-search
    role: manager
  - id: intern-1
    email: intern@example.test
    first_name: Иван
    second_name: Петров
    position_id: pos-intern
    team_id: team-search
    role: intern
`,
		"profiles.yaml": `profiles:
  - id: profile-1
    employee_id: intern-1
    mentor_id: mentor-1
    internship_start_date: "2026-11-01"
`,
		"resources.yaml": `resources:
  - id: wiki
    name: Вики
    service: wiki
    external_id: demo/wiki
    access_levels: [read]
`,
		"access_requirements.yaml": `access_requirements:
  - id: req-1
    team_id: team-search
    position_id: pos-intern
    resource_id: wiki
    access_level: read
    purpose: Инструкции
    approver_id: manager-1
`,
		"employee_accesses.yaml": `employee_accesses:
  - id: grant-1
    employee_id: intern-1
    resource_id: wiki
    access_level: read
    verified_at: "2026-11-01T09:00:00+03:00"
`,
	}
	for name, contents := range fixtures {
		require.NoError(t, os.WriteFile(filepath.Join(directory, name), []byte(contents), 0o600))
	}

	database, err := gorm.Open(sqlite.Open("file:seed-test?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.Migrate(context.Background(), database))
	for range 2 {
		require.NoError(t, Seed(context.Background(), database, directory))
	}
	var teams, mentors, employees, profiles, resources, requirements, grants, providerAccesses int64
	require.NoError(t, database.Model(&models.Team{}).Count(&teams).Error)
	require.NoError(t, database.Model(&models.Mentor{}).Count(&mentors).Error)
	require.NoError(t, database.Model(&models.Employee{}).Count(&employees).Error)
	require.NoError(t, database.Model(&models.Profile{}).Count(&profiles).Error)
	require.NoError(t, database.Model(&models.Resource{}).Count(&resources).Error)
	require.NoError(t, database.Model(&models.AccessRequirement{}).Count(&requirements).Error)
	require.NoError(t, database.Model(&models.EmployeeAccess{}).Count(&grants).Error)
	require.NoError(t, database.Model(&models.DemoProviderAccess{}).Count(&providerAccesses).Error)
	require.EqualValues(t, 1, teams)
	require.EqualValues(t, 1, mentors)
	require.EqualValues(t, 2, employees)
	require.EqualValues(t, 1, profiles)
	require.EqualValues(t, 1, resources)
	require.EqualValues(t, 1, requirements)
	require.EqualValues(t, 1, grants)
	require.EqualValues(t, 1, providerAccesses)
}
