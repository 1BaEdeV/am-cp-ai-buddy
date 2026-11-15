package accessrequest_test

import (
	"context"
	"errors"
	"testing"
	"time"

	requestservice "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/accessrequest"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/auth"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestConfirmAccessRequestRevalidatesDraft(t *testing.T) {
	cases := []struct {
		name   string
		change func(*testing.T, *requestservice.Service, *gorm.DB, requestservice.Draft)
		want   error
	}{
		{"истёк срок", func(t *testing.T, s *requestservice.Service, db *gorm.DB, d requestservice.Draft) {
			expireDraft(t, db, d.ID)
		}, requestservice.ErrDraftExpired},
		{"изменился уровень", func(t *testing.T, s *requestservice.Service, db *gorm.DB, d requestservice.Draft) {
			require.NoError(t, db.Model(&models.AccessRequirement{}).Where("id = ?", "gitlab").Update("access_level", "Reporter").Error)
		}, requestservice.ErrDraftChanged},
		{"изменился внешний ресурс", func(t *testing.T, s *requestservice.Service, db *gorm.DB, d requestservice.Draft) {
			require.NoError(t, db.Model(&models.Resource{}).Where("id = ?", "gitlab").Update("external_id", "another/repo").Error)
		}, requestservice.ErrDraftChanged},
		{"изменилось имя согласующего", func(t *testing.T, s *requestservice.Service, db *gorm.DB, d requestservice.Draft) {
			require.NoError(t, db.Model(&models.Employee{}).Where("id = ?", "manager").Update("first_name", "Другое имя").Error)
		}, requestservice.ErrDraftChanged},
		{"изменилась должность", func(t *testing.T, s *requestservice.Service, db *gorm.DB, d requestservice.Draft) {
			require.NoError(t, db.Model(&models.Employee{}).Where("id = ?", "intern").Update("position_id", "qa").Error)
		}, requestservice.ErrDraftChanged},
		{"правило удалено", func(t *testing.T, s *requestservice.Service, db *gorm.DB, d requestservice.Draft) {
			require.NoError(t, db.Delete(&models.AccessRequirement{}, "id = ?", "gitlab").Error)
		}, requestservice.ErrDraftChanged},
		{"доступ появился", func(t *testing.T, s *requestservice.Service, db *gorm.DB, d requestservice.Draft) {
			require.NoError(t, db.Create(&models.EmployeeAccess{ID: "new", EmployeeID: "intern", ResourceID: "gitlab", AccessLevel: "Developer", VerifiedAt: time.Now().UTC()}).Error)
		}, requestservice.ErrAccessAlreadyGranted},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, db := requestService(t)
			draft := preparedDraft(t, s, "gitlab")
			tc.change(t, s, db, draft)
			_, created, err := s.ConfirmAccessRequest(context.Background(), "intern", draft.ID)
			require.ErrorIs(t, err, tc.want)
			require.False(t, created)
			var count int64
			require.NoError(t, db.Model(&models.AccessRequest{}).Count(&count).Error)
			require.Zero(t, count)
			var stored models.AccessDraft
			require.NoError(t, db.Take(&stored, "id = ?", draft.ID).Error)
			require.Nil(t, stored.ConfirmedAt)
		})
	}
}

func TestConfirmChecksOwnerAndOtherDrafts(t *testing.T) {
	s, _ := requestService(t)
	first := preparedDraft(t, s, "gitlab")
	second := preparedDraft(t, s, "gitlab")
	_, _, err := s.ConfirmAccessRequest(context.Background(), "other-intern", first.ID)
	require.ErrorIs(t, err, requestservice.ErrDraftNotFound)
	_, _, err = s.ConfirmAccessRequest(context.Background(), "manager", first.ID)
	require.ErrorIs(t, err, auth.ErrForbidden)
	_, _, err = s.ConfirmAccessRequest(context.Background(), "intern", "missing")
	require.ErrorIs(t, err, requestservice.ErrDraftNotFound)
	request, _, err := s.ConfirmAccessRequest(context.Background(), "intern", first.ID)
	require.NoError(t, err)
	_, _, err = s.ConfirmAccessRequest(context.Background(), "intern", second.ID)
	require.ErrorIs(t, err, requestservice.ErrActiveRequestExists)
	_, err = s.DecideAccessRequest(context.Background(), "manager", request.ID, requestservice.DecisionInput{Decision: requestservice.DecisionReject, Reason: "Не нужен сейчас"})
	require.NoError(t, err)
	next, created, err := s.ConfirmAccessRequest(context.Background(), "intern", second.ID)
	require.NoError(t, err)
	require.True(t, created)
	require.NotEqual(t, request.ID, next.ID)
}

func TestConfirmationIsAtomic(t *testing.T) {
	s, db := requestService(t)
	draft := preparedDraft(t, s, "gitlab")
	failure := errors.New("ошибка записи подтверждения")
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:fail_confirmation", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "access_drafts" {
			require.ErrorIs(t, tx.AddError(failure), failure)
		}
	}))
	_, _, err := s.ConfirmAccessRequest(context.Background(), "intern", draft.ID)
	require.ErrorIs(t, err, failure)
	var count int64
	require.NoError(t, db.Model(&models.AccessRequest{}).Count(&count).Error)
	require.Zero(t, count, "заявка откатывается вместе с подтверждением")
}
