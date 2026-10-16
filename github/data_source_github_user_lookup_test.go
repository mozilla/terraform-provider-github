package github

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/google/go-github/v92/github"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/shurcooL/githubv4"
)

func TestDataSourceGithubUserLookupValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		resource  *schema.Resource
		config    map[string]any
		wantError bool
	}{
		{"username", dataSourceGithubUser(), map[string]any{"username": "octocat"}, false},
		{"current user", dataSourceGithubUser(), map[string]any{"username": ""}, false},
		{"user ID", dataSourceGithubUser(), map[string]any{"user_id": 123}, false},
		{"missing user input", dataSourceGithubUser(), map[string]any{}, true},
		{"both user inputs", dataSourceGithubUser(), map[string]any{"username": "", "user_id": 123}, true},
		{"zero user ID", dataSourceGithubUser(), map[string]any{"user_id": 0}, true},
		{"negative user ID", dataSourceGithubUser(), map[string]any{"user_id": -1}, true},
		{"usernames", dataSourceGithubUsers(), map[string]any{"usernames": []any{"octocat"}}, false},
		{"empty usernames", dataSourceGithubUsers(), map[string]any{"usernames": []any{}}, false},
		{"user IDs", dataSourceGithubUsers(), map[string]any{"user_ids": []any{123, 456}}, false},
		{"empty user IDs", dataSourceGithubUsers(), map[string]any{"user_ids": []any{}}, false},
		{"missing users input", dataSourceGithubUsers(), map[string]any{}, true},
		{"both users inputs", dataSourceGithubUsers(), map[string]any{"usernames": []any{}, "user_ids": []any{123}}, true},
		{"zero in user IDs", dataSourceGithubUsers(), map[string]any{"user_ids": []any{123, 0}}, true},
		{"negative in user IDs", dataSourceGithubUsers(), map[string]any{"user_ids": []any{-1}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			diags := tt.resource.Validate(terraform.NewResourceConfigRaw(tt.config))
			if diags.HasError() != tt.wantError {
				t.Fatalf("validation diagnostics = %v, want error = %t", diags, tt.wantError)
			}
		})
	}
}

func TestDataSourceGithubUserReadLookup(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		config map[string]any
		path   string
		status int
	}{
		{"ID", map[string]any{"user_id": 123}, "/user/123", http.StatusOK},
		{"username", map[string]any{"username": "old-login"}, "/users/old-login", http.StatusOK},
		{"current user", map[string]any{"username": ""}, "/user", http.StatusOK},
		{"missing ID", map[string]any{"user_id": 123}, "/user/123", http.StatusNotFound},
		{"API failure", map[string]any{"user_id": 123}, "/user/123", http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var paths []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case tt.path:
					w.WriteHeader(tt.status)
					mustWrite(w, `{"id":123,"login":"new-login","node_id":"U_123","email":"public@example.com"}`)
				case "/users/new-login/gpg_keys":
					mustWrite(w, `[{"public_key":"gpg-key"}]`)
				case "/users/new-login/keys":
					mustWrite(w, `[{"key":"ssh-key"}]`)
				default:
					t.Errorf("unexpected request: %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			baseURL := server.URL + "/"
			client, err := github.NewClient(github.WithURLs(&baseURL, nil))
			if err != nil {
				t.Fatal(err)
			}
			data := schema.TestResourceDataRaw(t, dataSourceGithubUser().Schema, tt.config)
			diags := dataSourceGithubUserRead(t.Context(), data, &Owner{v3client: client})
			if tt.status != http.StatusOK {
				if !diags.HasError() {
					t.Fatal("expected lookup error")
				}
				if len(paths) != 1 {
					t.Fatalf("requests after failed lookup: %v", paths)
				}
				return
			}
			if diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			if data.Id() != "123" {
				t.Errorf("id = %q, want 123", data.Id())
			}
			for key, want := range map[string]any{
				"login": "new-login", "node_id": "U_123", "email": "public@example.com",
				"gpg_keys": []any{"gpg-key"}, "ssh_keys": []any{"ssh-key"},
			} {
				if got := data.Get(key); !reflect.DeepEqual(got, want) {
					t.Errorf("%s = %#v, want %#v", key, got, want)
				}
			}
			if len(paths) != 3 {
				t.Errorf("requests = %v, want user and both key endpoints", paths)
			}
		})
	}
}

