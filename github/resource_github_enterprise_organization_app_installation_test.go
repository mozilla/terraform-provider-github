package github

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestResourceGithubEnterpriseOrganizationAppInstallationWithoutRepositoryPermissions(t *testing.T) {
	t.Parallel()

	const (
		enterpriseSlug = "example"
		organization   = "example-org"
		clientID       = "Iv1.example"
		installationID = int64(1234)
	)

	installed := false
	installRequests := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/enterprises/example/apps/organizations/example-org/installations", func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodPost:
			installRequests++
			var body struct {
				ClientID            string   `json:"client_id"`
				RepositorySelection string   `json:"repository_selection"`
				Repositories        []string `json:"repositories"`
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Errorf("decode install request: %v", err)
			}
			if body.ClientID != clientID {
				t.Errorf("client_id = %q, want %q", body.ClientID, clientID)
			}
			if body.RepositorySelection != enterpriseAppInstallationRepositorySelectionNone {
				t.Errorf("repository_selection = %q, want %q", body.RepositorySelection, enterpriseAppInstallationRepositorySelectionNone)
			}
			if len(body.Repositories) != 0 {
				t.Errorf("repositories = %v, want empty", body.Repositories)
			}

			installed = true
			w.WriteHeader(http.StatusCreated)
			mustWrite(w, `{"id":1234,"client_id":"Iv1.example","app_slug":"example-app","repository_selection":"selected"}`)
		case http.MethodGet:
			if !installed {
				mustWrite(w, `[]`)
				return
			}

			mustWrite(w, `[{"id":1234,"client_id":"Iv1.example","app_slug":"example-app","repository_selection":"selected"}]`)
		default:
			t.Errorf("request method = %s, want GET or POST", req.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/enterprises/example/apps/organizations/example-org/installations/1234/repositories", func(w http.ResponseWriter, req *http.Request) {
		mustWrite(w, `[]`)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	r := resourceGithubEnterpriseOrganizationAppInstallation()
	d := schema.TestResourceDataRaw(t, r.Schema, map[string]any{
		"enterprise_slug":      enterpriseSlug,
		"organization":         organization,
		"client_id":            clientID,
		"repository_selection": enterpriseAppInstallationRepositorySelectionNone,
	})
	meta := &Owner{v3client: mustCreateTestGitHubClient(t, server.URL+"/"), maxPerPage: 100}

	if diags := resourceGithubEnterpriseOrganizationAppInstallationCreate(t.Context(), d, meta); diags.HasError() {
		t.Fatalf("create diagnostics: %v", diags)
	}
	if got, want := d.Id(), "example:example-org:Iv1.example"; got != want {
		t.Fatalf("resource ID = %q, want %q", got, want)
	}

	if diags := resourceGithubEnterpriseOrganizationAppInstallationRead(t.Context(), d, meta); diags.HasError() {
		t.Fatalf("read diagnostics: %v", diags)
	}
	if got := d.Get("repository_selection"); got != enterpriseAppInstallationRepositorySelectionNone {
		t.Errorf("repository_selection after read = %q, want %q", got, enterpriseAppInstallationRepositorySelectionNone)
	}
	if got := d.Get("selected_repositories").(*schema.Set).Len(); got != 0 {
		t.Errorf("selected_repositories length after read = %d, want 0", got)
	}

	updateData := schema.TestResourceDataRaw(t, r.Schema, map[string]any{
		"enterprise_slug":      enterpriseSlug,
		"organization":         organization,
		"client_id":            clientID,
		"repository_selection": enterpriseAppInstallationRepositorySelectionNone,
		"installation_id":      int(installationID),
	})
	updateData.SetId(d.Id())
	if diags := resourceGithubEnterpriseOrganizationAppInstallationUpdate(t.Context(), updateData, meta); diags.HasError() {
		t.Fatalf("update diagnostics: %v", diags)
	}
	if installRequests != 2 {
		t.Errorf("install request count after update = %d, want 2", installRequests)
	}
}

func TestAccGithubEnterpriseOrganizationAppInstallation(t *testing.T) {
	t.Parallel()

	skipUnlessEnterprise(t)
	skipUnlessHasEnterpriseAppClient(t)

	const resourceName = "github_enterprise_organization_app_installation.test"

	allConfig := `
resource "github_enterprise_organization_app_installation" "test" {
	enterprise_slug      = "%s"
	organization         = "%s"
	client_id            = "%s"
	repository_selection = "all"
}
`

	selectedConfig := `
resource "github_enterprise_organization_app_installation" "test" {
	enterprise_slug       = "%s"
	organization          = "%s"
	client_id             = "%s"
	repository_selection  = "selected"
	selected_repositories = %s
}
`

	t.Run("installs an app with access to every repository", func(t *testing.T) {
		resource.Test(t, resource.TestCase{
			ProviderFactories: providerFactories,
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(allConfig, testAccConf.enterpriseSlug, testAccConf.meta.name, testAccConf.enterpriseAppClient),
					ConfigStateChecks: []statecheck.StateCheck{
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("repository_selection"), knownvalue.StringExact("all")),
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("selected_repositories"), knownvalue.SetSizeExact(0)),
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("app_slug"), knownvalue.NotNull()),
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

	t.Run("restricts an existing installation to selected repositories", func(t *testing.T) {
		repo := mustCreateTestRepository(t)

		resource.Test(t, resource.TestCase{
			ProviderFactories: providerFactories,
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(allConfig, testAccConf.enterpriseSlug, testAccConf.meta.name, testAccConf.enterpriseAppClient),
					ConfigStateChecks: []statecheck.StateCheck{
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("repository_selection"), knownvalue.StringExact("all")),
					},
				},
				{
					Config: fmt.Sprintf(selectedConfig, testAccConf.enterpriseSlug, testAccConf.meta.name, testAccConf.enterpriseAppClient, fmt.Sprintf(`["%s"]`, repo.GetName())),
					ConfigStateChecks: []statecheck.StateCheck{
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("repository_selection"), knownvalue.StringExact("selected")),
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("selected_repositories"), knownvalue.SetExact([]knownvalue.Check{
							knownvalue.StringExact(repo.GetName()),
						})),
					},
				},
			},
		})
	})

	t.Run("swaps the selected repositories of an installation", func(t *testing.T) {
		// Exercises the grant-before-revoke path in Update, which the API requires because it
		// refuses to remove the last repository from an installation.
		first := mustCreateTestRepository(t)
		second := mustCreateTestRepository(t)

		resource.Test(t, resource.TestCase{
			ProviderFactories: providerFactories,
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(selectedConfig, testAccConf.enterpriseSlug, testAccConf.meta.name, testAccConf.enterpriseAppClient, fmt.Sprintf(`["%s"]`, first.GetName())),
					ConfigStateChecks: []statecheck.StateCheck{
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("selected_repositories"), knownvalue.SetExact([]knownvalue.Check{
							knownvalue.StringExact(first.GetName()),
						})),
					},
				},
				{
					Config: fmt.Sprintf(selectedConfig, testAccConf.enterpriseSlug, testAccConf.meta.name, testAccConf.enterpriseAppClient, fmt.Sprintf(`["%s"]`, second.GetName())),
					ConfigStateChecks: []statecheck.StateCheck{
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("selected_repositories"), knownvalue.SetExact([]knownvalue.Check{
							knownvalue.StringExact(second.GetName()),
						})),
					},
				},
			},
		})
	})

	t.Run("reinstalls an app uninstalled outside terraform", func(t *testing.T) {
		config := fmt.Sprintf(allConfig, testAccConf.enterpriseSlug, testAccConf.meta.name, testAccConf.enterpriseAppClient)

		resource.Test(t, resource.TestCase{
			ProviderFactories: providerFactories,
			Steps: []resource.TestStep{
				{
					Config: config,
					ConfigStateChecks: []statecheck.StateCheck{
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("installation_id"), knownvalue.NotNull()),
					},
				},
				{
					PreConfig: func() {
						mustUninstallEnterpriseOrganizationApp(t, testAccConf.meta.name, testAccConf.enterpriseAppClient)
					},
					Config: config,
					ConfigStateChecks: []statecheck.StateCheck{
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("installation_id"), knownvalue.NotNull()),
					},
				},
			},
		})
	})

	t.Run("rejects a selection that disagrees with the repository list", func(t *testing.T) {
		resource.Test(t, resource.TestCase{
			ProviderFactories: providerFactories,
			Steps: []resource.TestStep{
				{
					Config:      fmt.Sprintf(selectedConfig, testAccConf.enterpriseSlug, testAccConf.meta.name, testAccConf.enterpriseAppClient, `[]`),
					ExpectError: regexp.MustCompile("`selected_repositories` must not be empty when `repository_selection` is \"selected\""),
				},
			},
		})
	})
}

