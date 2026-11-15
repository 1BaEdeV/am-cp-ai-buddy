package db

import (
	"context"
	"fmt"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"gorm.io/gorm"
)

// Migrate creates or updates every application table in one place so the API
// and the demo-data command use the same schema.
func Migrate(ctx context.Context, conn *gorm.DB) error {
	if err := conn.WithContext(ctx).AutoMigrate(
		&models.Position{}, &models.Team{}, &models.Mentor{}, &models.Employee{}, &models.Profile{},
		&models.TeamIntroduction{}, &models.ChatMessage{}, &models.ChatRun{},
		&models.Resource{}, &models.AccessRequirement{}, &models.EmployeeAccess{},
		&models.AccessDraft{}, &models.AccessRequest{}, &models.AccessIssuanceJob{},
		&models.DemoProviderAccess{}, &models.DemoProviderOperation{},
	); err != nil {
		return fmt.Errorf("подготовить таблицы приложения: %w", err)
	}
	return nil
}
