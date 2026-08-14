package github

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccGithubEnterpriseTeam(t *testing.T) {
	t.Parallel()

	skipUnlessEnterprise(t)

	const resourceName = "github_enterprise_team.test"

	config := `
resource "github_enterprise_team" "test" {
	enterprise_slug             = "%s"
	name                        = "%s"
	description                 = "%s"
	organization_selection_type = "%s"
}
`

	t.Run("creates and updates a team", func(t *testing.T) {
		t.Parallel()

		name := fmt.Sprintf("%s%s", testResourcePrefix, acctest.RandString(testRandomIDLength))
		updatedName := name + "-updated"

		resource.Test(t, resource.TestCase{
			ProviderFactories: providerFactories,
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(config, testAccConf.enterpriseSlug, name, "initial description", "disabled"),
					ConfigStateChecks: []statecheck.StateCheck{
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("name"), knownvalue.StringExact(name)),
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("description"), knownvalue.StringExact("initial description")),
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("organization_selection_type"), knownvalue.StringExact("disabled")),
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("slug"), knownvalue.NotNull()),
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("team_id"), knownvalue.NotNull()),
					},
				},
				{
					// Renaming the team updates it in place and recomputes the slug.
					Config: fmt.Sprintf(config, testAccConf.enterpriseSlug, updatedName, "updated description", "selected"),
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PreApply: []plancheck.PlanCheck{
							plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionUpdate),
						},
					},
					ConfigStateChecks: []statecheck.StateCheck{
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("name"), knownvalue.StringExact(updatedName)),
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("description"), knownvalue.StringExact("updated description")),
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("organization_selection_type"), knownvalue.StringExact("selected")),
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

	t.Run("clears the description", func(t *testing.T) {
		t.Parallel()

		name := fmt.Sprintf("%s%s", testResourcePrefix, acctest.RandString(testRandomIDLength))

		emptyDescriptionConfig := `
resource "github_enterprise_team" "test" {
	enterprise_slug = "%s"
	name            = "%s"
}
`

		resource.Test(t, resource.TestCase{
			ProviderFactories: providerFactories,
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(config, testAccConf.enterpriseSlug, name, "a description to remove", "disabled"),
					ConfigStateChecks: []statecheck.StateCheck{
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("description"), knownvalue.StringExact("a description to remove")),
					},
				},
				{
					// Dropping description from the config must actually clear it on GitHub. An
					// update that omits the field instead of sending an empty one leaves the old
					// value in place and the next plan is non-empty.
					Config: fmt.Sprintf(emptyDescriptionConfig, testAccConf.enterpriseSlug, name),
					ConfigStateChecks: []statecheck.StateCheck{
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("description"), knownvalue.StringExact("")),
					},
				},
			},
		})
	})

	t.Run("recreates the team when it is deleted out of band", func(t *testing.T) {
		t.Parallel()

		name := fmt.Sprintf("%s%s", testResourcePrefix, acctest.RandString(testRandomIDLength))

		resource.Test(t, resource.TestCase{
			ProviderFactories: providerFactories,
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(config, testAccConf.enterpriseSlug, name, "description", "disabled"),
				},
				{
					PreConfig: func() {
						mustDeleteEnterpriseTeamByName(t, name)
					},
					Config: fmt.Sprintf(config, testAccConf.enterpriseSlug, name, "description", "disabled"),
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PreApply: []plancheck.PlanCheck{
							plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionCreate),
						},
					},
				},
			},
		})
	})
}
