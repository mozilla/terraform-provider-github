package github

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-github/v92/github"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestDataSourceGithubOrganizationOutsideCollaboratorsRead(t *testing.T) {
	t.Parallel()

	requests := 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requests++
		if got, want := req.URL.Path, "/orgs/example-organization/outside_collaborators"; got != want {
			t.Errorf("request path = %q, want %q", got, want)
		}
		if got, want := req.URL.Query().Get("per_page"), "1"; got != want {
			t.Errorf("per_page = %q, want %q", got, want)
		}

		w.Header().Set("Content-Type", "application/json")
		if requests == 1 {
			w.Header().Set("Link", fmt.Sprintf(`<%s/orgs/example-organization/outside_collaborators?page=2&per_page=1>; rel="next"`, server.URL))
			mustWrite(w, `[{"id":101,"node_id":"U_first","login":"octocat"}]`)
			return
		}

		if got, want := req.URL.Query().Get("page"), "2"; got != want {
			t.Errorf("page = %q, want %q", got, want)
		}
		mustWrite(w, `[{"id":202,"node_id":"U_second","login":"monalisa"}]`)
	}))
	defer server.Close()

	baseURL := server.URL + "/"
	client, err := github.NewClient(github.WithURLs(&baseURL, nil))
	if err != nil {
		t.Fatalf("creating GitHub client: %v", err)
	}

	data := schema.TestResourceDataRaw(t, dataSourceGithubOrganizationOutsideCollaborators().Schema, map[string]any{
		"organization": "example-organization",
	})
	meta := &Owner{v3client: client, maxPerPage: 1}

	if diags := dataSourceGithubOrganizationOutsideCollaboratorsRead(t.Context(), data, meta); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if got, want := data.Id(), "example-organization"; got != want {
		t.Errorf("id = %q, want %q", got, want)
	}
	if got, want := requests, 2; got != want {
		t.Errorf("REST requests = %d, want %d", got, want)
	}

	collaborators := data.Get("outside_collaborators").([]any)
	want := []map[string]any{
		{"id": 101, "node_id": "U_first", "login": "octocat"},
		{"id": 202, "node_id": "U_second", "login": "monalisa"},
	}
	if got := len(collaborators); got != len(want) {
		t.Fatalf("outside_collaborators length = %d, want %d", got, len(want))
	}
	for i, collaborator := range collaborators {
		got := collaborator.(map[string]any)
		for key, value := range want[i] {
			if got[key] != value {
				t.Errorf("outside_collaborators[%d][%q] = %#v, want %#v", i, key, got[key], value)
			}
		}
	}
}
