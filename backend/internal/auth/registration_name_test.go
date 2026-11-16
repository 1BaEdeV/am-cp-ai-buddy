package auth

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRegistrationName(t *testing.T) {
	for _, tt := range []struct {
		value string
		want  employeeName
	}{
		{"  Сидоров  Алексей\tСергеевич  ", employeeName{"Алексей", "Сидоров", "Сергеевич"}},
		{"Петрова Анна", employeeName{"Анна", "Петрова", ""}},
		{"Иванова-Петрова Анна-Мария Сергеевна", employeeName{"Анна-Мария", "Иванова-Петрова", "Сергеевна"}},
		{"O’Connor Sean", employeeName{"Sean", "O’Connor", ""}},
		{"O'Connor Jose\u0301", employeeName{"Jose\u0301", "O'Connor", ""}},
		{"OʼConnor Sean", employeeName{"Sean", "OʼConnor", ""}},
	} {
		t.Run(tt.value, func(t *testing.T) {
			got, err := registrationName(&tt.value, models.Employee{})
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
	for _, value := range []string{"", "  ", "Алексей", "Иванов Иван Иванович Ещё", "Иванов Иван123", "--- Алексей", "Иванов <Иван>", strings.Repeat("Я", 150) + " Иван"} {
		t.Run("invalid/"+value, func(t *testing.T) {
			_, err := registrationName(&value, models.Employee{})
			require.ErrorIs(t, err, ErrInvalidInput)
		})
	}
	legacy, err := registrationName(nil, models.Employee{FirstName: "Иван", SecondName: "Петров", MiddleName: "Олегович"})
	require.NoError(t, err)
	require.Equal(t, employeeName{"Иван", "Петров", "Олегович"}, legacy)
}

func TestRegisterFullName(t *testing.T) {
	t.Run("saves name and preserves it on repeated registration", func(t *testing.T) {
		s, f := newService(t)
		value := " Сидоров  Алексей Сергеевич "
		input := RegistrationInput{FullName: &value, MentorID: f.Mentor2, InternshipStartDate: "2026-11-16"}
		profile, created, err := s.Register(context.Background(), f.Intern, input)
		require.NoError(t, err)
		require.True(t, created)
		require.Equal(t, "Алексей", profile.FirstName)
		require.Equal(t, "Сидоров", profile.SecondName)
		require.Equal(t, "Сергеевич", profile.MiddleName)
		me, err := s.Me(context.Background(), f.Intern)
		require.NoError(t, err)
		require.Equal(t, profile, me)
		value = "Сидоров Алексей Сергеевич"
		repeated, created, err := s.Register(context.Background(), f.Intern, input)
		require.NoError(t, err)
		require.False(t, created)
		require.Equal(t, profile, repeated)
		value = "Петров Иван"
		_, _, err = s.Register(context.Background(), f.Intern, input)
		require.ErrorIs(t, err, ErrProfileAlreadyExists)
		me, err = s.Me(context.Background(), f.Intern)
		require.NoError(t, err)
		require.Equal(t, profile, me)
	})
	t.Run("invalid name leaves employee unchanged", func(t *testing.T) {
		s, f := newService(t)
		var before models.Employee
		require.NoError(t, s.db.Take(&before, "id = ?", f.Intern).Error)
		value := "Алексей123"
		_, _, err := s.Register(context.Background(), f.Intern, RegistrationInput{
			FullName: &value, MentorID: f.Mentor2, InternshipStartDate: "2026-11-16",
		})
		require.ErrorIs(t, err, ErrInvalidInput)
		var after models.Employee
		require.NoError(t, s.db.Take(&after, "id = ?", f.Intern).Error)
		require.Equal(t, before, after)
	})
	t.Run("failed profile creation rolls back name update", func(t *testing.T) {
		s, f := newService(t)
		var before models.Employee
		require.NoError(t, s.db.Take(&before, "id = ?", f.Intern).Error)
		failure := errors.New("profile write failed")
		require.NoError(t, s.db.Callback().Create().Before("gorm:create").Register("test:reject_profile", func(tx *gorm.DB) {
			if tx.Statement.Table == "profiles" {
				tx.AddError(failure)
			}
		}))
		value := "Сидоров Алексей Сергеевич"
		_, _, err := s.Register(context.Background(), f.Intern, RegistrationInput{
			FullName: &value, MentorID: f.Mentor2, InternshipStartDate: "2026-11-16",
		})
		require.ErrorIs(t, err, failure)
		var after models.Employee
		require.NoError(t, s.db.Take(&after, "id = ?", f.Intern).Error)
		require.Equal(t, before, after)
	})
}
