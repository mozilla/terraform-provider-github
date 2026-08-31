package github

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestUpdateEnterpriseTeamMembersBatchesRequests(t *testing.T) {
	t.Parallel()

	members := make([]string, 0, 201)
	for i := range 201 {
		members = append(members, fmt.Sprintf("user-%03d", i))
	}

	tests := map[string]struct {
		current      []string
		want         []string
		operationURL string
	}{
		"add": {
			want:         members,
			operationURL: "/enterprises/example/teams/example-team/memberships/add",
		},
		"remove": {
			current:      members,
			operationURL: "/enterprises/example/teams/example-team/memberships/remove",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var batches [][]string
			mux := http.NewServeMux()
			mux.HandleFunc("/enterprises/example/teams/example-team/memberships", func(w http.ResponseWriter, _ *http.Request) {
				users := make([]map[string]string, 0, len(tc.current))
				for _, login := range tc.current {
					users = append(users, map[string]string{"login": login})
				}
				if err := json.NewEncoder(w).Encode(users); err != nil {
					t.Errorf("encode members response: %v", err)
				}
			})
			mux.HandleFunc(tc.operationURL, func(w http.ResponseWriter, req *http.Request) {
				var body struct {
					Usernames []string `json:"usernames"`
				}
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					t.Errorf("decode membership request: %v", err)
				}
				batches = append(batches, body.Usernames)
				mustWrite(w, `[]`)
			})

			server := httptest.NewServer(mux)
			defer server.Close()

			meta := &Owner{v3client: mustCreateTestGitHubClient(t, server.URL+"/"), maxPerPage: 100}
			if err := updateEnterpriseTeamMembers(t.Context(), meta, "example", "example-team", tc.want); err != nil {
				t.Fatalf("updateEnterpriseTeamMembers() error = %v", err)
			}

			if got, want := len(batches), 3; got != want {
				t.Fatalf("request count = %d, want %d", got, want)
			}
			for i, want := range []int{100, 100, 1} {
				if got := len(batches[i]); got != want {
					t.Errorf("batch %d length = %d, want %d", i, got, want)
				}
			}
			if got := slices.Concat(batches...); !slices.Equal(got, members) {
				t.Errorf("batched members = %v, want %v", got, members)
			}
		})
	}
}

func TestAccGithubEnterpriseTeamMembers(t *testing.T) {
	t.Parallel()

	skipUnlessEnterprise(t)
	skipUnlessHasOrgUser1(t)
	skipUnlessHasOrgUser2(t)

	const resourceName = "github_enterprise_team_members.test"

	config := `
resource "github_enterprise_team_members" "test" {
	enterprise_slug = "%s"
	team_slug       = "%s"
	members         = %s
}
`

	t.Run("adds and removes members", func(t *testing.T) {
		t.Parallel()

		team := mustCreateTestEnterpriseTeam(t)
		both := fmt.Sprintf(`["%s", "%s"]`, testAccConf.testOrgUser1, testAccConf.testOrgUser2)
		one := fmt.Sprintf(`["%s"]`, testAccConf.testOrgUser1)

		resource.Test(t, resource.TestCase{
			ProviderFactories: providerFactories,
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(config, testAccConf.enterpriseSlug, team.Slug, both),
					ConfigStateChecks: []statecheck.StateCheck{
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("team_slug"), knownvalue.StringExact(team.Slug)),
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("members"), knownvalue.SetSizeExact(2)),
					},
				},
				{
					Config: fmt.Sprintf(config, testAccConf.enterpriseSlug, team.Slug, one),
					ConfigStateChecks: []statecheck.StateCheck{
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("members"), knownvalue.SetSizeExact(1)),
					},
				},
				{
					Config: fmt.Sprintf(config, testAccConf.enterpriseSlug, team.Slug, both),
					ConfigStateChecks: []statecheck.StateCheck{
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("members"), knownvalue.SetSizeExact(2)),
					},
				},
				{
					ResourceName:      resourceName,
					ImportState:       true,
					ImportStateVerify: true,
				},
			},
		})
	})

	t.Run("removes members added out of band", func(t *testing.T) {
		t.Parallel()

		team := mustCreateTestEnterpriseTeam(t)
		one := fmt.Sprintf(`["%s"]`, testAccConf.testOrgUser1)

		resource.Test(t, resource.TestCase{
			ProviderFactories: providerFactories,
			Steps: []resource.TestStep{
				{
					// The resource is authoritative, so a member added outside Terraform is
					// removed rather than adopted.
					PreConfig: func() {
						mustAddEnterpriseTeamMembers(t, team, testAccConf.testOrgUser2)
					},
					Config: fmt.Sprintf(config, testAccConf.enterpriseSlug, team.Slug, one),
					ConfigStateChecks: []statecheck.StateCheck{
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("members"), knownvalue.SetSizeExact(1)),
					},
				},
			},
		})
	})

	t.Run("tolerates differences in username casing", func(t *testing.T) {
		t.Parallel()

		team := mustCreateTestEnterpriseTeam(t)
		// The set hash lowercases, so an upper-cased login must not plan as a change.
		shouted := fmt.Sprintf(`["%s"]`, strings.ToUpper(testAccConf.testOrgUser1))

		resource.Test(t, resource.TestCase{
			ProviderFactories: providerFactories,
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(config, testAccConf.enterpriseSlug, team.Slug, shouted),
					ConfigStateChecks: []statecheck.StateCheck{
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("members"), knownvalue.SetSizeExact(1)),
					},
				},
			},
		})
	})
}
