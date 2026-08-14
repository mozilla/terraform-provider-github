package github

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccGithubEnterpriseTeamOrganizations(t *testing.T) {
	t.Parallel()

	skipUnlessEnterprise(t)

	const resourceName = "github_enterprise_team_organizations.test"

	config := `
resource "github_enterprise_team_organizations" "test" {
	enterprise_slug    = "%s"
	team_slug          = "%s"
	organization_slugs = %s
}
`

	t.Run("assigns a team to an organization", func(t *testing.T) {
		t.Parallel()

		// Organization assignment is only permitted for teams whose selection type is "selected".
		team := mustCreateTestEnterpriseTeam(t, withEnterpriseTeamOrganizationSelection("selected"))
		organizations := fmt.Sprintf(`["%s"]`, testAccConf.meta.name)

		resource.Test(t, resource.TestCase{
			ProviderFactories: providerFactories,
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(config, testAccConf.enterpriseSlug, team.Slug, organizations),
					ConfigStateChecks: []statecheck.StateCheck{
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("team_slug"), knownvalue.StringExact(team.Slug)),
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("organization_slugs"), knownvalue.SetSizeExact(1)),
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

	t.Run("unassigns organizations removed from the configuration", func(t *testing.T) {
		t.Parallel()

		team := mustCreateTestEnterpriseTeam(t, withEnterpriseTeamOrganizationSelection("selected"))
		organizations := fmt.Sprintf(`["%s"]`, testAccConf.meta.name)

		resource.Test(t, resource.TestCase{
			ProviderFactories: providerFactories,
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(config, testAccConf.enterpriseSlug, team.Slug, organizations),
					ConfigStateChecks: []statecheck.StateCheck{
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("organization_slugs"), knownvalue.SetSizeExact(1)),
					},
				},
				{
					// Destroying the resource must leave the team assigned to nothing, so the
					// following create starts from a clean slate rather than adopting state.
					Config:  fmt.Sprintf(config, testAccConf.enterpriseSlug, team.Slug, organizations),
					Destroy: true,
				},
			},
		})
	})
}
