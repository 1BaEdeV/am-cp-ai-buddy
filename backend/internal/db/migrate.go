package db

import (
	"context"
	"fmt"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"gorm.io/gorm"
)

// Migrate использует один набор таблиц для API и команды загрузки демо.
func Migrate(ctx context.Context, conn *gorm.DB) error {
	if err := conn.WithContext(ctx).AutoMigrate(
		&models.Position{}, &models.Team{}, &models.Mentor{}, &models.Employee{}, &models.Profile{},
		&models.Resource{}, &models.AccessRequirement{}, &models.EmployeeAccess{},
		&models.AccessDraft{}, &models.AccessRequest{}, &models.AccessIssuanceJob{},
		&models.DemoProviderAccess{}, &models.DemoProviderOperation{},
	); err != nil {
		return fmt.Errorf("подготовить таблицы приложения: %w", err)
	}
	return nil
}
