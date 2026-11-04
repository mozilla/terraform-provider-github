package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestActionsPolicyTerraformLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name    string
		scope   actionsPolicyScope
		parent  string
		targets string
		path    string
	}{
		{"github_actions_repository_policy", actionsPolicyRepository, `repository = "repo"`, "", "/repos/org/repo/actions/policies"},
		{"github_actions_organization_policy", actionsPolicyOrganization, "", `repository_name { include = ["repo"] }`, "/orgs/org/actions/policies"},
		{"github_actions_enterprise_policy", actionsPolicyEnterprise, `enterprise_slug = "enterprise"`, `organization_name { include = ["org"] }
repository_name { include = ["repo"] }`, "/enterprises/enterprise/actions/policies"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			var stored *actionsPolicy
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				if req.Header.Get("X-GitHub-Api-Version") != actionsPolicyAPIVersion {
					t.Errorf("API version = %q", req.Header.Get("X-GitHub-Api-Version"))
				}
				if req.URL.Path != tc.path && req.URL.Path != tc.path+"/10894" {
					t.Errorf("unexpected path %s", req.URL.Path)
					http.NotFound(w, req)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				switch req.Method {
				case http.MethodPost, http.MethodPut:
					var change actionsPolicy
					if err := json.NewDecoder(req.Body).Decode(&change); err != nil {
						t.Error(err)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					if stored != nil {
						if _, present := change.Conditions["workflow_path"]; !present {
							if workflow, exists := stored.Conditions["workflow_path"]; exists {
								change.Conditions["workflow_path"] = workflow
							}
						}
					}
					change.ID = 10894
					change.NodeID = "node-policy"
					change.SourceType = string(tc.scope)
					stored = &change
					if req.Method == http.MethodPost {
						w.WriteHeader(http.StatusCreated)
					}
				case http.MethodGet:
					if stored == nil {
						w.WriteHeader(http.StatusNotFound)
						return
					}
				case http.MethodDelete:
					stored = nil
					w.WriteHeader(http.StatusNoContent)
					return
				default:
					t.Errorf("unexpected method %s", req.Method)
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				if err := json.NewEncoder(w).Encode(stored); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			meta := &Owner{name: "org", v3client: mustCreateTestGitHubClient(t, server.URL+"/")}
			factory := func() (*schema.Provider, error) {
				p := &schema.Provider{ResourcesMap: map[string]*schema.Resource{tc.name: resourceGithubActionsPolicy(tc.scope)}, ConfigureContextFunc: func(context.Context, *schema.ResourceData) (any, diag.Diagnostics) { return meta, nil }}
				return p, p.InternalValidate()
			}
			config := func(name, workflow, rules string) string {
				conditions := ""
				if tc.targets != "" || workflow != "" {
					conditions = "conditions {\n" + tc.targets + "\n" + workflow + "\n}"
				}
				return fmt.Sprintf("resource %q \"test\" {\n%s\nname = %q\nenforcement = \"disabled\"\n%s\nrules {\n%s\n}\n}", tc.name, tc.parent, name, conditions, rules)
			}
			address := tc.name + ".test"
			resource.UnitTest(t, resource.TestCase{
				ProviderFactories: map[string]func() (*schema.Provider, error){"github": factory},
				Steps: []resource.TestStep{
					{Config: config("all rules", `workflow_path { include = [".github/workflows/*.yml"] }`, `require_lockfile = true
restrict_actions_actors {
 allowed_actors {
 id = 578466
 type = "User"
 }
 allowed_actors {
 id = 5
 type = "RepositoryRole"
 }
 allowed_actors {
 id = 18367458
 type = "Team"
 }
 allowed_actors {
 id = 1693627
 type = "App"
 }
 allowed_actors {
 id = 49699333
 type = "Bot"
 }
}
restrict_action_events { allowed_events = ["push", "pull_request", "workflow_dispatch"] }`), ConfigStateChecks: []statecheck.StateCheck{
						statecheck.ExpectKnownValue(address, tfjsonpath.New("policy_id"), knownvalue.Int64Exact(10894)),
						statecheck.ExpectKnownValue(address, tfjsonpath.New("node_id"), knownvalue.StringExact("node-policy")),
					}},
					{ResourceName: address, ImportState: true, ImportStateVerify: true},
					{Config: config("deny all", "", `restrict_actions_actors {}
restrict_action_events {}`)},
					{Config: config("all workflows", `workflow_path { include = ["~ALL"] }`, `require_lockfile = true`)},
					{ResourceName: address, ImportState: true, ImportStateVerify: true},
					{Config: config("lockfile only", "", `require_lockfile = true`)},
					{Config: config("no restrictions", "", "")},
				},
			})
			mu.Lock()
			defer mu.Unlock()
			if stored != nil {
				t.Error("policy was not deleted")
			}
		})
	}
}

func TestActionsPolicyConditionsRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		scope      actionsPolicyScope
		conditions string
	}{
		{actionsPolicyRepository, `{"workflow_path":{"include":[],"exclude":[".github/workflows/legacy.yml"]}}`},
		{actionsPolicyOrganization, `{"repository_id":{"repository_ids":[123,456]}}`},
		{actionsPolicyOrganization, `{"repository_name":{"include":["~ALL"],"exclude":["legacy"],"protected":true}}`},
		{actionsPolicyOrganization, `{"repository_property":{"include":[{"name":"tier","source":"custom","property_values":["production"]}],"exclude":[{"name":"visibility","source":"system","property_values":["public"]}]}}`},
		{actionsPolicyEnterprise, `{"organization_name":{"include":["~ALL"],"exclude":[]},"repository_name":{"include":["~ALL"],"exclude":[],"protected":false}}`},
		{actionsPolicyEnterprise, `{"organization_id":{"organization_ids":[123]},"repository_property":{"include":[{"name":"tier","source":"custom","property_values":["production"]}],"exclude":[]}}`},
		{actionsPolicyEnterprise, `{"organization_property":{"include":[{"name":"division","property_values":["engineering"]}],"exclude":[]},"repository_name":{"include":["~ALL"],"exclude":[],"protected":false}}`},
	} {
		t.Run(string(tc.scope)+tc.conditions, func(t *testing.T) {
			r := resourceGithubActionsPolicy(tc.scope)
			var conditions map[string]any
			if err := json.Unmarshal([]byte(tc.conditions), &conditions); err != nil {
				t.Fatal(err)
			}
			d := schema.TestResourceDataRaw(t, r.Schema, nil)
			if err := flattenActionsPolicy(d, actionsPolicy{ID: 10894, Name: "test", Enforcement: "disabled", Conditions: conditions}, r.Schema, true); err != nil {
				t.Fatal(err)
			}
			value := d.Get("conditions").([]any)[0].(map[string]any)
			expanded := expandActionsPolicyObject(value, r.Schema["conditions"].Elem.(*schema.Resource).Schema)
			// Compare after JSON encoding to normalize Go integers and set ordering.
			data, err := json.Marshal(expanded)
			if err != nil {
				t.Fatal(err)
			}
			var actual map[string]any
			if err := json.Unmarshal(data, &actual); err != nil {
				t.Fatal(err)
			}
			other := schema.TestResourceDataRaw(t, r.Schema, nil)
			if err := other.Set("conditions", []any{flattenActionsPolicyObject(actual, r.Schema["conditions"].Elem.(*schema.Resource).Schema)}); err != nil {
				t.Fatal(err)
			}
			left := schema.NewSet(schema.HashResource(r.Schema["conditions"].Elem.(*schema.Resource)), d.Get("conditions").([]any))
			right := schema.NewSet(schema.HashResource(r.Schema["conditions"].Elem.(*schema.Resource)), other.Get("conditions").([]any))
			if !left.Equal(right) {
				t.Errorf("round trip: want %s, got %s", tc.conditions, data)
			}
		})
	}
}

