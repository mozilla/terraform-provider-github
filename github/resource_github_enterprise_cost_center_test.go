package github

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestResourceGithubEnterpriseCostCenterLifecycle(t *testing.T) {
	t.Parallel()

	const path = "/enterprises/example/settings/billing/cost-centers"
	center := costCenter{ID: "cc-1", Name: "Engineering", State: "active", AICreditPoolEnabled: false}
	var archived bool
	mux := http.NewServeMux()
	mux.HandleFunc(path, func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("X-GitHub-Api-Version") != costCenterAPIVersion {
			t.Errorf("API version = %q", req.Header.Get("X-GitHub-Api-Version"))
		}
		if req.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", req.Method)
		}
		var body map[string]any
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["name"] != "Engineering" || body["ai_credit_pool_enabled"] != false {
			t.Errorf("create body = %#v", body)
		}
		json.NewEncoder(w).Encode(center)
	})
	mux.HandleFunc(path+"/cc-1", func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("X-GitHub-Api-Version") != costCenterAPIVersion {
			t.Errorf("API version = %q", req.Header.Get("X-GitHub-Api-Version"))
		}
		switch req.Method {
		case http.MethodGet:
			if archived {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			json.NewEncoder(w).Encode(center)
		case http.MethodPatch:
			var body map[string]any
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if name, ok := body["name"].(string); ok {
				center.Name = name
			}
			if enabled, ok := body["ai_credit_pool_enabled"].(bool); ok {
				center.AICreditPoolEnabled = enabled
			}
			json.NewEncoder(w).Encode(center)
		case http.MethodDelete:
			archived = true
			json.NewEncoder(w).Encode(map[string]string{"message": "Cost center successfully deleted."})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	meta := &Owner{v3client: mustCreateTestGitHubClient(t, server.URL+"/")}
	d := schema.TestResourceDataRaw(t, resourceGithubEnterpriseCostCenter().Schema, map[string]any{
		"enterprise_slug": "example", "name": "Engineering", "ai_credit_pool_enabled": false,
	})
	if diags := resourceGithubEnterpriseCostCenterCreate(t.Context(), d, meta); diags.HasError() {
		t.Fatalf("create: %v", diags)
	}
	if d.Id() != "example:cc-1" || d.Get("cost_center_id") != "cc-1" {
		t.Errorf("created ID = %q, cost center ID = %v", d.Id(), d.Get("cost_center_id"))
	}
	if err := d.Set("name", "Platform"); err != nil {
		t.Fatal(err)
	}
	if diags := resourceGithubEnterpriseCostCenterUpdate(t.Context(), d, meta); diags.HasError() {
		t.Fatalf("update: %v", diags)
	}
	if center.Name != "Platform" {
		t.Errorf("name after update = %q", center.Name)
	}
	if diags := resourceGithubEnterpriseCostCenterDelete(t.Context(), d, meta); diags.HasError() {
		t.Fatalf("delete: %v", diags)
	}
	if !archived || d.Id() != "" {
		t.Errorf("archived = %v, ID = %q", archived, d.Id())
	}
}

func TestResourceGithubEnterpriseCostCenterResourcesReconcile(t *testing.T) {
	t.Parallel()
	const path = "/enterprises/example/settings/billing/cost-centers/cc-1"
	resources := []costCenterResource{{Type: "User", Name: "alice"}, {Type: "Repo", Name: "org/old"}}
	var added, removed costCenterResourcesChange
	mux := http.NewServeMux()
	mux.HandleFunc(path, func(w http.ResponseWriter, req *http.Request) {
		json.NewEncoder(w).Encode(costCenter{ID: "cc-1", State: "active", Resources: resources})
	})
	mux.HandleFunc(path+"/resource", func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("X-GitHub-Api-Version") != costCenterAPIVersion {
			t.Errorf("API version = %q", req.Header.Get("X-GitHub-Api-Version"))
		}
		var change costCenterResourcesChange
		if err := json.NewDecoder(req.Body).Decode(&change); err != nil {
			t.Fatal(err)
		}
		switch req.Method {
		case http.MethodPost:
			added = change
			for _, user := range change.Users {
				resources = append(resources, costCenterResource{Type: "User", Name: user})
			}
			for _, team := range change.EnterpriseTeams {
				resources = append(resources, costCenterResource{Type: "Team", Name: team})
			}
		case http.MethodDelete:
			removed = change
			resources = slices.DeleteFunc(resources, func(r costCenterResource) bool {
				return slices.Contains(change.Repositories, r.Name) || slices.Contains(change.Users, r.Name) || slices.Contains(change.EnterpriseTeams, r.Name)
			})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
		json.NewEncoder(w).Encode(map[string]string{"message": "ok"})
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	meta := &Owner{v3client: mustCreateTestGitHubClient(t, server.URL+"/")}
	d := schema.TestResourceDataRaw(t, resourceGithubEnterpriseCostCenterResources().Schema, map[string]any{
		"enterprise_slug": "example", "cost_center_id": "cc-1",
		"users": []any{"Alice", "Bob"}, "enterprise_teams": []any{"ent:platform"},
	})
	if diags := resourceGithubEnterpriseCostCenterResourcesCreate(t.Context(), d, meta); diags.HasError() {
		t.Fatalf("create: %v", diags)
	}
	if d.Id() != "example:cc-1" || !slices.Equal(added.Users, []string{"bob"}) || !slices.Equal(added.EnterpriseTeams, []string{"ent:platform"}) || !slices.Equal(removed.Repositories, []string{"org/old"}) {
		t.Errorf("ID = %q, added = %#v, removed = %#v", d.Id(), added, removed)
	}
	if diags := resourceGithubEnterpriseCostCenterResourcesDelete(t.Context(), d, meta); diags.HasError() {
		t.Fatalf("delete: %v", diags)
	}
	if len(resources) != 0 || d.Id() != "" {
		t.Errorf("resources after delete = %#v, ID = %q", resources, d.Id())
	}
}

func TestResourceGithubEnterpriseCostCenterMissingOnRead(t *testing.T) {
	t.Parallel()

	for _, status := range []int{http.StatusNotFound, http.StatusOK} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.WriteHeader(status)
				if status == http.StatusOK {
					json.NewEncoder(w).Encode(costCenter{ID: "cc-1", State: "deleted"})
				}
			}))
			defer server.Close()
			meta := &Owner{v3client: mustCreateTestGitHubClient(t, server.URL+"/")}
			for _, r := range []*schema.Resource{resourceGithubEnterpriseCostCenter(), resourceGithubEnterpriseCostCenterResources()} {
				d := schema.TestResourceDataRaw(t, r.Schema, nil)
				d.SetId("example:cc-1")
				if diags := r.ReadContext(t.Context(), d, meta); diags.HasError() {
					t.Fatalf("read: %v", diags)
				}
				if d.Id() != "" {
					t.Errorf("ID after missing or archived cost center = %q", d.Id())
				}
			}
		})
	}
}
