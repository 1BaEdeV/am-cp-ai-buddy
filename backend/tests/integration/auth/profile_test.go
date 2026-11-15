package auth_test

import (
	"context"
	"testing"

	authservice "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/auth"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMe(t *testing.T) {
	t.Run("empty id -> ErrEmployeeNotFound", func(t *testing.T) {
		s, _ := newService(t)
		_, err := s.Me(context.Background(), "   ")
		assert.ErrorIs(t, err, authservice.ErrEmployeeNotFound)
	})

	t.Run("employee not found", func(t *testing.T) {
		s, _ := newService(t)
		_, err := s.Me(context.Background(), "nope")
		assert.ErrorIs(t, err, authservice.ErrEmployeeNotFound)
	})

	t.Run("invalid role -> ErrForbidden", func(t *testing.T) {
		s, f := newService(t)
		insertRawEmployee(t, s.db, "emp-bad", "bad@example.com", f.Pos1, f.Team1, "unknown")
		_, err := s.Me(context.Background(), "emp-bad")
		assert.ErrorIs(t, err, authservice.ErrForbidden)
	})

	t.Run("profile missing -> ErrProfileRequired", func(t *testing.T) {
		s, f := newService(t)
		_, err := s.Me(context.Background(), f.Manager)
		assert.ErrorIs(t, err, authservice.ErrProfileRequired)
	})

	t.Run("manager with profile", func(t *testing.T) {
		s, f := newService(t)
		_, _, err := s.Register(context.Background(), f.Manager, authservice.RegistrationInput{})
		require.NoError(t, err)

		profile, err := s.Me(context.Background(), f.Manager)
		require.NoError(t, err)
		assert.Equal(t, f.Manager, profile.EmployeeID)
		assert.Equal(t, models.RoleManager, profile.Role)
		assert.Equal(t, f.Pos1, mustPosID(t, s.db, f.Manager))
	})

	t.Run("intern with profile", func(t *testing.T) {
		s, f := newService(t)
		_, _, err := s.Register(context.Background(), f.Intern, authservice.RegistrationInput{
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
