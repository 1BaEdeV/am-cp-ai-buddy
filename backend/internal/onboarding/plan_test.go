package onboarding

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	accessservice "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/access"
	accessrequest "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/accessrequest"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/auth"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type accessReaderStub struct {
	result accessservice.WorkingAccesses
	err    error
}

func (s accessReaderStub) ListRequiredAccesses(context.Context, string) (accessservice.WorkingAccesses, error) {
	return s.result, s.err
}

type requestReaderStub struct {
	result accessrequest.RequestList
	err    error
}

func (s requestReaderStub) ListAccessRequests(context.Context, string, accessrequest.RequestFilter) (accessrequest.RequestList, error) {
	return s.result, s.err
}

func newPlanTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.Position{}, &models.Team{}, &models.Mentor{}, &models.Employee{},
		&models.Profile{}, &models.TeamIntroduction{},
	))
	require.NoError(t, db.Create(&models.Position{ID: "pos-intern", Name: "Go-разработчик"}).Error)
	require.NoError(t, db.Create(&models.Position{ID: "pos-manager", Name: "Руководитель"}).Error)
	require.NoError(t, db.Create(&models.Team{ID: "team-search", Name: "Поиск", Description: "Поисковый сервис"}).Error)
	require.NoError(t, db.Create(&models.Mentor{ID: "mentor-1", TeamID: "team-search", FirstName: "Катя", SecondName: "Волкова", Contact: "katya@example.test"}).Error)
	require.NoError(t, db.Create(&models.Employee{ID: "intern-1", Email: "intern@example.test", FirstName: "Иван", SecondName: "Петров", PositionID: "pos-intern", TeamID: "team-search", Role: models.RoleIntern}).Error)
	require.NoError(t, db.Create(&models.Employee{ID: "manager-1", Email: "manager@example.test", FirstName: "Дмитрий", SecondName: "Орлов", PositionID: "pos-manager", TeamID: "team-search", Role: models.RoleManager}).Error)
	mentorID := "mentor-1"
	require.NoError(t, db.Create(&models.Profile{ID: "profile-1", EmployeeID: "intern-1", MentorID: &mentorID}).Error)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestGetPlanUsesCurrentAccessAndRequestState(t *testing.T) {
	db := newPlanTestDB(t)
	accesses := accessservice.WorkingAccesses{
		Resources: []accessservice.RequiredResource{
			{ID: "wiki", Name: "Командная вики", Status: accessservice.StatusGranted},
			{ID: "gitlab", Name: "GitLab", Status: accessservice.StatusPending, CanRequest: false, Request: &accessservice.RequestSummary{ID: "request-1"}},
		},
		Progress: accessservice.Progress{Completed: 1, Total: 2, InProgress: 1},
	}
	requests := accessrequest.RequestList{Counts: accessrequest.RequestCounts{Pending: 2, Issuing: 1, Granted: 1, Rejected: 1, Failed: 1}}
	service := NewServiceWithReaders(db, accessReaderStub{result: accesses}, requestReaderStub{result: requests})

	plan, err := service.GetPlan(context.Background(), "intern-1")
	require.NoError(t, err)
	require.Len(t, plan.Tasks, 3)
	require.Equal(t, PlanProgress{Completed: 0, Total: 3}, plan.Progress)
	require.Equal(t, Progress{Completed: 1, Total: 2, InProgress: 1}, plan.AccessProgress)
	require.Equal(t, "in_progress", plan.Tasks[1].Status)
	require.Equal(t, "team-intro", plan.NextTaskID)
	require.Equal(t, AccessRequestSummary{Pending: 2, InProgress: 3, Processed: 3}, plan.AccessRequestSummary)
	require.NotNil(t, plan.UpcomingMeeting)
	require.Equal(t, "Катя Волкова", plan.UpcomingMeeting.MentorName)

	details, err := service.GetTaskDetails(context.Background(), "intern-1", "working-accesses")
	require.NoError(t, err)
	require.Equal(t, "request-1", details.WorkingAccesses.Resources[1].RequestID)

	firstTask, err := service.GetTaskDetails(context.Background(), "intern-1", "first-task")
	require.NoError(t, err)
	require.Equal(t, "locked", firstTask.Status)
	require.False(t, firstTask.ChatEnabled)
	_, err = service.GetTaskDetails(context.Background(), "intern-1", "missing")
	require.ErrorIs(t, err, ErrInvalidTask)
}

