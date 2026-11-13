package auth_test

import (
	"context"
	"strings"
	"testing"

	authservice "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogin(t *testing.T) {
	t.Run("empty identifier -> ErrInvalidInput", func(t *testing.T) {
		s, _ := newService(t)
		_, err := s.Login(context.Background(), "   ")
		assert.ErrorIs(t, err, authservice.ErrInvalidInput)
	})

	t.Run("too long identifier -> ErrInvalidInput", func(t *testing.T) {
		s, _ := newService(t)
		_, err := s.Login(context.Background(), strings.Repeat("a", 255))
		assert.ErrorIs(t, err, authservice.ErrInvalidInput)
	})

	t.Run("id not found, no @ -> ErrEmployeeNotFound", func(t *testing.T) {
		s, _ := newService(t)
		_, err := s.Login(context.Background(), "missing-id")
		assert.ErrorIs(t, err, authservice.ErrEmployeeNotFound)
	})

	t.Run("email not found -> ErrEmployeeNotFound", func(t *testing.T) {
		s, _ := newService(t)
		_, err := s.Login(context.Background(), "ghost@example.com")
		assert.ErrorIs(t, err, authservice.ErrEmployeeNotFound)
	})

	t.Run("found by id with profile -> LoginReady", func(t *testing.T) {
		s, f := newService(t)
		_, _, err := s.Register(context.Background(), f.Manager, authservice.RegistrationInput{})
		require.NoError(t, err)

		res, err := s.Login(context.Background(), f.Manager)
		require.NoError(t, err)
		assert.Equal(t, authservice.LoginReady, res.Status)
		require.NotNil(t, res.Profile)
		assert.Equal(t, f.Manager, res.Profile.EmployeeID)
	})

	t.Run("found by email with profile -> LoginReady", func(t *testing.T) {
		s, f := newService(t)
		_, _, err := s.Register(context.Background(), f.Manager, authservice.RegistrationInput{})
		require.NoError(t, err)

		res, err := s.Login(context.Background(), "MANAGER@EXAMPLE.COM")
		require.NoError(t, err)
		assert.Equal(t, authservice.LoginReady, res.Status)
		require.NotNil(t, res.Profile)
	})

	t.Run("found without profile -> NeedsRegistration with form data", func(t *testing.T) {
		s, f := newService(t)

		res, err := s.Login(context.Background(), f.Manager)
		require.NoError(t, err)
		assert.Equal(t, authservice.LoginNeedsRegistration, res.Status)
		require.NotNil(t, res.Team)
		assert.Equal(t, f.Team1, res.Team.ID)
		assert.NotEmpty(t, res.PositionOptions)
	})

	t.Run("intern without mentors in assigned team -> ErrRegistrationUnavailable", func(t *testing.T) {
		s, f := newService(t)

		// Удаляем единственного ментора в команде стажёра
		require.NoError(t, s.db.Exec(`DELETE FROM mentors WHERE id = ?`, f.Mentor2).Error)

		_, err := s.Login(context.Background(), f.Intern)
		assert.ErrorIs(t, err, authservice.ErrRegistrationUnavailable)
	})

	t.Run("no positions -> ErrRegistrationUnavailable", func(t *testing.T) {
		s, f := newService(t)
		require.NoError(t, s.db.Exec(`DELETE FROM positions`).Error)

		_, err := s.Login(context.Background(), f.Manager)
		assert.ErrorIs(t, err, authservice.ErrRegistrationUnavailable)
	})

	t.Run("assigned team deleted -> ErrRegistrationUnavailable", func(t *testing.T) {
		s, f := newService(t)
		require.NoError(t, s.db.Exec(`DELETE FROM teams WHERE id = ?`, f.Team1).Error)

		_, err := s.Login(context.Background(), f.Manager)
		assert.ErrorIs(t, err, authservice.ErrRegistrationUnavailable)
	})
}
