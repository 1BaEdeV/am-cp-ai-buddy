package accessrequest_test

import (
	"context"
	"testing"

	accessservice "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/access"
	requestservice "github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/accessrequest"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/auth"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPrepareAndConfirmAccessRequest(t *testing.T) {
	s, db := requestService(t)
	draft := preparedDraft(t, s, " gitlab ")
	require.Equal(t, "Developer", draft.Parameters.AccessLevel)
	require.Equal(t, "Дмитрий Орлов", draft.Parameters.ApproverName)
	require.Equal(t, draft.CreatedAt.Add(requestservice.DraftTTL), draft.ExpiresAt)
	var count int64
	require.NoError(t, db.Model(&models.AccessRequest{}).Count(&count).Error)
	require.Zero(t, count, "подготовка не создаёт заявку")

	request, created, err := s.ConfirmAccessRequest(context.Background(), "intern", draft.ID)
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, draft.Parameters, request.Parameters)
	require.Equal(t, draft.ID, request.DraftID)
	var stored models.AccessDraft
	require.NoError(t, db.Take(&stored, "id = ?", draft.ID).Error)
	require.NotNil(t, stored.ConfirmedAt)
	require.True(t, stored.ConfirmedAt.Equal(request.CreatedAt))

	// Повтор не применяет новые правила к уже отправленной заявке.
	expireDraft(t, db, draft.ID)
	require.NoError(t, db.Model(&models.Resource{}).Where("id = ?", "gitlab").Update("name", "Новое имя").Error)
	again, created, err := s.ConfirmAccessRequest(context.Background(), "intern", draft.ID)
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, request.ID, again.ID)
	require.Equal(t, request.Parameters, again.Parameters)
	var jobs int64
	require.NoError(t, db.Model(&models.AccessIssuanceJob{}).Count(&jobs).Error)
	require.Zero(t, jobs, "до решения руководителя задания на выдачу нет")
}

func TestPrepareAccessRequestChecksEligibility(t *testing.T) {
	cases := []struct {
		name, employeeID, resourceID string
		change                       func(*testing.T, *gorm.DB)
		want                         error
	}{
		{name: "сотрудник отсутствует", employeeID: "missing", resourceID: "gitlab", want: auth.ErrEmployeeNotFound},
		{name: "пустой сотрудник", resourceID: "gitlab", want: auth.ErrEmployeeNotFound},
		{name: "руководитель не просит доступ", employeeID: "manager", resourceID: "gitlab", want: auth.ErrForbidden},
		{name: "нет профиля", employeeID: "other-intern", resourceID: "gitlab", want: auth.ErrProfileRequired},
		{name: "ресурс не нужен", employeeID: "intern", resourceID: "unknown", want: requestservice.ErrResourceNotRequired},
		{name: "пустой ресурс", employeeID: "intern", want: requestservice.ErrInvalidInput},
		{name: "недопустимый уровень", employeeID: "intern", resourceID: "gitlab", want: accessservice.ErrInvalidRequirement,
			change: func(t *testing.T, db *gorm.DB) {
				require.NoError(t, db.Model(&models.AccessRequirement{}).Where("id = ?", "gitlab").Update("access_level", "Admin").Error)
			}},
		{name: "согласующий другой команды", employeeID: "intern", resourceID: "gitlab", want: accessservice.ErrInvalidRequirement,
			change: func(t *testing.T, db *gorm.DB) {
				require.NoError(t, db.Model(&models.Employee{}).Where("id = ?", "manager").Update("team_id", "other").Error)
			}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, db := requestService(t)
			if tc.change != nil {
				tc.change(t, db)
			}
			_, err := s.PrepareAccessRequest(context.Background(), tc.employeeID, tc.resourceID)
			require.ErrorIs(t, err, tc.want)
			var count int64
			require.NoError(t, db.Model(&models.AccessDraft{}).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}

func TestPrepareSkipsGrantedAccessAndExistingRequest(t *testing.T) {
	s, db := requestService(t)
	granted, err := s.PrepareAccessRequest(context.Background(), "intern", "wiki")
	require.NoError(t, err)
	require.Equal(t, requestservice.PreparationAlreadyGranted, granted.Status)
	require.Nil(t, granted.Draft)
	request := pendingRequest(t, s, "gitlab")
	existing, err := s.PrepareAccessRequest(context.Background(), "intern", "gitlab")
	require.NoError(t, err)
	require.Equal(t, requestservice.PreparationRequestExists, existing.Status)
	require.Equal(t, request.ID, existing.Request.ID)
	require.Nil(t, existing.Draft)
	var count int64
	require.NoError(t, db.Model(&models.AccessDraft{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
}
