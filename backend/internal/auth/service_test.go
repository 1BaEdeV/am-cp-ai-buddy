package auth

import (
	"testing"
	"time"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestNewService(t *testing.T) {
	db := &gorm.DB{}
	s := NewService(db)
	require.NotNil(t, s)
	assert.Same(t, db, s.db)
}

func TestRegistrationDate(t *testing.T) {
	t.Run("year one is a valid date", func(t *testing.T) {
		d, err := registrationDate(models.RoleIntern, RegistrationInput{
			MentorID: "m", InternshipStartDate: "0001-01-01",
		})
		require.NoError(t, err)
		require.NotNil(t, d)
		assert.Equal(t, "0001-01-01", d.Format(time.DateOnly))
	})

	t.Run("intern valid", func(t *testing.T) {
		d, err := registrationDate(models.RoleIntern, RegistrationInput{
			MentorID:            "m",
			InternshipStartDate: "2024-01-15",
		})
		require.NoError(t, err)
		require.NotNil(t, d)
		assert.Equal(t, "2024-01-15", d.Format(time.DateOnly))
	})

	t.Run("intern no mentor", func(t *testing.T) {
		_, err := registrationDate(models.RoleIntern, RegistrationInput{
			InternshipStartDate: "2024-01-15",
		})
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("intern no date", func(t *testing.T) {
		_, err := registrationDate(models.RoleIntern, RegistrationInput{MentorID: "m"})
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("intern bad date", func(t *testing.T) {
		_, err := registrationDate(models.RoleIntern, RegistrationInput{
			MentorID:            "m",
			InternshipStartDate: "not-a-date",
		})
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("manager clean", func(t *testing.T) {
		d, err := registrationDate(models.RoleManager, RegistrationInput{})
		require.NoError(t, err)
		assert.Nil(t, d)
	})

	t.Run("manager with mentor", func(t *testing.T) {
		_, err := registrationDate(models.RoleManager, RegistrationInput{MentorID: "m"})
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("manager with date", func(t *testing.T) {
		_, err := registrationDate(models.RoleManager, RegistrationInput{
			InternshipStartDate: "2024-01-15",
		})
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("unknown role", func(t *testing.T) {
		_, err := registrationDate("cto", RegistrationInput{})
		assert.ErrorIs(t, err, ErrForbidden)
	})
}

func TestMentorID(t *testing.T) {
	assert.Equal(t, "", mentorID(models.Profile{}))

	id := "m-1"
	assert.Equal(t, "m-1", mentorID(models.Profile{MentorID: &id}))
}

func TestDateString(t *testing.T) {
	assert.Equal(t, "", dateString(nil))

	d := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, "2024-01-15", dateString(&d))
}
