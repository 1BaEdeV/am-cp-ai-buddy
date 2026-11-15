package demoseed

import (
	"context"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	demodata "github.com/1BaEdeV/am-cp-ai-buddy/backend/seeds/demo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validData(t *testing.T) Data {
	t.Helper()
	d, err := Read(demodata.Files)
	require.NoError(t, err)
	return d
}

func copyFiles(t *testing.T) fstest.MapFS {
	t.Helper()
	files := fstest.MapFS{}
	entries, err := fs.ReadDir(demodata.Files, ".")
	require.NoError(t, err)
	for _, entry := range entries {
		content, err := fs.ReadFile(demodata.Files, entry.Name())
		require.NoError(t, err)
		files[entry.Name()] = &fstest.MapFile{Data: content}
	}
	return files
}

func TestReadEmbeddedDemo(t *testing.T) {
	d := validData(t)
	assert.Len(t, d.Teams, 10)
	assert.Len(t, d.Positions, 10)
	assert.Len(t, d.Mentors, 10)
	assert.Len(t, d.Employees, 20)
	assert.Len(t, d.Profiles, 11)
	assert.Len(t, d.Resources, 10)
	assert.Len(t, d.Accesses, 2)
	assert.Equal(t, "2026-11-10", d.Profiles[10].InternshipStartDate.Format(time.DateOnly))
	assert.Equal(t, "2026-11-12T06:00:00Z", d.Accesses[0].VerifiedAt.UTC().Format(time.RFC3339))
}

func TestReadRejectsMissingAndMalformedFiles(t *testing.T) {
	for _, file := range []string{"teams", "positions", "mentors", "employees", "profiles", "resources", "access_requirements", "employee_accesses"} {
		t.Run("missing/"+file, func(t *testing.T) {
			files := copyFiles(t)
			delete(files, file+".yaml")
			_, err := Read(files)
			require.ErrorIs(t, err, fs.ErrNotExist)
			require.ErrorContains(t, err, file)
		})
	}
	for _, tt := range []struct{ name, file, content string }{
		{"unknown nested field", "teams", "teams: [{id: a, name: b, arbitrary: true}]"},
		{"wrong root", "teams", "team: []"},
		{"extra root", "teams", "teams: []\nextra: []"},
		{"null list", "teams", "teams: null"},
		{"invalid yaml", "teams", "teams: ["},
		{"second document", "teams", "teams: []\n---\nnull"},
		{"duplicate key", "teams", "teams: []\nteams: []"},
		{"invalid profile date", "profiles", "profiles: [{id: p, employee_id: intern-alina, internship_start_date: tomorrow}]"},
		{"invalid verification time", "employee_accesses", "employee_accesses: [{id: a, verified_at: yesterday}]"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			files := copyFiles(t)
			files[tt.file+".yaml"] = &fstest.MapFile{Data: []byte(tt.content)}
			_, err := Read(files)
			require.ErrorIs(t, err, ErrInvalidData)
		})
	}
}

