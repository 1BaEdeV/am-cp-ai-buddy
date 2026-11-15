package auth

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"github.com/stretchr/testify/assert"
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
		`CREATE TABLE access_requirements (id TEXT PRIMARY KEY, team_id TEXT NOT NULL, position_id TEXT NOT NULL)`,
		`CREATE TABLE positions (id TEXT PRIMARY KEY, name TEXT NOT NULL)`,
		`CREATE TABLE teams (id TEXT PRIMARY KEY, name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE mentors (id TEXT PRIMARY KEY, team_id TEXT NOT NULL, first_name TEXT NOT NULL, second_name TEXT NOT NULL, contact TEXT NOT NULL DEFAULT '')`,
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
	require.NoError(t, db.Exec(`INSERT INTO access_requirements (id, team_id, position_id) VALUES (?, ?, ?)`, "rule-intern", f.Team2, f.Pos2).Error)
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

func newService(t *testing.T) (*Service, fixtures) {
	t.Helper()
	db := newTestDB(t)
	f := seedBase(t, db)
	return NewService(db), f
}

func TestNewService(t *testing.T) {
	db := newTestDB(t)
	s := NewService(db)
	require.NotNil(t, s)
	assert.Same(t, db, s.db)
}

func TestMe(t *testing.T) {
	t.Run("empty id -> ErrEmployeeNotFound", func(t *testing.T) {
		s, _ := newService(t)
		_, err := s.Me(context.Background(), "   ")
		assert.ErrorIs(t, err, ErrEmployeeNotFound)
	})

	t.Run("employee not found", func(t *testing.T) {
		s, _ := newService(t)
		_, err := s.Me(context.Background(), "nope")
		assert.ErrorIs(t, err, ErrEmployeeNotFound)
	})

	t.Run("invalid role -> ErrForbidden", func(t *testing.T) {
		s, f := newService(t)
		insertRawEmployee(t, s.db, "emp-bad", "bad@example.com", f.Pos1, f.Team1, "unknown")
		_, err := s.Me(context.Background(), "emp-bad")
		assert.ErrorIs(t, err, ErrForbidden)
	})

	t.Run("profile missing -> ErrProfileRequired", func(t *testing.T) {
		s, f := newService(t)
		_, err := s.Me(context.Background(), f.Manager)
		assert.ErrorIs(t, err, ErrProfileRequired)
	})

	t.Run("manager with profile", func(t *testing.T) {
		s, f := newService(t)
		_, _, err := s.Register(context.Background(), f.Manager, RegistrationInput{})
		require.NoError(t, err)

		profile, err := s.Me(context.Background(), f.Manager)
		require.NoError(t, err)
		assert.Equal(t, f.Manager, profile.EmployeeID)
		assert.Equal(t, models.RoleManager, profile.Role)
		assert.Equal(t, f.Pos1, mustPosID(t, s.db, f.Manager))
	})

	t.Run("intern with profile", func(t *testing.T) {
		s, f := newService(t)
		_, _, err := s.Register(context.Background(), f.Intern, RegistrationInput{
			MentorID:            f.Mentor2,
			InternshipStartDate: "2024-01-15",
		})
		require.NoError(t, err)

		profile, err := s.Me(context.Background(), f.Intern)
		require.NoError(t, err)
		require.NotNil(t, profile.Mentor)
		assert.Equal(t, f.Mentor2, profile.Mentor.ID)
		assert.Equal(t, "2024-01-15", profile.InternshipStartDate)
	})
}

