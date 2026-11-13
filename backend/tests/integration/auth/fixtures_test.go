package auth_test

import (
	"fmt"
	"strings"
	"testing"

	authservice "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/auth"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	name := "memdb_" + strings.ReplaceAll(t.Name(), "/", "_")
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", name)

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	require.NoError(t, db.Callback().Query().
		Before("gorm:query").
		Register("test:strip_locking", func(db *gorm.DB) {
			delete(db.Statement.Clauses, "FOR")
		}))

	stmts := []string{
		`CREATE TABLE positions (id TEXT PRIMARY KEY, name TEXT NOT NULL)`,
		`CREATE TABLE teams (id TEXT PRIMARY KEY, name TEXT NOT NULL)`,
		`CREATE TABLE mentors (id TEXT PRIMARY KEY, team_id TEXT NOT NULL, first_name TEXT NOT NULL, second_name TEXT NOT NULL)`,
		`CREATE INDEX idx_mentors_team_id ON mentors(team_id)`,
		`CREATE TABLE employees (id TEXT PRIMARY KEY, email TEXT NOT NULL, first_name TEXT NOT NULL, second_name TEXT NOT NULL, position_id TEXT NOT NULL, team_id TEXT NOT NULL, role TEXT NOT NULL)`,
		`CREATE UNIQUE INDEX ux_employees_email ON employees(lower(email))`,
		`CREATE TABLE profiles (id TEXT PRIMARY KEY, employee_id TEXT NOT NULL UNIQUE, mentor_id TEXT, internship_start_date DATE)`,
	}
	for _, q := range stmts {
		require.NoError(t, db.Exec(q).Error)
	}

	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

type fixtures struct {
	Pos1, Pos2       string
	Team1, Team2     string
	Mentor1, Mentor2 string
	Manager, Intern  string
	OtherManager     string
}

func seedBase(t *testing.T, db *gorm.DB) fixtures {
	t.Helper()
	f := fixtures{
		Pos1: "pos-1", Pos2: "pos-2",
		Team1: "team-1", Team2: "team-2",
		Mentor1: "mentor-1", Mentor2: "mentor-2",
		Manager: "emp-manager", Intern: "emp-intern",
		OtherManager: "emp-manager-2",
	}
	require.NoError(t, db.Create(&models.Position{ID: f.Pos1, Name: "Backend"}).Error)
	require.NoError(t, db.Create(&models.Position{ID: f.Pos2, Name: "Frontend"}).Error)
	require.NoError(t, db.Create(&models.Team{ID: f.Team1, Name: "Alpha"}).Error)
	require.NoError(t, db.Create(&models.Team{ID: f.Team2, Name: "Beta"}).Error)
	require.NoError(t, db.Create(&models.Mentor{ID: f.Mentor1, TeamID: f.Team1, FirstName: "M", SecondName: "One"}).Error)
	require.NoError(t, db.Create(&models.Mentor{ID: f.Mentor2, TeamID: f.Team2, FirstName: "M", SecondName: "Two"}).Error)
	require.NoError(t, db.Create(&models.Employee{
		ID: f.Manager, Email: "manager@example.com", FirstName: "Boss", SecondName: "One",
		PositionID: f.Pos1, TeamID: f.Team1, Role: models.RoleManager,
	}).Error)
	require.NoError(t, db.Create(&models.Employee{
		ID: f.Intern, Email: "intern@example.com", FirstName: "Ivan", SecondName: "Petrov",
		PositionID: f.Pos2, TeamID: f.Team2, Role: models.RoleIntern,
	}).Error)
	require.NoError(t, db.Create(&models.Employee{
		ID: f.OtherManager, Email: "mgr2@example.com", FirstName: "Boss", SecondName: "Two",
		PositionID: f.Pos1, TeamID: f.Team1, Role: models.RoleManager,
	}).Error)
	return f
}

// insertRawEmployee создаёт сотрудника в обход проверок (для тестов невалидной роли).
func insertRawEmployee(t *testing.T, db *gorm.DB, id, email, posID, teamID, role string) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO employees (id, email, first_name, second_name, position_id, team_id, role)
		 VALUES (?, ?, 'X', 'Y', ?, ?, ?)`,
		id, email, posID, teamID, role,
	).Error)
}

func newService(t *testing.T) (*testService, fixtures) {
	t.Helper()
	db := newTestDB(t)
	f := seedBase(t, db)
	return &testService{Service: authservice.NewService(db), db: db}, f
}

func mustPosID(t *testing.T, db *gorm.DB, employeeID string) string {
	t.Helper()
	var posID string
	require.NoError(t, db.Raw(`SELECT position_id FROM employees WHERE id = ?`, employeeID).Scan(&posID).Error)
	return posID
}

type testService struct {
	*authservice.Service
	db *gorm.DB
}
