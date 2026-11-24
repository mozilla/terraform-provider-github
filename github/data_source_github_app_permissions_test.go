package github

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/integrations/terraform-provider-github/v6/internal/githubapppermissions"
)

func TestDataSourceGithubAppPermissionsRead(t *testing.T) {
	t.Parallel()

	dataSource := NewProvider("test", "none")().DataSourcesMap["github_app_permissions"]
	if dataSource == nil {
		t.Fatal("github_app_permissions is missing from the provider")
	}
	d := schema.TestResourceDataRaw(t, dataSource.Schema, nil)
	if diags := dataSource.ReadContext(t.Context(), d, nil); diags.HasError() {
		t.Fatalf("read diagnostics: %v", diags)
	}
	if got := d.Id(); got != "github-app-permissions" {
		t.Errorf("data source ID = %q, want github-app-permissions", got)
	}

	got := d.Get("permission_scopes").(map[string]any)
	want := githubapppermissions.Scopes()
	if len(got) != len(want) {
		t.Errorf("permission_scopes contains %d entries, want %d", len(got), len(want))
	}
	for permission, scope := range want {
		if got[permission] != string(scope) {
			t.Errorf("permission_scopes[%q] = %v, want %q", permission, got[permission], scope)
		}
	}
}
