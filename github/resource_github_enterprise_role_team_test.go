package github

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestResourceGithubEnterpriseRoleTeamLifecycle(t *testing.T) {
	t.Parallel()

	const (
		enterpriseSlug = "example"
		teamSlug       = "ent:security"
		roleID         = int64(8030)
	)

	assigned := false
	mux := http.NewServeMux()
	mux.HandleFunc("/enterprises/example/enterprise-roles/teams/ent:security/8030", func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodPut:
			assigned = true
			w.WriteHeader(http.StatusNoContent)
		case http.MethodDelete:
			assigned = false
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("request method = %s, want PUT or DELETE", req.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/enterprises/example/enterprise-roles/8030/teams", func(w http.ResponseWriter, req *http.Request) {
		if got := req.URL.Query().Get("per_page"); got != "100" {
			t.Errorf("per_page = %q, want 100", got)
		}

		teams := []map[string]any{}
		if assigned {
			teams = append(teams, map[string]any{"id": 1, "slug": teamSlug})
		}
		if err := json.NewEncoder(w).Encode(teams); err != nil {
			t.Errorf("encode teams response: %v", err)
		}
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	r := resourceGithubEnterpriseRoleTeam()
	d := schema.TestResourceDataRaw(t, r.Schema, map[string]any{
		"enterprise_slug": enterpriseSlug,
		"role_id":         int(roleID),
		"team_slug":       teamSlug,
	})
	meta := &Owner{v3client: mustCreateTestGitHubClient(t, server.URL+"/"), maxPerPage: 100}

	if diags := resourceGithubEnterpriseRoleTeamCreate(t.Context(), d, meta); diags.HasError() {
		t.Fatalf("create diagnostics: %v", diags)
	}
	if got, want := d.Id(), "example:8030:ent:security"; got != want {
		t.Fatalf("resource ID = %q, want %q", got, want)
	}

	if diags := resourceGithubEnterpriseRoleTeamRead(t.Context(), d, meta); diags.HasError() {
		t.Fatalf("read diagnostics: %v", diags)
	}
	if d.Id() == "" {
		t.Fatal("read unexpectedly removed the resource from state")
	}

	if diags := resourceGithubEnterpriseRoleTeamDelete(t.Context(), d, meta); diags.HasError() {
		t.Fatalf("delete diagnostics: %v", diags)
	}
	if assigned {
		t.Fatal("delete did not remove the assignment")
	}

	if diags := resourceGithubEnterpriseRoleTeamRead(t.Context(), d, meta); diags.HasError() {
		t.Fatalf("read after delete diagnostics: %v", diags)
	}
	if d.Id() != "" {
		t.Fatalf("resource ID after missing assignment = %q, want empty", d.Id())
	}
}

func TestEnterpriseRoleAssignedToTeamPaginates(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/enterprises/example/enterprise-roles/8030/teams", func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Query().Get("page") == "2" {
			mustWrite(w, `[{"slug":"ent:wanted"}]`)
			return
		}

		w.Header().Set("Link", fmt.Sprintf(`<http://%s/enterprises/example/enterprise-roles/8030/teams?page=2>; rel="next"`, req.Host))
		mustWrite(w, `[{"slug":"ent:other"}]`)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := mustCreateTestGitHubClient(t, server.URL+"/")
	got, err := enterpriseRoleAssignedToTeam(t.Context(), client, 100, "example", "ent:wanted", 8030)
	if err != nil {
		t.Fatalf("enterpriseRoleAssignedToTeam() error = %v", err)
	}
	if !got {
		t.Fatal("enterpriseRoleAssignedToTeam() = false, want true")
	}
}

func TestAccGithubEnterpriseRoleTeam(t *testing.T) {
	t.Parallel()

	skipUnlessEnterprise(t)

	team := mustCreateTestEnterpriseTeam(t)
	roleID := mustGetTestEnterpriseRoleID(t)

	const resourceName = "github_enterprise_role_team.test"
	config := fmt.Sprintf(`
resource "github_enterprise_role_team" "test" {
	enterprise_slug = %q
	role_id         = %d
	team_slug       = %q
}
`, testAccConf.enterpriseSlug, roleID, team.Slug)

	resource.Test(t, resource.TestCase{
		ProviderFactories: providerFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("enterprise_slug"), knownvalue.StringExact(testAccConf.enterpriseSlug)),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("role_id"), knownvalue.Int64Exact(roleID)),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("team_slug"), knownvalue.StringExact(team.Slug)),
				},
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func mustGetTestEnterpriseRoleID(t *testing.T) int64 {
	t.Helper()

	endpoint := fmt.Sprintf("enterprises/%s/enterprise-roles", testAccConf.enterpriseSlug)
	req, err := testAccConf.meta.v3client.NewRequest(t.Context(), http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("create enterprise roles request: %v", err)
	}

	var response struct {
		Roles []struct {
			ID int64 `json:"id"`
		} `json:"roles"`
	}
	if _, err := testAccConf.meta.v3client.Do(req, &response); err != nil {
		t.Skipf("enterprise roles API is unavailable to the acceptance test account: %v", err)
	}
	if len(response.Roles) == 0 {
		t.Skip("acceptance test enterprise has no enterprise roles")
	}

	if response.Roles[0].ID < 1 {
		t.Fatalf("enterprise role ID = %s, want a positive integer", strconv.FormatInt(response.Roles[0].ID, 10))
	}

	return response.Roles[0].ID
}
