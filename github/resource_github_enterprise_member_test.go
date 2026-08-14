package github

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccGithubEnterpriseMember(t *testing.T) {
	t.Parallel()

	const resourceName = "github_enterprise_member.test"

	config := `
resource "github_enterprise_member" "test" {
	enterprise_slug = "%s"
	username        = "%s"
}
`

	// These subtests are not parallel: they share the enterprise and the small pool of external
	// test users, so concurrent invitations for the same user would collide.
	t.Run("invites a user to the enterprise", func(t *testing.T) {
		resource.Test(t, resource.TestCase{
			PreCheck: func() {
				skipUnlessEnterprise(t)
				// An EMU enterprise provisions members through its identity provider and cannot
				// invite outside users.
				skipIfEMUEnterprise(t)
				skipUnlessHasExternalUser1(t)
			},
			ProviderFactories: providerFactories,
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(config, testAccConf.enterpriseSlug, testAccConf.testExternalUser1),
					ConfigStateChecks: []statecheck.StateCheck{
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("enterprise_slug"), knownvalue.StringExact(testAccConf.enterpriseSlug)),
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("username"), knownvalue.StringExact(testAccConf.testExternalUser1)),
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("status"), knownvalue.StringExact("pending")),
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("invitation_id"), knownvalue.NotNull()),
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

	t.Run("forces a new resource when the username changes", func(t *testing.T) {
		resource.Test(t, resource.TestCase{
			PreCheck: func() {
				skipUnlessEnterprise(t)
				skipIfEMUEnterprise(t)
				skipUnlessHasExternalUser1(t)
				skipUnlessHasExternalUser2(t)
			},
			ProviderFactories: providerFactories,
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(config, testAccConf.enterpriseSlug, testAccConf.testExternalUser1),
				},
				{
					Config: fmt.Sprintf(config, testAccConf.enterpriseSlug, testAccConf.testExternalUser2),
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PreApply: []plancheck.PlanCheck{
							plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionDestroyBeforeCreate),
						},
					},
					ConfigStateChecks: []statecheck.StateCheck{
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("username"), knownvalue.StringExact(testAccConf.testExternalUser2)),
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("status"), knownvalue.StringExact("pending")),
					},
				},
			},
		})
	})

	t.Run("adopts an invitation that already exists", func(t *testing.T) {
		resource.Test(t, resource.TestCase{
			PreCheck: func() {
				skipUnlessEnterprise(t)
				skipIfEMUEnterprise(t)
				skipUnlessHasExternalUser2(t)
			},
			ProviderFactories: providerFactories,
			Steps: []resource.TestStep{
				{
					// Inviting out of band first means create runs against an invitation that
					// already exists, which the invite mutation on its own would reject.
					PreConfig: func() {
						mustInviteEnterpriseMember(t, testAccConf.enterpriseSlug, testAccConf.testExternalUser2)
					},
					Config: fmt.Sprintf(config, testAccConf.enterpriseSlug, testAccConf.testExternalUser2),
					ConfigStateChecks: []statecheck.StateCheck{
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("status"), knownvalue.StringExact("pending")),
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("invitation_id"), knownvalue.NotNull()),
					},
				},
			},
		})
	})
}
