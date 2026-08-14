package github

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

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