func TestActionsPolicyReadAndDeleteErrors(t *testing.T) {
	for _, scope := range []actionsPolicyScope{actionsPolicyRepository, actionsPolicyOrganization, actionsPolicyEnterprise} {
		for _, status := range []int{http.StatusNotFound, http.StatusForbidden, http.StatusInternalServerError} {
			t.Run(fmt.Sprintf("%s/%d", scope, status), func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { w.WriteHeader(status) }))
				defer server.Close()
				r := resourceGithubActionsPolicy(scope)
				meta := &Owner{name: "org", v3client: mustCreateTestGitHubClient(t, server.URL+"/")}
				for _, operation := range []func(context.Context, *schema.ResourceData, any) diag.Diagnostics{r.ReadContext, r.DeleteContext} {
					d := schema.TestResourceDataRaw(t, r.Schema, map[string]any{"repository": "repo", "enterprise_slug": "enterprise"})
					id := "10894"
					if scope != actionsPolicyOrganization {
						id = "parent:" + id
					}
					d.SetId(id)
					diags := operation(t.Context(), d, meta)
					if status == http.StatusNotFound {
						if diags.HasError() || d.Id() != "" {
							t.Errorf("404 should clear state: %v, id %q", diags, d.Id())
						}
					} else if !diags.HasError() || d.Id() != id {
						t.Errorf("error should preserve state: %v, id %q", diags, d.Id())
					}
				}
			})
		}
	}
}