func TestValidateEnterpriseAppInstallationRepositorySelection(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		selection string
		count     int
		wantErr   bool
	}{
		"all with no repositories":          {selection: "all", count: 0, wantErr: false},
		"all with repositories":             {selection: "all", count: 1, wantErr: true},
		"selected with repositories":        {selection: "selected", count: 1, wantErr: false},
		"selected with many repositories":   {selection: "selected", count: 90, wantErr: false},
		"selected without any repository":   {selection: "selected", count: 0, wantErr: true},
		"none without any repository":       {selection: "none", count: 0, wantErr: false},
		"none with repositories":            {selection: "none", count: 1, wantErr: true},
		"unset selection is treated as all": {selection: "", count: 0, wantErr: false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := validateEnterpriseAppInstallationRepositorySelection(tc.selection, tc.count)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateEnterpriseAppInstallationRepositorySelection(%q, %d) error = %v, wantErr %v", tc.selection, tc.count, err, tc.wantErr)
			}
		})
	}
}

func TestNormalizeEnterpriseAppInstallationRepositorySelection(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		selection     string
		selectedCount int
		want          string
	}{
		"all":                         {selection: "all", selectedCount: 0, want: "all"},
		"selected with repository":    {selection: "selected", selectedCount: 1, want: "selected"},
		"selected without repository": {selection: "selected", selectedCount: 0, want: "none"},
		"none":                        {selection: "none", selectedCount: 0, want: "none"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := normalizeEnterpriseAppInstallationRepositorySelection(tc.selection, tc.selectedCount)
			if got != tc.want {
				t.Errorf("normalizeEnterpriseAppInstallationRepositorySelection(%q, %d) = %q, want %q", tc.selection, tc.selectedCount, got, tc.want)
			}
		})
	}
}