func TestGetPlanUnlocksFirstTaskOnlyAfterIntroAndAllAccesses(t *testing.T) {
	db := newPlanTestDB(t)
	require.NoError(t, db.Create(&models.TeamIntroduction{EmployeeID: "intern-1", CompletedAt: time.Now().UTC()}).Error)
	accesses := accessservice.WorkingAccesses{
		Resources: []accessservice.RequiredResource{{ID: "wiki", Name: "Командная вики", Status: accessservice.StatusGranted}},
		Progress:  accessservice.Progress{Completed: 1, Total: 1},
	}
	service := NewServiceWithReaders(db, accessReaderStub{result: accesses}, requestReaderStub{})

	plan, err := service.GetPlan(context.Background(), "intern-1")
	require.NoError(t, err)
	require.Equal(t, PlanProgress{Completed: 2, Total: 3}, plan.Progress)
	require.Equal(t, "coming_soon", plan.Tasks[2].Status)
	require.Equal(t, "first-task", plan.NextTaskID)
}

func TestCompleteTeamIntroPersistsOnlyOneCompletion(t *testing.T) {
	db := newPlanTestDB(t)
	service := NewService(db)
	ctx := context.Background()

	require.NoError(t, service.CompleteTeamIntro(ctx, "intern-1"))
	require.NoError(t, service.CompleteTeamIntro(ctx, "intern-1"))
	var count int64
	require.NoError(t, db.Model(&models.TeamIntroduction{}).Where("employee_id = ?", "intern-1").Count(&count).Error)
	require.EqualValues(t, 1, count)
	intro, err := service.GetTeamIntro(ctx, "intern-1")
	require.NoError(t, err)
	require.Equal(t, "completed", intro.Status)
}

func TestOnboardingMapsAuthorizationErrorsFromAccessServices(t *testing.T) {
	db := newPlanTestDB(t)
	ctx := context.Background()
	for _, test := range []struct {
		name string
		err  error
		want error
	}{
		{name: "employee missing", err: auth.ErrEmployeeNotFound, want: ErrInternNotFound},
		{name: "forbidden", err: auth.ErrForbidden, want: ErrForbidden},
		{name: "profile missing", err: auth.ErrProfileRequired, want: ErrProfileNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := NewServiceWithReaders(db, accessReaderStub{err: test.err}, requestReaderStub{})
			_, err := service.GetPlan(ctx, "intern-1")
			require.ErrorIs(t, err, test.want)
			service = NewServiceWithReaders(db, accessReaderStub{}, requestReaderStub{err: test.err})
			_, err = service.GetPlan(ctx, "intern-1")
			require.ErrorIs(t, err, test.want)
		})
	}
	service := NewServiceWithReaders(db, accessReaderStub{}, requestReaderStub{err: errors.New("database failure")})
	_, err := service.GetPlan(ctx, "intern-1")
	require.Error(t, err)
}

func TestGetTeamIntroReportsMissingPeopleAndForbiddenRole(t *testing.T) {
	db := newPlanTestDB(t)
	service := NewService(db)
	_, err := service.GetTeamIntro(context.Background(), "missing")
	require.ErrorIs(t, err, ErrInternNotFound)
	_, err = service.GetTeamIntro(context.Background(), "manager-1")
	require.ErrorIs(t, err, ErrForbidden)
	require.NoError(t, db.Delete(&models.Employee{}, "id = ?", "manager-1").Error)
	_, err = service.GetTeamIntro(context.Background(), "intern-1")
	require.ErrorIs(t, err, ErrManagerNotFound)
}

func TestCompleteTeamIntroRejectsMissingEmployeeAndRole(t *testing.T) {
	db := newPlanTestDB(t)
	service := NewService(db)
	ctx := context.Background()
	require.ErrorIs(t, service.CompleteTeamIntro(ctx, "missing"), ErrInternNotFound)
	require.ErrorIs(t, service.CompleteTeamIntro(ctx, "manager-1"), ErrForbidden)
	require.NoError(t, db.Create(&models.Employee{ID: "intern-no-profile", Role: models.RoleIntern}).Error)
	require.ErrorIs(t, service.CompleteTeamIntro(ctx, "intern-no-profile"), ErrProfileNotFound)
}