func TestActionsPolicyImportValidation(t *testing.T) {
	for _, scope := range []actionsPolicyScope{actionsPolicyRepository, actionsPolicyOrganization, actionsPolicyEnterprise} {
		r := resourceGithubActionsPolicy(scope)
		for _, id := range []string{"", "0", "-1", "abc", "repo:abc", "repo:0", "repo:1:2", ":123"} {
			d := schema.TestResourceDataRaw(t, r.Schema, nil)
			d.SetId(id)
			if _, err := r.Importer.StateContext(t.Context(), d, nil); err == nil {
				t.Errorf("%s accepted invalid import %q", scope, id)
			}
		}
	}
}

func TestActionsPolicyUnknownRule(t *testing.T) {
	r := resourceGithubActionsRepositoryPolicy()
	d := schema.TestResourceDataRaw(t, r.Schema, nil)
	err := flattenActionsPolicy(d, actionsPolicy{Rules: []actionsPolicyRule{{Type: "future_rule"}}}, r.Schema, false)
	if err == nil || !strings.Contains(err.Error(), "unsupported rule type") {
		t.Fatalf("unknown rule error = %v", err)
	}
}

func TestActionsPolicyWorkflowValidation(t *testing.T) {
	for _, tc := range []struct{ workflow, message string }{
		{`include = []`, "at least one included or excluded"},
		{`include = ["~ALL", "build.yml"]`, "cannot combine"},
		{`exclude = ["~ALL"]`, "exclude cannot contain"},
	} {
		t.Run(tc.message, func(t *testing.T) {
			resource.UnitTest(t, resource.TestCase{ProviderFactories: map[string]func() (*schema.Provider, error){"github": func() (*schema.Provider, error) {
				p := &schema.Provider{ResourcesMap: map[string]*schema.Resource{"github_actions_repository_policy": resourceGithubActionsRepositoryPolicy()}}
				return p, p.InternalValidate()
			}}, Steps: []resource.TestStep{{Config: fmt.Sprintf(`resource "github_actions_repository_policy" "test" {
repository = "repo"
name = "test"
enforcement = "disabled"
conditions {
 workflow_path { %s }
}
rules { require_lockfile = true }
}`, tc.workflow), ExpectError: regexp.MustCompile(tc.message)}}})
		})
	}
}
