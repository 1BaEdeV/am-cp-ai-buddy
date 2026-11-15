package demoseed

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/accessrequest"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/demoprovider"
	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type positionRow struct {
	ID   string `yaml:"id"`
	Name string `yaml:"name"`
}
type teamRow struct {
	ID          string `yaml:"id"`
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}
type mentorRow struct {
	ID         string `yaml:"id"`
	TeamID     string `yaml:"team_id"`
	FirstName  string `yaml:"first_name"`
	SecondName string `yaml:"second_name"`
	Contact    string `yaml:"contact"`
}
type employeeRow struct {
	ID         string `yaml:"id"`
	Email      string `yaml:"email"`
	FirstName  string `yaml:"first_name"`
	SecondName string `yaml:"second_name"`
	PositionID string `yaml:"position_id"`
	TeamID     string `yaml:"team_id"`
	Role       string `yaml:"role"`
}
type profileRow struct {
	ID                  string  `yaml:"id"`
	EmployeeID          string  `yaml:"employee_id"`
	MentorID            *string `yaml:"mentor_id"`
	InternshipStartDate *string `yaml:"internship_start_date"`
}
type resourceRow struct {
	ID           string   `yaml:"id"`
	Name         string   `yaml:"name"`
	Service      string   `yaml:"service"`
	ExternalID   string   `yaml:"external_id"`
	AccessLevels []string `yaml:"access_levels"`
}
type requirementRow struct {
	ID          string `yaml:"id"`
	TeamID      string `yaml:"team_id"`
	PositionID  string `yaml:"position_id"`
	ResourceID  string `yaml:"resource_id"`
	AccessLevel string `yaml:"access_level"`
	Purpose     string `yaml:"purpose"`
	ApproverID  string `yaml:"approver_id"`
}
type employeeAccessRow struct {
	ID          string `yaml:"id"`
	EmployeeID  string `yaml:"employee_id"`
	ResourceID  string `yaml:"resource_id"`
	AccessLevel string `yaml:"access_level"`
	VerifiedAt  string `yaml:"verified_at"`
}

type fileRows[T any] struct {
	Rows []T `yaml:"-"`
}