func TestValidateRejectsInvalidReferencesAndDuplicates(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(*Data)
	}{
		{"empty data", func(d *Data) { *d = Data{} }},
		{"blank id", func(d *Data) { d.Teams[0].ID = " " }},
		{"padded id", func(d *Data) { d.Teams[0].ID = " team " }},
		{"duplicate team", func(d *Data) { d.Teams = append(d.Teams, d.Teams[0]) }},
		{"duplicate position", func(d *Data) { d.Positions = append(d.Positions, d.Positions[0]) }},
		{"duplicate mentor", func(d *Data) { d.Mentors = append(d.Mentors, d.Mentors[0]) }},
		{"duplicate employee", func(d *Data) { d.Employees = append(d.Employees, d.Employees[0]) }},
		{"duplicate profile", func(d *Data) { d.Profiles = append(d.Profiles, d.Profiles[0]) }},
		{"duplicate resource", func(d *Data) { d.Resources = append(d.Resources, d.Resources[0]) }},
		{"duplicate requirement", func(d *Data) { d.Requirements = append(d.Requirements, d.Requirements[0]) }},
		{"duplicate access", func(d *Data) { d.Accesses = append(d.Accesses, d.Accesses[0]) }},
		{"blank team name", func(d *Data) { d.Teams[0].Name = " " }},
		{"blank position name", func(d *Data) { d.Positions[0].Name = "" }},
		{"unknown mentor team", func(d *Data) { d.Mentors[0].TeamID = "unknown" }},
		{"unknown employee team", func(d *Data) { d.Employees[0].TeamID = "unknown" }},
		{"unknown employee position", func(d *Data) { d.Employees[0].PositionID = "unknown" }},
		{"unknown role", func(d *Data) { d.Employees[0].Role = "admin" }},
		{"blank employee name", func(d *Data) { d.Employees[0].FirstName = " " }},
		{"duplicate email ignoring case", func(d *Data) { d.Employees[1].Email = strings.ToUpper(d.Employees[0].Email) }},
		{"unknown profile owner", func(d *Data) { d.Profiles[0].EmployeeID = "unknown" }},
		{"duplicate profile owner", func(d *Data) { d.Profiles[1].EmployeeID = d.Profiles[0].EmployeeID }},
		{"manager with mentor", func(d *Data) { id := d.Mentors[0].ID; d.Profiles[0].MentorID = &id }},
		{"intern without mentor", func(d *Data) { d.Profiles[10].MentorID = nil }},
		{"intern with unknown mentor", func(d *Data) { id := "unknown"; d.Profiles[10].MentorID = &id }},
		{"intern with foreign mentor", func(d *Data) { id := d.Mentors[1].ID; d.Profiles[10].MentorID = &id }},
		{"intern without date", func(d *Data) { d.Profiles[10].InternshipStartDate = nil }},
		{"blank resource", func(d *Data) { d.Resources[0].Service = "" }},
		{"no access levels", func(d *Data) { d.Resources[0].AccessLevels = nil }},
		{"duplicate external resource", func(d *Data) {
			d.Resources[1].Service, d.Resources[1].ExternalID = d.Resources[0].Service, d.Resources[0].ExternalID
		}},
		{"blank level", func(d *Data) { d.Resources[0].AccessLevels = []string{" "} }},
		{"duplicate level", func(d *Data) { d.Resources[0].AccessLevels = []string{"read", "read"} }},
		{"unknown requirement team", func(d *Data) { d.Requirements[0].TeamID = "unknown" }},
		{"unknown requirement position", func(d *Data) { d.Requirements[0].PositionID = "unknown" }},
		{"unknown requirement resource", func(d *Data) { d.Requirements[0].ResourceID = "unknown" }},
		{"unknown approver", func(d *Data) { d.Requirements[0].ApproverID = "unknown" }},
		{"foreign approver", func(d *Data) { d.Requirements[0].ApproverID = "manager-anna" }},
		{"unsupported requirement level", func(d *Data) { d.Requirements[0].AccessLevel = "Owner" }},
		{"duplicate rule key", func(d *Data) { r := d.Requirements[0]; r.ID = "other-rule"; d.Requirements = append(d.Requirements, r) }},
		{"unknown access employee", func(d *Data) { d.Accesses[0].EmployeeID = "unknown" }},
		{"unknown access resource", func(d *Data) { d.Accesses[0].ResourceID = "unknown" }},
		{"unsupported initial level", func(d *Data) { d.Accesses[0].AccessLevel = "Owner" }},
		{"unverified initial rights", func(d *Data) { d.Accesses[0].VerifiedAt = time.Time{} }},
		{"duplicate initial rights", func(d *Data) { r := d.Accesses[0]; r.ID = "other-access"; d.Accesses = append(d.Accesses, r) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d := validData(t)
			tt.change(&d)
			require.ErrorIs(t, d.Validate(), ErrInvalidData)
		})
	}
}

func TestEarlyErrorsDoNotNeedDatabase(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Load(ctx, nil, nil)
	require.ErrorIs(t, err, context.Canceled)
	_, err = (Data{}).Apply(ctx, nil)
	require.ErrorIs(t, err, context.Canceled)
	_, err = Load(context.Background(), nil, fstest.MapFS{})
	require.ErrorIs(t, err, fs.ErrNotExist)
	_, err = (Data{}).Apply(context.Background(), nil)
	require.ErrorIs(t, err, ErrInvalidData)
	_, err = validData(t).Apply(context.Background(), nil)
	require.ErrorIs(t, err, ErrInvalidData)
}