func TestSplitEnterpriseAppInstallationRepositories(t *testing.T) {
	t.Parallel()

	// The install and toggle endpoints accept at most 50 repositories in one request; anything
	// beyond that has to be granted separately, so the split has to be exact at the boundary.
	repositories := func(count int) []string {
		names := make([]string, 0, count)
		for i := range count {
			names = append(names, fmt.Sprintf("repo-%d", i))
		}

		return names
	}

	tests := map[string]struct {
		in            []string
		wantInitial   int
		wantRemainder int
	}{
		"none":              {in: nil, wantInitial: 0, wantRemainder: 0},
		"under the limit":   {in: repositories(1), wantInitial: 1, wantRemainder: 0},
		"exactly the limit": {in: repositories(50), wantInitial: 50, wantRemainder: 0},
		"one over":          {in: repositories(51), wantInitial: 50, wantRemainder: 1},
		"two batches over":  {in: repositories(150), wantInitial: 50, wantRemainder: 100},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			initial, remainder := splitEnterpriseAppInstallationRepositories(tc.in)

			if len(initial) != tc.wantInitial {
				t.Errorf("initial length = %d, want %d", len(initial), tc.wantInitial)
			}

			if len(remainder) != tc.wantRemainder {
				t.Errorf("remainder length = %d, want %d", len(remainder), tc.wantRemainder)
			}

			// Every repository has to end up in exactly one of the two batches, in order.
			got := slices.Concat(initial, remainder)
			if !slices.Equal(got, tc.in) {
				t.Errorf("initial + remainder = %v, want %v", got, tc.in)
			}
		})
	}
}

func TestEnterpriseAppInstallationRepositories(t *testing.T) {
	t.Parallel()

	// Repository names are passed through with their configured casing, unlike the sets of logins
	// and slugs elsewhere in the enterprise resources.
	d := schema.TestResourceDataRaw(t, resourceGithubEnterpriseOrganizationAppInstallation().Schema, map[string]any{
		"selected_repositories": []any{"MixedCase", ""},
	})

	got := enterpriseAppInstallationRepositories(d)
	if !slices.Equal(got, []string{"MixedCase"}) {
		t.Errorf("enterpriseAppInstallationRepositories() = %v, want [MixedCase]", got)
	}
}