// Seed loads demo YAML files in dependency order. Re-running it updates only
// rows with the same fixture IDs and safely leaves unrelated rows untouched.
func Seed(ctx context.Context, database *gorm.DB, directory string) error {
	var positions struct {
		Rows []positionRow `yaml:"positions"`
	}
	var teams struct {
		Rows []teamRow `yaml:"teams"`
	}
	var mentors struct {
		Rows []mentorRow `yaml:"mentors"`
	}
	var employees struct {
		Rows []employeeRow `yaml:"employees"`
	}
	var profiles struct {
		Rows []profileRow `yaml:"profiles"`
	}
	var resources struct {
		Rows []resourceRow `yaml:"resources"`
	}
	var requirements struct {
		Rows []requirementRow `yaml:"access_requirements"`
	}
	var employeeAccesses struct {
		Rows []employeeAccessRow `yaml:"employee_accesses"`
	}
	files := []struct {
		name string
		out  any
	}{
		{"positions.yaml", &positions}, {"teams.yaml", &teams}, {"mentors.yaml", &mentors},
		{"employees.yaml", &employees}, {"profiles.yaml", &profiles}, {"resources.yaml", &resources},
		{"access_requirements.yaml", &requirements}, {"employee_accesses.yaml", &employeeAccesses},
	}
	for _, file := range files {
		path := filepath.Join(directory, file.name)
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read demo fixture %s: %w", file.name, err)
		}
		if err := yaml.Unmarshal(data, file.out); err != nil {
			return fmt.Errorf("parse demo fixture %s: %w", file.name, err)
		}
	}

	var dbResources []models.Resource
	var grants []models.EmployeeAccess
	err := database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := upsert(tx, toModels(positions.Rows, func(r positionRow) models.Position { return models.Position{ID: r.ID, Name: r.Name} })); err != nil {
			return fmt.Errorf("seed positions: %w", err)
		}
		if err := upsert(tx, toModels(teams.Rows, func(r teamRow) models.Team { return models.Team{ID: r.ID, Name: r.Name, Description: r.Description} })); err != nil {
			return fmt.Errorf("seed teams: %w", err)
		}
		if err := upsert(tx, toModels(mentors.Rows, func(r mentorRow) models.Mentor {
			return models.Mentor{ID: r.ID, TeamID: r.TeamID, FirstName: r.FirstName, SecondName: r.SecondName, Contact: r.Contact}
		})); err != nil {
			return fmt.Errorf("seed mentors: %w", err)
		}
		if err := upsert(tx, toModels(employees.Rows, func(r employeeRow) models.Employee {
			return models.Employee{ID: r.ID, Email: r.Email, FirstName: r.FirstName, SecondName: r.SecondName, PositionID: r.PositionID, TeamID: r.TeamID, Role: models.EmployeeRole(r.Role)}
		})); err != nil {
			return fmt.Errorf("seed employees: %w", err)
		}
		profileModels := make([]models.Profile, 0, len(profiles.Rows))
		for _, row := range profiles.Rows {
			profile := models.Profile{ID: row.ID, EmployeeID: row.EmployeeID, MentorID: row.MentorID}
			if row.InternshipStartDate != nil && *row.InternshipStartDate != "" {
				date, err := time.Parse("2006-01-02", *row.InternshipStartDate)
				if err != nil {
					return fmt.Errorf("parse profile %s internship_start_date: %w", row.ID, err)
				}
				profile.InternshipStartDate = &date
			}
			profileModels = append(profileModels, profile)
		}
		if err := upsert(tx, profileModels); err != nil {
			return fmt.Errorf("seed profiles: %w", err)
		}
		resourceModels := toModels(resources.Rows, func(r resourceRow) models.Resource {
			return models.Resource{ID: r.ID, Name: r.Name, Service: r.Service, ExternalID: r.ExternalID, AccessLevels: r.AccessLevels}
		})
		if err := upsert(tx, resourceModels); err != nil {
			return fmt.Errorf("seed resources: %w", err)
		}
		requirementModels := toModels(requirements.Rows, func(r requirementRow) models.AccessRequirement {
			return models.AccessRequirement{ID: r.ID, TeamID: r.TeamID, PositionID: r.PositionID, ResourceID: r.ResourceID, AccessLevel: r.AccessLevel, Purpose: r.Purpose, ApproverID: r.ApproverID}
		})
		if err := upsert(tx, requirementModels); err != nil {
			return fmt.Errorf("seed access requirements: %w", err)
		}
		grantModels := make([]models.EmployeeAccess, 0, len(employeeAccesses.Rows))
		for _, row := range employeeAccesses.Rows {
			verifiedAt, err := time.Parse(time.RFC3339, row.VerifiedAt)
			if err != nil {
				return fmt.Errorf("parse employee access %s verified_at: %w", row.ID, err)
			}
			grantModels = append(grantModels, models.EmployeeAccess{ID: row.ID, EmployeeID: row.EmployeeID, ResourceID: row.ResourceID, AccessLevel: row.AccessLevel, VerifiedAt: verifiedAt})
		}
		if err := upsert(tx, grantModels); err != nil {
			return fmt.Errorf("seed employee accesses: %w", err)
		}
		dbResources, grants = resourceModels, grantModels
		return nil
	})
	if err != nil {
		return err
	}

	resourceByID := make(map[string]models.Resource, len(dbResources))
	for _, resource := range dbResources {
		resourceByID[resource.ID] = resource
	}
	provider := demoprovider.NewProvider(database)
	for _, grant := range grants {
		resource, ok := resourceByID[grant.ResourceID]
		if !ok {
			return fmt.Errorf("seed grant %s references missing resource %s", grant.ID, grant.ResourceID)
		}
		if err := provider.SeedAccess(ctx, accessrequest.GrantTarget{
			EmployeeID: grant.EmployeeID, Service: resource.Service, ExternalID: resource.ExternalID, AccessLevel: grant.AccessLevel,
		}); err != nil {
			return fmt.Errorf("seed simulated provider access %s: %w", grant.ID, err)
		}
	}
	return nil
}

func upsert[T any](tx *gorm.DB, rows []T) error {
	if len(rows) == 0 {
		return nil
	}
	return tx.Omit(clause.Associations).Clauses(clause.OnConflict{UpdateAll: true}).Create(&rows).Error
}

func toModels[Input any, Output any](rows []Input, convert func(Input) Output) []Output {
	result := make([]Output, 0, len(rows))
	for _, row := range rows {
		result = append(result, convert(row))
	}
	return result
}