func TestLogin(t *testing.T) {
	t.Run("empty identifier -> ErrInvalidInput", func(t *testing.T) {
		s, _ := newService(t)
		_, err := s.Login(context.Background(), "   ")
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("too long identifier -> ErrInvalidInput", func(t *testing.T) {
		s, _ := newService(t)
		_, err := s.Login(context.Background(), strings.Repeat("a", 255))
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("id not found, no @ -> ErrEmployeeNotFound", func(t *testing.T) {
		s, _ := newService(t)
		_, err := s.Login(context.Background(), "missing-id")
		assert.ErrorIs(t, err, ErrEmployeeNotFound)
	})

	t.Run("email not found -> ErrEmployeeNotFound", func(t *testing.T) {
		s, _ := newService(t)
		_, err := s.Login(context.Background(), "ghost@example.com")
		assert.ErrorIs(t, err, ErrEmployeeNotFound)
	})

	t.Run("found by id with profile -> LoginReady", func(t *testing.T) {
		s, f := newService(t)
		_, _, err := s.Register(context.Background(), f.Manager, RegistrationInput{})
		require.NoError(t, err)

		res, err := s.Login(context.Background(), f.Manager)
		require.NoError(t, err)
		assert.Equal(t, LoginReady, res.Status)
		require.NotNil(t, res.Profile)
		assert.Equal(t, f.Manager, res.Profile.EmployeeID)
	})

	t.Run("found by email with profile -> LoginReady", func(t *testing.T) {
		s, f := newService(t)
		_, _, err := s.Register(context.Background(), f.Manager, RegistrationInput{})
		require.NoError(t, err)

		res, err := s.Login(context.Background(), "MANAGER@EXAMPLE.COM")
		require.NoError(t, err)
		assert.Equal(t, LoginReady, res.Status)
		require.NotNil(t, res.Profile)
	})

	t.Run("found without profile -> NeedsRegistration with form data", func(t *testing.T) {
		s, f := newService(t)

		res, err := s.Login(context.Background(), f.Manager)
		require.NoError(t, err)
		assert.Equal(t, LoginNeedsRegistration, res.Status)
		require.NotNil(t, res.Team)
		assert.Equal(t, f.Team1, res.Team.ID)
		assert.NotEmpty(t, res.PositionOptions)
	})

	t.Run("intern without mentors in assigned team -> ErrRegistrationUnavailable", func(t *testing.T) {
		s, f := newService(t)

		// Удаляем единственного ментора в команде стажёра
		require.NoError(t, s.db.Exec(`DELETE FROM mentors WHERE id = ?`, f.Mentor2).Error)

		_, err := s.Login(context.Background(), f.Intern)
		assert.ErrorIs(t, err, ErrRegistrationUnavailable)
	})

	t.Run("no positions -> ErrRegistrationUnavailable", func(t *testing.T) {
		s, f := newService(t)
		require.NoError(t, s.db.Exec(`DELETE FROM positions`).Error)

		_, err := s.Login(context.Background(), f.Manager)
		assert.ErrorIs(t, err, ErrRegistrationUnavailable)
	})

	t.Run("assigned team deleted -> ErrRegistrationUnavailable", func(t *testing.T) {
		s, f := newService(t)
		require.NoError(t, s.db.Exec(`DELETE FROM teams WHERE id = ?`, f.Team1).Error)

		_, err := s.Login(context.Background(), f.Manager)
		assert.ErrorIs(t, err, ErrRegistrationUnavailable)
	})
}

func TestRegister(t *testing.T) {
	t.Run("empty employeeID -> ErrEmployeeNotFound", func(t *testing.T) {
		s, _ := newService(t)
		_, _, err := s.Register(context.Background(), "   ", RegistrationInput{})
		assert.ErrorIs(t, err, ErrEmployeeNotFound)
	})

	t.Run("employee not found", func(t *testing.T) {
		s, _ := newService(t)
		_, _, err := s.Register(context.Background(), "nope", RegistrationInput{})
		assert.ErrorIs(t, err, ErrEmployeeNotFound)
	})

	t.Run("invalid role -> ErrForbidden", func(t *testing.T) {
		s, f := newService(t)
		insertRawEmployee(t, s.db, "emp-bad", "bad@example.com", f.Pos1, f.Team1, "unknown")
		_, _, err := s.Register(context.Background(), "emp-bad", RegistrationInput{})
		assert.ErrorIs(t, err, ErrForbidden)
	})

	t.Run("intern without mentor -> ErrInvalidInput", func(t *testing.T) {
		s, f := newService(t)
		_, _, err := s.Register(context.Background(), f.Intern, RegistrationInput{
			InternshipStartDate: "2024-01-15",
		})
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("intern without date -> ErrInvalidInput", func(t *testing.T) {
		s, f := newService(t)
		_, _, err := s.Register(context.Background(), f.Intern, RegistrationInput{
			MentorID: f.Mentor2,
		})
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("intern with bad date format -> ErrInvalidInput", func(t *testing.T) {
		s, f := newService(t)
		_, _, err := s.Register(context.Background(), f.Intern, RegistrationInput{
			MentorID:            f.Mentor2,
			InternshipStartDate: "15.01.2024",
		})
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("intern with date year < 1 -> ErrInvalidInput", func(t *testing.T) {
		s, f := newService(t)
		_, _, err := s.Register(context.Background(), f.Intern, RegistrationInput{
			MentorID:            f.Mentor2,
			InternshipStartDate: "0000-01-01",
		})
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("manager with mentor -> ErrInvalidInput", func(t *testing.T) {
		s, f := newService(t)
		_, _, err := s.Register(context.Background(), f.Manager, RegistrationInput{
			MentorID: f.Mentor1,
		})
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("manager with date -> ErrInvalidInput", func(t *testing.T) {
		s, f := newService(t)
		_, _, err := s.Register(context.Background(), f.Manager, RegistrationInput{
			InternshipStartDate: "2024-01-15",
		})
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("manager success -> created=true", func(t *testing.T) {
		s, f := newService(t)
		profile, created, err := s.Register(context.Background(), f.Manager, RegistrationInput{})
		require.NoError(t, err)
		assert.True(t, created)
		assert.Equal(t, f.Manager, profile.EmployeeID)
		assert.Equal(t, f.Team1, profile.Team.ID)
		assert.Nil(t, profile.Mentor)
		assert.Empty(t, profile.InternshipStartDate)
	})

	t.Run("intern success -> created=true with mentor", func(t *testing.T) {
		s, f := newService(t)
		profile, created, err := s.Register(context.Background(), f.Intern, RegistrationInput{
			MentorID:            f.Mentor2,
			InternshipStartDate: "2024-01-15",
		})
		require.NoError(t, err)
		assert.True(t, created)
		require.NotNil(t, profile.Mentor)
		assert.Equal(t, f.Mentor2, profile.Mentor.ID)
		assert.Equal(t, "2024-01-15", profile.InternshipStartDate)
	})

	t.Run("retry same input -> created=false, same profile", func(t *testing.T) {
		s, f := newService(t)
		input := RegistrationInput{
			MentorID:            f.Mentor2,
			InternshipStartDate: "2024-01-15",
		}
		p1, created1, err := s.Register(context.Background(), f.Intern, input)
		require.NoError(t, err)
		assert.True(t, created1)

		p2, created2, err := s.Register(context.Background(), f.Intern, input)
		require.NoError(t, err)
		assert.False(t, created2)
		assert.Equal(t, p1.ID, p2.ID)
	})

	t.Run("retry with different mentor -> ErrProfileAlreadyExists", func(t *testing.T) {
		s, f := newService(t)
		_, _, err := s.Register(context.Background(), f.Intern, RegistrationInput{
			MentorID:            f.Mentor2,
			InternshipStartDate: "2024-01-15",
		})
		require.NoError(t, err)

		// Повторяем регистрацию с другим наставником.
		_, _, err = s.Register(context.Background(), f.Intern, RegistrationInput{
			MentorID:            f.Mentor1,
			InternshipStartDate: "2024-01-15",
		})
		assert.ErrorIs(t, err, ErrProfileAlreadyExists)
	})

	t.Run("retry with different date -> ErrProfileAlreadyExists", func(t *testing.T) {
		s, f := newService(t)
		_, _, err := s.Register(context.Background(), f.Intern, RegistrationInput{
			MentorID:            f.Mentor2,
			InternshipStartDate: "2024-01-15",
		})
		require.NoError(t, err)

		_, _, err = s.Register(context.Background(), f.Intern, RegistrationInput{
			MentorID:            f.Mentor2,
			InternshipStartDate: "2024-02-01",
		})
		assert.ErrorIs(t, err, ErrProfileAlreadyExists)
	})

	t.Run("retry with different position -> ErrProfileAlreadyExists", func(t *testing.T) {
		s, f := newService(t)
		_, _, err := s.Register(context.Background(), f.Manager, RegistrationInput{})
		require.NoError(t, err)

		_, _, err = s.Register(context.Background(), f.Manager, RegistrationInput{
			PositionID: f.Pos2, // было pos-1, хотим pos-2
		})
		assert.ErrorIs(t, err, ErrProfileAlreadyExists)
	})

	t.Run("position change on first register updates employee", func(t *testing.T) {
		s, f := newService(t)
		profile, created, err := s.Register(context.Background(), f.Manager, RegistrationInput{
			PositionID: f.Pos2,
		})
		require.NoError(t, err)
		assert.True(t, created)
		assert.Equal(t, "Frontend", profile.Position)
		assert.Equal(t, f.Pos2, mustPosID(t, s.db, f.Manager))
	})

	t.Run("position not found -> ErrInvalidInput", func(t *testing.T) {
		s, f := newService(t)
		_, _, err := s.Register(context.Background(), f.Manager, RegistrationInput{
			PositionID: "does-not-exist",
		})
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("mentor from another team -> ErrInvalidInput", func(t *testing.T) {
		s, f := newService(t)
		// Intern в team-2, mentor-1 в team-1
		_, _, err := s.Register(context.Background(), f.Intern, RegistrationInput{
			MentorID:            f.Mentor1,
			InternshipStartDate: "2024-01-15",
		})
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("assigned team deleted -> ErrInvalidInput", func(t *testing.T) {
		s, f := newService(t)
		require.NoError(t, s.db.Exec(`DELETE FROM teams WHERE id = ?`, f.Team1).Error)

		_, _, err := s.Register(context.Background(), f.Manager, RegistrationInput{})
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("whitespace in input is trimmed", func(t *testing.T) {
		s, f := newService(t)
		profile, created, err := s.Register(context.Background(), f.Intern, RegistrationInput{
			MentorID:            "  " + f.Mentor2 + "  ",
			InternshipStartDate: "  2024-01-15  ",
		})
		require.NoError(t, err)
		assert.True(t, created)
		require.NotNil(t, profile.Mentor)
		assert.Equal(t, "2024-01-15", profile.InternshipStartDate)
	})
}

func TestRegistrationDate(t *testing.T) {
	t.Run("year one is a valid date", func(t *testing.T) {
		d, err := registrationDate(models.RoleIntern, RegistrationInput{
			MentorID: "m", InternshipStartDate: "0001-01-01",
		})
		require.NoError(t, err)
		require.NotNil(t, d)
		assert.Equal(t, "0001-01-01", d.Format(time.DateOnly))
	})

	t.Run("intern valid", func(t *testing.T) {
		d, err := registrationDate(models.RoleIntern, RegistrationInput{
			MentorID:            "m",
			InternshipStartDate: "2024-01-15",
		})
		require.NoError(t, err)
		require.NotNil(t, d)
		assert.Equal(t, "2024-01-15", d.Format(time.DateOnly))
	})

	t.Run("intern no mentor", func(t *testing.T) {
		_, err := registrationDate(models.RoleIntern, RegistrationInput{
			InternshipStartDate: "2024-01-15",
		})
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("intern no date", func(t *testing.T) {
		_, err := registrationDate(models.RoleIntern, RegistrationInput{MentorID: "m"})
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("intern bad date", func(t *testing.T) {
		_, err := registrationDate(models.RoleIntern, RegistrationInput{
			MentorID:            "m",
			InternshipStartDate: "not-a-date",
		})
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("manager clean", func(t *testing.T) {
		d, err := registrationDate(models.RoleManager, RegistrationInput{})
		require.NoError(t, err)
		assert.Nil(t, d)
	})

	t.Run("manager with mentor", func(t *testing.T) {
		_, err := registrationDate(models.RoleManager, RegistrationInput{MentorID: "m"})
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("manager with date", func(t *testing.T) {
		_, err := registrationDate(models.RoleManager, RegistrationInput{
			InternshipStartDate: "2024-01-15",
		})
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("unknown role", func(t *testing.T) {
		_, err := registrationDate("cto", RegistrationInput{})
		assert.ErrorIs(t, err, ErrForbidden)
	})
}

func TestMentorID(t *testing.T) {
	assert.Equal(t, "", mentorID(models.Profile{}))

	id := "m-1"
	assert.Equal(t, "m-1", mentorID(models.Profile{MentorID: &id}))
}

func TestDateString(t *testing.T) {
	assert.Equal(t, "", dateString(nil))

	d := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, "2024-01-15", dateString(&d))
}

func mustPosID(t *testing.T, db *gorm.DB, employeeID string) string {
	t.Helper()
	var posID string
	require.NoError(t, db.Raw(`SELECT position_id FROM employees WHERE id = ?`, employeeID).Scan(&posID).Error)
	return posID
}
