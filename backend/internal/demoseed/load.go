package demoseed

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/accessrequest"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/demoprovider"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Counts struct {
	Teams, Positions, Mentors, Employees, Profiles, Resources, Requirements, Accesses int
}

func Load(ctx context.Context, db *gorm.DB, files fs.FS) (Counts, error) {
	if err := ctx.Err(); err != nil {
		return Counts{}, err
	}
	data, err := Read(files)
	if err != nil {
		return Counts{}, err
	}
	return data.Apply(ctx, db)
}

// Apply добавляет отсутствующие записи в одной транзакции. Существующие
// справочники, зарегистрированные профили, заявки и прогресс не перезаписывает.
func (d Data) Apply(ctx context.Context, db *gorm.DB) (Counts, error) {
	if err := ctx.Err(); err != nil {
		return Counts{}, err
	}
	if err := d.Validate(); err != nil {
		return Counts{}, err
	}
	if db == nil {
		return Counts{}, invalid("не задано подключение к БД")
	}
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, batch := range []struct {
			name string
			rows any
			size int
		}{
			{"teams", &d.Teams, len(d.Teams)}, {"positions", &d.Positions, len(d.Positions)},
			{"mentors", &d.Mentors, len(d.Mentors)}, {"employees", &d.Employees, len(d.Employees)},
			{"profiles", &d.Profiles, len(d.Profiles)}, {"resources", &d.Resources, len(d.Resources)},
			{"access_requirements", &d.Requirements, len(d.Requirements)}, {"employee_accesses", &d.Accesses, len(d.Accesses)},
		} {
			if batch.size == 0 {
				continue
			}
			if err := tx.Omit(clause.Associations).Clauses(clause.OnConflict{DoNothing: true}).Create(batch.rows).Error; err != nil {
				return fmt.Errorf("загрузить %s: %w", batch.name, err)
			}
		}
		provider := demoprovider.NewProvider(tx)
		for _, initial := range d.Accesses {
			var saved models.EmployeeAccess
			if err := tx.Take(&saved, "employee_id = ? AND resource_id = ?", initial.EmployeeID, initial.ResourceID).Error; err != nil {
				return fmt.Errorf("прочитать начальный доступ: %w", err)
			}
			if saved.VerifiedAt.IsZero() {
				continue
			}
			var resource models.Resource
			if err := tx.Take(&resource, "id = ?", initial.ResourceID).Error; err != nil {
				return fmt.Errorf("прочитать ресурс начального доступа: %w", err)
			}
			// Не переносим исходные права на другой внешний ресурс после перенастройки справочника.
			for _, original := range d.Resources {
				if original.ID == resource.ID && (original.Service != resource.Service || original.ExternalID != resource.ExternalID) {
					return invalid("ресурс %s изменил внешнее назначение", resource.ID)
				}
			}
			if err := provider.SeedAccess(ctx, accessrequest.GrantTarget{EmployeeID: saved.EmployeeID, Service: resource.Service, ExternalID: resource.ExternalID, AccessLevel: saved.AccessLevel}); err != nil {
				return fmt.Errorf("загрузить права демо-провайдера: %w", err)
			}
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return Counts{}, err
	}
	return Counts{len(d.Teams), len(d.Positions), len(d.Mentors), len(d.Employees), len(d.Profiles), len(d.Resources), len(d.Requirements), len(d.Accesses)}, nil
}