func TestDataSourceGithubUsersReadLookup(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                                                 string
		config                                               map[string]any
		wantLogins, wantEmails, wantNodes, wantUnknownLogins []any
		wantUnknownIDs                                       []any
		wantRequests                                         int
		wantError                                            bool
	}{
		{
			name: "IDs preserve order and duplicates", config: map[string]any{"user_ids": []any{456, 999, 123, 456}},
			wantLogins: []any{"user456", "user123", "user456"}, wantEmails: []any{"", "public@example.com", ""},
			wantNodes: []any{"U_456", "U_123", "U_456"}, wantUnknownIDs: []any{999}, wantRequests: 4,
		},
		{name: "all IDs missing", config: map[string]any{"user_ids": []any{999}}, wantUnknownIDs: []any{999}, wantRequests: 1},
		{name: "empty IDs", config: map[string]any{"user_ids": []any{}}},
		{name: "empty usernames", config: map[string]any{"usernames": []any{}}},
		{name: "API failure", config: map[string]any{"user_ids": []any{500}}, wantRequests: 1, wantError: true},
		{
			name: "usernames", config: map[string]any{"usernames": []any{"old-login", "missing"}},
			wantLogins: []any{"user123"}, wantEmails: []any{"public@example.com"}, wantNodes: []any{"U_123"},
			wantUnknownLogins: []any{"missing"}, wantRequests: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/user/123":
					mustWrite(w, `{"id":123,"login":"user123","node_id":"U_123","email":"public@example.com"}`)
				case "/user/456":
					mustWrite(w, `{"id":456,"login":"user456","node_id":"U_456"}`)
				case "/user/999":
					w.WriteHeader(http.StatusNotFound)
					mustWrite(w, `{"message":"Not Found"}`)
				case "/user/500":
					w.WriteHeader(http.StatusForbidden)
					mustWrite(w, `{"message":"Forbidden"}`)
				case "/graphql":
					var body struct {
						Variables map[string]string `json:"variables"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if body.Variables["User0"] != "old-login" || body.Variables["User1"] != "missing" {
						t.Errorf("unexpected GraphQL variables: %v", body.Variables)
					}
					mustWrite(w, `{"data":{"User0":{"id":"U_123","login":"user123","email":"public@example.com"},"User1":null},"errors":[{"message":"Could not resolve to a User with the login of 'missing'."}]}`)
				default:
					t.Errorf("unexpected request: %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			baseURL := server.URL + "/"
			client, err := github.NewClient(github.WithURLs(&baseURL, nil))
			if err != nil {
				t.Fatal(err)
			}
			data := schema.TestResourceDataRaw(t, dataSourceGithubUsers().Schema, tt.config)
			meta := &Owner{v3client: client, v4client: githubv4.NewEnterpriseClient(server.URL+"/graphql", server.Client())}
			// A refresh must clear stale unmatched results when the input changes.
			for key, value := range map[string]any{"unknown_user_ids": []int{888}, "unknown_logins": []string{"stale"}} {
				if err := data.Set(key, value); err != nil {
					t.Fatal(err)
				}
			}
			diags := dataSourceGithubUsersRead(t.Context(), data, meta)
			if requests != tt.wantRequests {
				t.Errorf("requests = %d, want %d", requests, tt.wantRequests)
			}
			if diags.HasError() != tt.wantError {
				t.Fatalf("diagnostics = %v, want error = %t", diags, tt.wantError)
			}
			if tt.wantError {
				return
			}
			if data.Id() == "" {
				t.Error("missing data source ID")
			}
			for key, want := range map[string][]any{
				"logins": tt.wantLogins, "emails": tt.wantEmails, "node_ids": tt.wantNodes,
				"unknown_logins": tt.wantUnknownLogins, "unknown_user_ids": tt.wantUnknownIDs,
			} {
				got := data.Get(key).([]any)
				if len(got) != len(want) || (len(want) > 0 && !reflect.DeepEqual(got, want)) {
					t.Errorf("%s = %v, want %v", key, got, want)
				}
			}
			var identifiers []string
			for _, value := range []string{"usernames", "user_ids"} {
				if inputs, ok := tt.config[value]; ok {
					for _, input := range inputs.([]any) {
						identifiers = append(identifiers, fmt.Sprint(input))
					}
				}
			}
			if want := buildChecksumID(identifiers); data.Id() != want {
				t.Errorf("id = %q, want %q", data.Id(), want)
			}
		})
	}
}
