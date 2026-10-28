package github

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/google/go-github/v92/github"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccGithubAppDataSource(t *testing.T) {
	t.Parallel()

	t.Run("global", func(t *testing.T) {
		t.Parallel()

		config := `
			data "github_app" "test" {
				slug = "github-actions"
			}
		`

		check := resource.ComposeTestCheckFunc(
			resource.TestCheckResourceAttrSet(
				"data.github_app.test", "name",
			),
		)

		resource.Test(t, resource.TestCase{
			ProviderFactories: providerFactories,
			Steps: []resource.TestStep{
				{
					Config: config,
					Check:  check,
				},
			},
		})
	})
}

func TestDataSourceGithubAppRead(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		response string
		owner    map[string]any
	}{
		{name: "user", response: `{
  "id": 123,
  "slug": "test-app",
  "node_id": "A_123",
  "name": "Test App",
  "description": "Example app",
  "client_id": "Iv1.example",
  "external_url": "https://example.com",
  "html_url": "https://github.com/apps/test-app",
  "created_at": "2026-03-10T12:34:56-04:00",
  "updated_at": "2026-03-11T12:34:56Z",
  "permissions": {
    "contents": "read",
    "enterprise_organizations": "write",
    "future_permission": "read",
    "single_file": "write"
  },
  "events": [
    "push",
    "pull_request"
  ],
  "installations_count": 7,
  "owner": {
    "name": "owner-name",
    "email": "owner-email",
    "login": "owner-login",
    "id": 42,
    "node_id": "owner-node_id",
    "avatar_url": "owner-avatar_url",
    "gravatar_id": "owner-gravatar_id",
    "url": "owner-url",
    "html_url": "owner-html_url",
    "followers_url": "owner-followers_url",
    "following_url": "owner-following_url",
    "gists_url": "owner-gists_url",
    "starred_url": "owner-starred_url",
    "subscriptions_url": "owner-subscriptions_url",
    "organizations_url": "owner-organizations_url",
    "repos_url": "owner-repos_url",
    "events_url": "owner-events_url",
    "received_events_url": "owner-received_events_url",
    "type": "owner-type",
    "site_admin": true,
    "starred_at": "owner-starred_at",
    "user_view_type": "owner-user_view_type"
  }
}`, owner: map[string]any{
			"name":                "owner-name",
			"email":               "owner-email",
			"login":               "owner-login",
			"id":                  42,
			"node_id":             "owner-node_id",
			"avatar_url":          "owner-avatar_url",
			"gravatar_id":         "owner-gravatar_id",
			"url":                 "owner-url",
			"html_url":            "owner-html_url",
			"followers_url":       "owner-followers_url",
			"following_url":       "owner-following_url",
			"gists_url":           "owner-gists_url",
			"starred_url":         "owner-starred_url",
			"subscriptions_url":   "owner-subscriptions_url",
			"organizations_url":   "owner-organizations_url",
			"repos_url":           "owner-repos_url",
			"events_url":          "owner-events_url",
			"received_events_url": "owner-received_events_url",
			"type":                "owner-type",
			"site_admin":          true,
			"starred_at":          "owner-starred_at",
			"user_view_type":      "owner-user_view_type",
		}},
		{name: "enterprise", response: `{
  "id": 123,
  "slug": "test-app",
  "node_id": "A_123",
  "name": "Test App",
  "description": "Example app",
  "client_id": "Iv1.example",
  "external_url": "https://example.com",
  "html_url": "https://github.com/apps/test-app",
  "created_at": "2026-03-10T12:34:56-04:00",
  "updated_at": "2026-03-11T12:34:56Z",
  "permissions": {
    "contents": "read",
    "enterprise_organizations": "write",
    "future_permission": "read",
    "single_file": "write"
  },
  "events": [
    "push",
    "pull_request"
  ],
  "installations_count": 7,
  "owner": {
    "description": "enterprise-description",
    "html_url": "enterprise-html_url",
    "website_url": "enterprise-website_url",
    "id": 99,
    "node_id": "enterprise-node_id",
    "name": "enterprise-name",
    "slug": "enterprise-slug",
    "created_at": "2026-03-10T12:34:56Z",
    "updated_at": "2026-03-10T12:34:56Z",
    "avatar_url": "enterprise-avatar_url"
  }
}`, owner: map[string]any{
			"description": "enterprise-description",
			"html_url":    "enterprise-html_url",
			"website_url": "enterprise-website_url",
			"id":          99,
			"node_id":     "enterprise-node_id",
			"name":        "enterprise-name",
			"slug":        "enterprise-slug",
			"created_at":  "2026-03-10T12:34:56Z",
			"updated_at":  "2026-03-10T12:34:56Z",
			"avatar_url":  "enterprise-avatar_url",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			response := tt.response
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/apps/test-app" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				mustWrite(w, response)
			}))
			defer server.Close()
			baseURL := server.URL + "/"
			client, err := github.NewClient(github.WithURLs(&baseURL, nil))
			if err != nil {
				t.Fatal(err)
			}
			data := schema.TestResourceDataRaw(t, dataSourceGithubApp().Schema, map[string]any{"slug": "test-app"})
			owner := &Owner{v3client: client}
			if diags := dataSourceGithubAppRead(t.Context(), data, owner); diags.HasError() {
				t.Fatal(diags)
			}
			if data.Id() != "123" {
				t.Fatalf("id = %q, want 123", data.Id())
			}
			want := map[string]any{
				"slug":         "test-app",
				"node_id":      "A_123",
				"name":         "Test App",
				"description":  "Example app",
				"client_id":    "Iv1.example",
				"external_url": "https://example.com",
				"html_url":     "https://github.com/apps/test-app",
				"created_at":   "2026-03-10T12:34:56-04:00",
				"updated_at":   "2026-03-11T12:34:56Z",
				"permissions": map[string]any{
					"contents":                 "read",
					"enterprise_organizations": "write",
					"future_permission":        "read",
					"single_file":              "write",
				},
				"events":              []any{"push", "pull_request"},
				"installations_count": 7,
			}
			for key, expected := range want {
				if got := data.Get(key); !reflect.DeepEqual(got, expected) {
					t.Errorf("%s = %#v, want %#v", key, got, expected)
				}
			}
			owners := data.Get("owner").([]any)
			if len(owners) != 1 {
				t.Fatalf("owner = %#v, want one owner", owners)
			}
			for key, expected := range tt.owner {
				if got := owners[0].(map[string]any)[key]; !reflect.DeepEqual(got, expected) {
					t.Errorf("owner.%s = %#v, want %#v", key, got, expected)
				}
			}

			// Refreshing an app must clear values that GitHub no longer returns.
			response = `{"id":123,"description":null,"owner":null,"permissions":null,"events":null}`
			if diags := dataSourceGithubAppRead(t.Context(), data, owner); diags.HasError() {
				t.Fatal(diags)
			}
			for key, expected := range map[string]any{
				"client_id": "", "description": "", "name": "", "node_id": "", "external_url": "", "html_url": "",
				"created_at": "", "updated_at": "", "permissions": map[string]any{}, "events": []any{}, "owner": []any{}, "installations_count": 0,
			} {
				if got := data.Get(key); !reflect.DeepEqual(got, expected) {
					t.Errorf("after refresh %s = %#v, want %#v", key, got, expected)
				}
			}
		})
	}
}

func TestDataSourceGithubAppReadError(t *testing.T) {
	t.Parallel()
	for _, status := range []int{http.StatusForbidden, http.StatusNotFound} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
			}))
			defer server.Close()
			baseURL := server.URL + "/"
			client, err := github.NewClient(github.WithURLs(&baseURL, nil))
			if err != nil {
				t.Fatal(err)
			}
			data := schema.TestResourceDataRaw(t, dataSourceGithubApp().Schema, map[string]any{"slug": "test-app"})
			if diags := dataSourceGithubAppRead(t.Context(), data, &Owner{v3client: client}); !diags.HasError() {
				t.Fatal("expected API error")
			}
			if data.Id() != "" {
				t.Errorf("id = %q after failed lookup", data.Id())
			}
		})
	}
}
