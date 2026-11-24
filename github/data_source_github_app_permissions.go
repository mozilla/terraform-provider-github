package github

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/integrations/terraform-provider-github/v6/internal/githubapppermissions"
)

func dataSourceGithubAppPermissions() *schema.Resource {
	return &schema.Resource{
		Description: "Look up GitHub App permission scopes using the GitHub Enterprise Cloud catalog bundled with the provider. The data source makes no GitHub API requests.",
		ReadContext: dataSourceGithubAppPermissionsRead,
		Schema: map[string]*schema.Schema{
			"permission_scopes": {
				Type:        schema.TypeMap,
				Computed:    true,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Description: "The scope of each permission, keyed by GitHub's permission name. Values are `repository`, `organization`, `enterprise`, or `user`.",
			},
		},
	}
}

func dataSourceGithubAppPermissionsRead(_ context.Context, d *schema.ResourceData, _ any) diag.Diagnostics {
	if err := d.Set("permission_scopes", githubapppermissions.Scopes()); err != nil {
		return diag.FromErr(err)
	}
	d.SetId("github-app-permissions")
	return nil
}
