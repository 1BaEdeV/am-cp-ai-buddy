package demoseed_test

import (
	"context"
	"testing"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/access"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/accessrequest"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/auth"
	appdb "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/db"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/demoseed"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/onboarding"
	demodata "github.com/1BaEdeV/am-cp-ai-buddy/backend/seeds/demo"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/tests/integration/testdb"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDemoRegistrationProvidesWorkingPlan(t *testing.T) {
	checkDemoRegistration(t, database(t))
}

func TestPostgresDemoRegistrationProvidesWorkingPlan(t *testing.T) {
	db := testdb.Postgres(t)
	require.NoError(t, appdb.Migrate(context.Background(), db))
	checkDemoRegistration(t, db)
}

func checkDemoRegistration(t *testing.T, db *gorm.DB) {
	t.Helper()
	ctx := context.Background()
	_, err := demoseed.Load(ctx, db, demodata.Files)
	require.NoError(t, err)
	authService := auth.NewService(db)
	// Конкретный сбой Ивана: должность существует, но у его команды нет таких правил.
	form, err := authService.Login(ctx, "intern-ivan")
	require.NoError(t, err)
	require.Equal(t, []auth.Option{{ID: "position-frontend", Name: "Frontend-разработчик"}}, form.PositionOptions)
	_, created, err := authService.Register(ctx, "intern-ivan", auth.RegistrationInput{
		PositionID: "position-backend-go", MentorID: "mentor-oleg", InternshipStartDate: "2026-11-16",
	})
	require.ErrorIs(t, err, auth.ErrInvalidInput)
	require.False(t, created)

	var interns []models.Employee
	require.NoError(t, db.Where("role = ?", models.RoleIntern).Order("id").Find(&interns).Error)
	registered := 0
	for _, intern := range interns {
		form, err := authService.Login(ctx, intern.ID)
		require.NoError(t, err, intern.ID)
		if form.Status == auth.LoginReady {
			continue
		}
		require.NotEmpty(t, form.PositionOptions, intern.ID)
		require.NotEmpty(t, form.Team.Mentors, intern.ID)
		for _, position := range form.PositionOptions {
			var count int64
			require.NoError(t, db.Model(&models.AccessRequirement{}).
				Where("team_id = ? AND position_id = ?", intern.TeamID, position.ID).Count(&count).Error)
			require.Positive(t, count, intern.ID)
		}
		_, created, err := authService.Register(ctx, intern.ID, auth.RegistrationInput{
			PositionID: form.PositionID, MentorID: form.Team.Mentors[0].ID, InternshipStartDate: "2026-11-16",
		})
		require.NoError(t, err, intern.ID)
		require.True(t, created, intern.ID)
		registered++
		card, err := access.NewService(db).ListRequiredAccesses(ctx, intern.ID)
		require.NoError(t, err, intern.ID)
		require.NotEmpty(t, card.Resources, intern.ID)
		plan, err := onboarding.NewService(db).GetPlan(ctx, intern.ID)
		require.NoError(t, err, intern.ID)
		require.Len(t, plan.Tasks, 3, intern.ID)
		require.Equal(t, card.Progress.Total, plan.AccessProgress.Total, intern.ID)
		prepared, err := accessrequest.NewService(db).PrepareAccessRequest(ctx, intern.ID, card.Resources[0].ID)
		require.NoError(t, err, intern.ID)
		require.Equal(t, accessrequest.PreparationReady, prepared.Status, intern.ID)
	}
	require.Equal(t, 9, registered)
}
