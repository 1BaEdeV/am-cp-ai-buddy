package demoseed_test

import (
	"context"
	"testing"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/access"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/accessrequest"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/auth"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/chat"
	appdb "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/db"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/demoseed"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/onboarding"
	demodata "github.com/1BaEdeV/am-cp-ai-buddy/backend/seeds/demo"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/tests/integration/testdb"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSelectedTeamAndMentorReachChatContext(t *testing.T) {
	checkTeamSelection(t, database(t))
}

func TestPostgresSelectedTeamAndMentorReachChatContext(t *testing.T) {
	db := testdb.Postgres(t)
	require.NoError(t, appdb.Migrate(context.Background(), db))
	checkTeamSelection(t, db)
}

func checkTeamSelection(t *testing.T, db *gorm.DB) {
	t.Helper()
	ctx := context.Background()
	_, err := demoseed.Load(ctx, db, demodata.Files)
	require.NoError(t, err)
	s := auth.NewService(db)
	form, err := s.Login(ctx, "intern-ivan")
	require.NoError(t, err)
	require.Len(t, form.TeamOptions, 10)
	for _, team := range form.TeamOptions {
		require.Len(t, team.Mentors, 2, team.ID)
		require.NotEmpty(t, team.PositionOptions, team.ID)
	}
	name := "Сидоров Алексей Сергеевич"
	input := auth.RegistrationInput{FullName: &name, TeamID: "team-search", PositionID: "position-backend-go",
		MentorID: "mentor-oleg", InternshipStartDate: "2026-11-16"}
	_, created, err := s.Register(ctx, "intern-ivan", input)
	require.ErrorIs(t, err, auth.ErrInvalidInput, "наставник другой команды не подходит")
	require.False(t, created)
	var employee models.Employee
	require.NoError(t, db.Take(&employee, "id = ?", "intern-ivan").Error)
	require.Equal(t, "team-web", employee.TeamID, "неудачный выбор не меняет сотрудника")
	input.MentorID = "mentor-sasha"
	profile, created, err := s.Register(ctx, "intern-ivan", input)
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, "team-search", profile.Team.ID)
	require.Equal(t, "mentor-sasha", profile.Mentor.ID)
	require.Equal(t, "Алексей", profile.FirstName)
	require.Equal(t, "Сидоров", profile.SecondName)
	require.Equal(t, "Сергеевич", profile.MiddleName)
	_, repeated, err := s.Register(ctx, "intern-ivan", input)
	require.NoError(t, err)
	require.False(t, repeated)
	changed := input
	changed.TeamID = "team-web"
	_, _, err = s.Register(ctx, "intern-ivan", changed)
	require.ErrorIs(t, err, auth.ErrProfileAlreadyExists)
	_, _, err = s.Register(ctx, "manager-anna", auth.RegistrationInput{TeamID: "team-search"})
	require.ErrorIs(t, err, auth.ErrInvalidInput, "роль руководителя не позволяет менять команду")
	accesses, requests, planService := access.NewService(db), accessrequest.NewService(db), onboarding.NewService(db)
	card, err := accesses.ListRequiredAccesses(ctx, "intern-ivan")
	require.NoError(t, err)
	require.Equal(t, 4, card.Progress.Total)
	resourceIDs := make([]string, 0, len(card.Resources))
	for _, r := range card.Resources {
		resourceIDs = append(resourceIDs, r.ID)
	}
	require.Contains(t, resourceIDs, "gitlab-search-api")
	require.NotContains(t, resourceIDs, "figma-design")
	prepared, err := requests.PrepareAccessRequest(ctx, "intern-ivan", "gitlab-search-api")
	require.NoError(t, err)
	require.Equal(t, "manager-dmitry", prepared.Draft.ApproverID)
	plan, err := planService.GetPlan(ctx, "intern-ivan")
	require.NoError(t, err)
	require.Len(t, plan.Tasks, 3)
	builder := chat.NewAppContextBuilder(planService, accesses, requests)
	for _, taskID := range []string{"team-intro", "working-accesses"} {
		context, err := builder.Build(ctx, "intern-ivan", taskID)
		require.NoError(t, err)
		intro := context.(map[string]any)["teamIntro"].(*onboarding.TeamIntro)
		require.Equal(t, "Алексей Сергеевич Сидоров", intro.Trainee.FullName)
		require.Equal(t, "Backend-разработчик Go", intro.Trainee.Position)
		require.Equal(t, "Поиск", intro.Team.Name)
		require.Equal(t, "Саша Лебедев", intro.Mentor.FullName)
		require.Contains(t, intro.Mentor.About, "качества поиска")
		require.Equal(t, "sasha.search@example.test", intro.Mentor.Email)
		require.Equal(t, "Дмитрий Орлов", intro.Manager.FullName)
		require.NotEmpty(t, intro.Manager.About)
		require.NotEmpty(t, intro.Manager.Email)
		require.Contains(t, intro.Description, "индексирование")
	}
	// Повторный seed и явное обновление знаний сохраняют выбранный профиль и черновик.
	_, err = demoseed.Load(ctx, db, demodata.Files)
	require.NoError(t, err)
	require.NoError(t, db.Model(&models.Team{}).Where("id = ?", "team-search").Update("description", "Старое описание").Error)
	require.NoError(t, demoseed.RefreshKnowledge(ctx, db, demodata.Files))
	saved, err := s.Me(ctx, "intern-ivan")
	require.NoError(t, err)
	require.Equal(t, profile, saved)
	intro, err := planService.GetTeamIntro(ctx, "intern-ivan")
	require.NoError(t, err)
	require.Contains(t, intro.TeamIntro.Description, "индексирование")
	var draft models.AccessDraft
	require.NoError(t, db.Take(&draft, "id = ?", prepared.Draft.ID).Error)
	require.Equal(t, "intern-ivan", draft.EmployeeID)
}
