package demoseed

import (
	"context"
	"fmt"
	"io/fs"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"gorm.io/gorm"
)

// RefreshKnowledge явно обновляет только описания и контакты демо-справочников.
// Профили, выбор команды, заявки, доступы и прогресс не меняются.
func RefreshKnowledge(ctx context.Context, db *gorm.DB, files fs.FS) error {
	data, err := Read(files)
	if err != nil {
		return err
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, team := range data.Teams {
			if err := tx.Model(&models.Team{}).Where("id = ?", team.ID).Update("description", team.Description).Error; err != nil {
				return fmt.Errorf("обновить описание команды: %w", err)
			}
		}
		for _, mentor := range data.Mentors {
			if err := tx.Model(&models.Mentor{}).Where("id = ? AND team_id = ?", mentor.ID, mentor.TeamID).
				Updates(map[string]any{"about": mentor.About, "contact": mentor.Contact}).Error; err != nil {
				return fmt.Errorf("обновить сведения о наставнике: %w", err)
			}
		}
		for _, employee := range data.Employees {
			if employee.Role != models.RoleManager {
				continue
			}
			if err := tx.Model(&models.Employee{}).Where("id = ? AND team_id = ? AND role = ?", employee.ID, employee.TeamID, models.RoleManager).
				Update("about", employee.About).Error; err != nil {
				return fmt.Errorf("обновить сведения о руководителе: %w", err)
			}
		}
		return nil
	})
}
