package chat

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newChatServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Employee{}, &models.Profile{}, &models.ChatMessage{}, &models.ChatRun{}))
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func seedChatIntern(t *testing.T, db *gorm.DB, id string, role models.EmployeeRole, withProfile bool) {
	t.Helper()
	require.NoError(t, db.Create(&models.Employee{ID: id, Email: id + "@example.test", Role: role}).Error)
	if withProfile {
		require.NoError(t, db.Create(&models.Profile{ID: "profile-" + id, EmployeeID: id}).Error)
	}
}

func TestSendMessageValidatesAccessAndQueuesIdempotently(t *testing.T) {
	db := newChatServiceTestDB(t)
	seedChatIntern(t, db, "intern-1", models.RoleIntern, true)
	seedChatIntern(t, db, "manager-1", models.RoleManager, true)
	seedChatIntern(t, db, "intern-no-profile", models.RoleIntern, false)
	service := NewService(db, nil, nil, nil)
	ctx := context.Background()

	_, err := service.SendMessage(ctx, "intern-1", "unknown", "key", MessageInput{Text: "Вопрос"})
	require.ErrorIs(t, err, ErrInvalidTask)
	for _, test := range []struct {
		key  string
		text string
	}{
		{key: "key", text: "  "},
		{key: "", text: "Вопрос"},
		{key: "key", text: strings.Repeat("я", 451)},
	} {
		_, err := service.SendMessage(ctx, "intern-1", "team-intro", test.key, MessageInput{Text: test.text})
		require.ErrorIs(t, err, ErrInvalidMessage)
	}
	_, err = service.SendMessage(ctx, "missing", "team-intro", "key", MessageInput{Text: "Вопрос"})
	require.ErrorIs(t, err, ErrEmployeeNotFound)
	_, err = service.SendMessage(ctx, "manager-1", "team-intro", "key", MessageInput{Text: "Вопрос"})
	require.ErrorIs(t, err, ErrForbidden)
	_, err = service.SendMessage(ctx, "intern-no-profile", "team-intro", "key", MessageInput{Text: "Вопрос"})
	require.ErrorIs(t, err, ErrProfileRequired)

	first, err := service.SendMessage(ctx, "intern-1", "team-intro", " key-1 ", MessageInput{Text: "Кто наставник?"})
	require.NoError(t, err)
	require.Equal(t, "queued", first.Status)
	require.NotEmpty(t, first.ID)

	repeated, err := service.SendMessage(ctx, "intern-1", "team-intro", "key-1", MessageInput{Text: "Кто наставник?"})
	require.NoError(t, err)
	require.Equal(t, first, repeated)

	_, err = service.SendMessage(ctx, "intern-1", "team-intro", "key-1", MessageInput{Text: "Другой вопрос"})
	require.ErrorIs(t, err, ErrIdempotencyKeyReused)
	_, err = service.SendMessage(ctx, "intern-1", "team-intro", "key-2", MessageInput{Text: "Другой вопрос"})
	require.ErrorIs(t, err, ErrChatBusy)

	require.NoError(t, db.Model(&models.ChatRun{}).Where("id = ?", first.ID).Update("status", "completed").Error)
	second, err := service.SendMessage(ctx, "intern-1", "team-intro", "key-2", MessageInput{Text: "Другой вопрос"})
	require.NoError(t, err)
	require.Equal(t, "queued", second.Status)
	require.NotEqual(t, first.ID, second.ID)

	chat, err := service.GetTaskChat(ctx, "intern-1", "team-intro")
	require.NoError(t, err)
	require.Len(t, chat.Messages, 2)
	require.NotNil(t, chat.LatestRun)
	require.Equal(t, second.ID, chat.LatestRun.ID)
}
