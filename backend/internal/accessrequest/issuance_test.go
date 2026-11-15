package accessrequest

import (
	"testing"

	"github.com/1BaEdeV/am-cp-ai-buddy/backend/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestIssuanceResourceMatches(t *testing.T) {
	parameters := models.AccessRequestParameters{
		ResourceName: "Backend", ResourceService: "gitlab", ResourceExternalID: "42", AccessLevel: "Reporter",
	}
	tests := []struct {
		name     string
		resource models.Resource
		want     bool
	}{
		{"same external resource", models.Resource{ID: "repo", Service: "gitlab", ExternalID: "42"}, true},
		{"missing resource", models.Resource{}, false},
		{"missing internal id", models.Resource{Service: "gitlab", ExternalID: "42"}, false},
		{"service changed", models.Resource{ID: "repo", Service: "figma", ExternalID: "42"}, false},
		{"external id changed", models.Resource{ID: "repo", Service: "gitlab", ExternalID: "43"}, false},
		{"missing service", models.Resource{ID: "repo", ExternalID: "42"}, false},
		{"missing external id", models.Resource{ID: "repo", Service: "gitlab"}, false},
		{"renaming does not change destination", models.Resource{ID: "repo", Name: "New name", Service: "gitlab", ExternalID: "42"}, true},
		{"level catalog does not replace approved snapshot", models.Resource{ID: "repo", Service: "gitlab", ExternalID: "42", AccessLevels: []string{"Developer"}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, issuanceResourceMatches(tt.resource, parameters))
		})
	}
}

func TestProviderHasRequiredAccess(t *testing.T) {
	tests := []struct {
		name, service, required string
		actual                  ProviderAccess
		want                    bool
	}{
		{"provider confirmed exact level", "gitlab", "Reporter", ProviderAccess{Granted: true, AccessLevel: "Reporter"}, true},
		{"gitlab higher level", "gitlab", "Reporter", ProviderAccess{Granted: true, AccessLevel: "Developer"}, true},
		{"figma higher level", "figma", "Viewer", ProviderAccess{Granted: true, AccessLevel: "Editor"}, true},
		{"s3 higher level", "s3", "read", ProviderAccess{Granted: true, AccessLevel: "read_write"}, true},
		{"unconfirmed exact level", "gitlab", "Reporter", ProviderAccess{AccessLevel: "Reporter"}, false},
		{"unconfirmed higher level", "gitlab", "Reporter", ProviderAccess{AccessLevel: "Developer"}, false},
		{"confirmed insufficient level", "gitlab", "Developer", ProviderAccess{Granted: true, AccessLevel: "Reporter"}, false},
		{"confirmed without level", "gitlab", "Reporter", ProviderAccess{Granted: true}, false},
		{"empty provider response", "gitlab", "Reporter", ProviderAccess{}, false},
		{"wrong service hierarchy", "figma", "Reporter", ProviderAccess{Granted: true, AccessLevel: "Developer"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := GrantTarget{EmployeeID: "intern", Service: tt.service, ExternalID: "42", AccessLevel: tt.required}
			assert.Equal(t, tt.want, providerHasRequiredAccess(tt.actual, target))
		})
	}
}
