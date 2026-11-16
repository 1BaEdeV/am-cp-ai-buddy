package auth

import (
	"context"
	"testing"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"github.com/stretchr/testify/require"
)

func TestRegistrationPositionsMatchTeamRules(t *testing.T) {
	t.Run("form excludes positions without rules for the assigned team", func(t *testing.T) {
		s, f := newService(t)
		// Эта должность существует, но подготовлена только в другой команде.
		require.NoError(t, s.db.Exec(`INSERT INTO access_requirements VALUES (?, ?, ?)`, "other-team-rule", f.Team1, f.Pos1).Error)
		form, err := s.Login(context.Background(), f.Intern)
		require.NoError(t, err)
		require.Equal(t, []Option{{ID: f.Pos2, Name: "Frontend"}}, form.PositionOptions)
		require.Equal(t, f.Pos2, form.PositionID)
	})
	t.Run("form selects a supported default when employee position is not configured", func(t *testing.T) {
		s, f := newService(t)
		require.NoError(t, s.db.Model(&models.Employee{}).Where("id = ?", f.Intern).Update("position_id", f.Pos1).Error)
		form, err := s.Login(context.Background(), f.Intern)
		require.NoError(t, err)
		require.Equal(t, f.Pos2, form.PositionID)
	})
	t.Run("missing team rules makes registration unavailable", func(t *testing.T) {
		s, f := newService(t)
		require.NoError(t, s.db.Exec(`DELETE FROM access_requirements WHERE team_id = ?`, f.Team2).Error)
		_, err := s.Login(context.Background(), f.Intern)
		require.ErrorIs(t, err, ErrRegistrationUnavailable)
	})
	t.Run("server rejects a forged position without creating a broken profile", func(t *testing.T) {
		s, f := newService(t)
		require.NoError(t, s.db.Exec(`INSERT INTO access_requirements VALUES (?, ?, ?)`, "other-team-rule", f.Team1, f.Pos1).Error)
		_, created, err := s.Register(context.Background(), f.Intern, RegistrationInput{
			PositionID: f.Pos1, MentorID: f.Mentor2, InternshipStartDate: "2026-11-16",
		})
		require.ErrorIs(t, err, ErrInvalidInput)
		require.False(t, created)
		var count int64
		require.NoError(t, s.db.Model(&models.Profile{}).Where("employee_id = ?", f.Intern).Count(&count).Error)
		require.Zero(t, count)
		require.Equal(t, f.Pos2, mustPosID(t, s.db, f.Intern))
	})
	t.Run("server rechecks rules removed after opening the form", func(t *testing.T) {
		s, f := newService(t)
		_, err := s.Login(context.Background(), f.Intern)
		require.NoError(t, err)
		require.NoError(t, s.db.Exec(`DELETE FROM access_requirements`).Error)
		_, created, err := s.Register(context.Background(), f.Intern, RegistrationInput{
			PositionID: f.Pos2, MentorID: f.Mentor2, InternshipStartDate: "2026-11-16",
		})
		require.ErrorIs(t, err, ErrInvalidInput)
		require.False(t, created)
	})
	t.Run("manager positions do not require trainee access rules", func(t *testing.T) {
		s, f := newService(t)
		form, err := s.Login(context.Background(), f.Manager)
		require.NoError(t, err)
		require.Len(t, form.PositionOptions, 2)
		_, created, err := s.Register(context.Background(), f.Manager, RegistrationInput{PositionID: f.Pos2})
		require.NoError(t, err)
		require.True(t, created)
	})
}
