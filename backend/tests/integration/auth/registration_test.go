package auth_test

import (
	"context"
	"testing"

	authservice "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegister(t *testing.T) {
	t.Run("empty employeeID -> ErrEmployeeNotFound", func(t *testing.T) {
		s, _ := newService(t)
		_, _, err := s.Register(context.Background(), "   ", authservice.RegistrationInput{})
		assert.ErrorIs(t, err, authservice.ErrEmployeeNotFound)
	})

	t.Run("employee not found", func(t *testing.T) {
		s, _ := newService(t)
		_, _, err := s.Register(context.Background(), "nope", authservice.RegistrationInput{})
		assert.ErrorIs(t, err, authservice.ErrEmployeeNotFound)
	})

	t.Run("invalid role -> ErrForbidden", func(t *testing.T) {
		s, f := newService(t)
		insertRawEmployee(t, s.db, "emp-bad", "bad@example.com", f.Pos1, f.Team1, "unknown")
		_, _, err := s.Register(context.Background(), "emp-bad", authservice.RegistrationInput{})
		assert.ErrorIs(t, err, authservice.ErrForbidden)
	})

	t.Run("intern without mentor -> ErrInvalidInput", func(t *testing.T) {
		s, f := newService(t)
		_, _, err := s.Register(context.Background(), f.Intern, authservice.RegistrationInput{
			InternshipStartDate: "2024-01-15",
		})
		assert.ErrorIs(t, err, authservice.ErrInvalidInput)
	})

	t.Run("intern without date -> ErrInvalidInput", func(t *testing.T) {
		s, f := newService(t)
		_, _, err := s.Register(context.Background(), f.Intern, authservice.RegistrationInput{
			MentorID: f.Mentor2,
		})
		assert.ErrorIs(t, err, authservice.ErrInvalidInput)
	})

	t.Run("intern with bad date format -> ErrInvalidInput", func(t *testing.T) {
		s, f := newService(t)
		_, _, err := s.Register(context.Background(), f.Intern, authservice.RegistrationInput{
			MentorID:            f.Mentor2,
			InternshipStartDate: "15.01.2024",
		})
		assert.ErrorIs(t, err, authservice.ErrInvalidInput)
	})

	t.Run("intern with date year < 1 -> ErrInvalidInput", func(t *testing.T) {
		s, f := newService(t)
		_, _, err := s.Register(context.Background(), f.Intern, authservice.RegistrationInput{
			MentorID:            f.Mentor2,
			InternshipStartDate: "0000-01-01",
		})
		assert.ErrorIs(t, err, authservice.ErrInvalidInput)
	})

	t.Run("manager with mentor -> ErrInvalidInput", func(t *testing.T) {
		s, f := newService(t)
		_, _, err := s.Register(context.Background(), f.Manager, authservice.RegistrationInput{
			MentorID: f.Mentor1,
		})
		assert.ErrorIs(t, err, authservice.ErrInvalidInput)
	})

	t.Run("manager with date -> ErrInvalidInput", func(t *testing.T) {
		s, f := newService(t)
		_, _, err := s.Register(context.Background(), f.Manager, authservice.RegistrationInput{
			InternshipStartDate: "2024-01-15",
		})
		assert.ErrorIs(t, err, authservice.ErrInvalidInput)
	})

	t.Run("manager success -> created=true", func(t *testing.T) {
		s, f := newService(t)
		profile, created, err := s.Register(context.Background(), f.Manager, authservice.RegistrationInput{})
		require.NoError(t, err)
		assert.True(t, created)
		assert.Equal(t, f.Manager, profile.EmployeeID)
		assert.Equal(t, f.Team1, profile.Team.ID)
		assert.Nil(t, profile.Mentor)
		assert.Empty(t, profile.InternshipStartDate)
	})

	t.Run("intern success -> created=true with mentor", func(t *testing.T) {
		s, f := newService(t)
		profile, created, err := s.Register(context.Background(), f.Intern, authservice.RegistrationInput{
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
		input := authservice.RegistrationInput{
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
		_, _, err := s.Register(context.Background(), f.Intern, authservice.RegistrationInput{
			MentorID:            f.Mentor2,
			InternshipStartDate: "2024-01-15",
		})
		require.NoError(t, err)

		// Повторяем регистрацию с другим наставником.
		_, _, err = s.Register(context.Background(), f.Intern, authservice.RegistrationInput{
			MentorID:            f.Mentor1,
			InternshipStartDate: "2024-01-15",
		})
		assert.ErrorIs(t, err, authservice.ErrProfileAlreadyExists)
	})

	t.Run("retry with different date -> ErrProfileAlreadyExists", func(t *testing.T) {
		s, f := newService(t)
		_, _, err := s.Register(context.Background(), f.Intern, authservice.RegistrationInput{
			MentorID:            f.Mentor2,
			InternshipStartDate: "2024-01-15",
		})
		require.NoError(t, err)

		_, _, err = s.Register(context.Background(), f.Intern, authservice.RegistrationInput{
			MentorID:            f.Mentor2,
			InternshipStartDate: "2024-02-01",
		})
		assert.ErrorIs(t, err, authservice.ErrProfileAlreadyExists)
	})

	t.Run("retry with different position -> ErrProfileAlreadyExists", func(t *testing.T) {
		s, f := newService(t)
		_, _, err := s.Register(context.Background(), f.Manager, authservice.RegistrationInput{})
		require.NoError(t, err)

		_, _, err = s.Register(context.Background(), f.Manager, authservice.RegistrationInput{
			PositionID: f.Pos2, // было pos-1, хотим pos-2
		})
		assert.ErrorIs(t, err, authservice.ErrProfileAlreadyExists)
	})

	t.Run("position change on first register updates employee", func(t *testing.T) {
		s, f := newService(t)
		profile, created, err := s.Register(context.Background(), f.Manager, authservice.RegistrationInput{
			PositionID: f.Pos2,
		})
		require.NoError(t, err)
		assert.True(t, created)
		assert.Equal(t, "Frontend", profile.Position)
		assert.Equal(t, f.Pos2, mustPosID(t, s.db, f.Manager))
	})

	t.Run("position not found -> ErrInvalidInput", func(t *testing.T) {
		s, f := newService(t)
		_, _, err := s.Register(context.Background(), f.Manager, authservice.RegistrationInput{
			PositionID: "does-not-exist",
		})
		assert.ErrorIs(t, err, authservice.ErrInvalidInput)
	})

	t.Run("mentor from another team -> ErrInvalidInput", func(t *testing.T) {
		s, f := newService(t)
		// Intern в team-2, mentor-1 в team-1
		_, _, err := s.Register(context.Background(), f.Intern, authservice.RegistrationInput{
			MentorID:            f.Mentor1,
			InternshipStartDate: "2024-01-15",
		})
		assert.ErrorIs(t, err, authservice.ErrInvalidInput)
	})

	t.Run("assigned team deleted -> ErrInvalidInput", func(t *testing.T) {
		s, f := newService(t)
		require.NoError(t, s.db.Exec(`DELETE FROM teams WHERE id = ?`, f.Team1).Error)

		_, _, err := s.Register(context.Background(), f.Manager, authservice.RegistrationInput{})
		assert.ErrorIs(t, err, authservice.ErrInvalidInput)
	})

	t.Run("whitespace in input is trimmed", func(t *testing.T) {
		s, f := newService(t)
		profile, created, err := s.Register(context.Background(), f.Intern, authservice.RegistrationInput{
			MentorID:            "  " + f.Mentor2 + "  ",
			InternshipStartDate: "  2024-01-15  ",
		})
		require.NoError(t, err)
		assert.True(t, created)
		require.NotNil(t, profile.Mentor)
		assert.Equal(t, "2024-01-15", profile.InternshipStartDate)
	})
}
