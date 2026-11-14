package github

import (
	"net/http"
	"testing"

	gh "github.com/google/go-github/v92/github"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestEnterpriseAssignmentsPreserveRenamedAccount(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		resource *schema.Resource
		initial  map[string]any
		desired  map[string]any
		id       string
		endpoint string
		response string
	}{
		"team": {
			resource: resourceGithubEnterpriseTeamMembers(),
			initial:  map[string]any{"enterprise_slug": "example", "team_slug": "staff", "members": []any{"old-login"}},
			desired:  map[string]any{"enterprise_slug": "example", "team_slug": "staff", "members": []any{"new-login"}},
			id:       "example:staff",
			endpoint: "/enterprises/example/teams/staff/memberships",
			response: `[{"id":123,"login":"new-login"}]`,
		},
		"cost center": {
			resource: resourceGithubEnterpriseCostCenterResources(),
			initial:  map[string]any{"enterprise_slug": "example", "cost_center_id": "cc-1", "users": []any{"old-login"}},
			desired:  map[string]any{"enterprise_slug": "example", "cost_center_id": "cc-1", "users": []any{"new-login"}},
			id:       "example:cc-1",
			endpoint: "/enterprises/example/settings/billing/cost-centers/cc-1",
			response: `{"id":"cc-1","state":"active","resources":[{"type":"User","name":"new-login"}]}`,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			mux := http.NewServeMux()
			gets := 0
			mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
				if req.Method != http.MethodGet || req.URL.Path != tc.endpoint {
					t.Errorf("rename should not modify assignments: %s %s", req.Method, req.URL.Path)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				gets++
				mustWrite(w, tc.response)
			})
			client, err := gh.NewClient(gh.WithHTTPClient(&http.Client{Transport: localRoundTripper{handler: mux}}))
			if err != nil {
				t.Fatal(err)
			}
			owner := &Owner{v3client: client, maxPerPage: 100}
			d := schema.TestResourceDataRaw(t, tc.resource.Schema, tc.initial)
			d.SetId(tc.id)
			if diags := tc.resource.ReadContext(t.Context(), d, owner); diags.HasError() {
				t.Fatal(diags)
			}
			diff, err := tc.resource.Diff(t.Context(), d.State(), terraform.NewResourceConfigRaw(tc.desired), owner)
			if err != nil {
				t.Fatal(err)
			}
			if diff != nil && !diff.Empty() {
				t.Errorf("rename should have empty plan: %#v", diff)
			}
			// Reconciliation itself also re-reads canonical membership before calculating changes.
			if diags := tc.resource.UpdateContext(t.Context(), d, owner); diags.HasError() {
				t.Fatal(diags)
			}
			if gets < 2 {
				t.Errorf("expected refresh and reconciliation reads, got %d", gets)
			}
		})
	}
}
