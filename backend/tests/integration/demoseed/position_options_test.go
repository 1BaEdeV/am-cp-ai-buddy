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

func TestTeamPositionsProvideWorkingRequests(t *testing.T) {
	checkTeamPositions(t, database(t))
}

func TestPostgresTeamPositionsProvideWorkingRequests(t *testing.T) {
	db := testdb.Postgres(t)
	require.NoError(t, appdb.Migrate(context.Background(), db))
	checkTeamPositions(t, db)
}

func checkTeamPositions(t *testing.T, db *gorm.DB) {
	t.Helper()
	ctx := context.Background()
	_, err := demoseed.Load(ctx, db, demodata.Files)
	require.NoError(t, err)
	authService := auth.NewService(db)
	form, err := authService.Login(ctx, "intern-ivan")
	require.NoError(t, err)
	accesses, requests, plans := access.NewService(db), accessrequest.NewService(db), onboarding.NewService(db)
	builder := chat.NewAppContextBuilder(plans, accesses, requests)
	positions := 0
	for _, team := range form.TeamOptions {
		require.GreaterOrEqual(t, len(team.PositionOptions), 3, team.ID)
		for _, position := range team.PositionOptions {
			positions++
			t.Run(team.ID+"/"+position.ID, func(t *testing.T) {
				employeeID := "test-" + team.ID + "-" + position.ID
				employee := models.Employee{ID: employeeID, Email: employeeID + "@example.test",
					FirstName: "Тестовый", SecondName: "Стажёр", Role: models.RoleIntern,
					TeamID: "team-web", PositionID: "position-frontend"}
				require.NoError(t, db.Create(&employee).Error)
				input := auth.RegistrationInput{TeamID: team.ID, PositionID: position.ID,
					MentorID: team.Mentors[0].ID, InternshipStartDate: "2026-11-16"}
				profile, created, err := authService.Register(ctx, employeeID, input)
				require.NoError(t, err)
				require.True(t, created)
				require.Equal(t, position.Name, profile.Position)
				card, err := accesses.ListRequiredAccesses(ctx, employeeID)
				require.NoError(t, err)
				require.GreaterOrEqual(t, len(card.Resources), 3)
				for _, resource := range card.Resources {
					require.Equal(t, access.StatusRequestable, resource.Status)
				}
				if team.ID == "team-web" {
					levels := map[string]string{}
					for _, resource := range card.Resources {
						levels[resource.ID] = resource.AccessLevel
					}
					switch position.ID {
					case "position-qa":
						require.Equal(t, "Reporter", levels["gitlab-onboarding"])
						require.Equal(t, "Viewer", levels["figma-design"])
					case "position-designer":
						require.Equal(t, "Editor", levels["figma-design"])
						require.NotContains(t, levels, "gitlab-onboarding")
					case "position-backend-go":
						require.Equal(t, "Developer", levels["gitlab-onboarding"])
						require.NotContains(t, levels, "figma-design")
					}
				}
				prepared, err := requests.PrepareAccessRequest(ctx, employeeID, card.Resources[0].ID)
				require.NoError(t, err)
				request, created, err := requests.ConfirmAccessRequest(ctx, employeeID, prepared.Draft.ID)
				require.NoError(t, err)
				require.True(t, created)
				require.Equal(t, models.AccessRequestPending, request.Status)
				var approver models.Employee
				require.NoError(t, db.Take(&approver, "id = ?", request.ApproverID).Error)
				require.Equal(t, team.ID, approver.TeamID)
				context, err := builder.Build(ctx, employeeID, "working-accesses")
				require.NoError(t, err)
				intro := context.(map[string]any)["teamIntro"].(*onboarding.TeamIntro)
				require.Equal(t, position.Name, intro.Trainee.Position)
				_, err = demoseed.Load(ctx, db, demodata.Files)
				require.NoError(t, err)
				saved, err := authService.Me(ctx, employeeID)
				require.NoError(t, err)
				require.Equal(t, profile, saved)
			})
		}
	}
	require.Equal(t, 31, positions)
}
