package github

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func testAccActionsPolicyLifecycle(t *testing.T, scope actionsPolicyScope) {
	t.Helper()
	switch scope {
	case actionsPolicyRepository:
		skipUnauthenticated(t)
	case actionsPolicyOrganization:
		skipUnlessHasOrgs(t)
	case actionsPolicyEnterprise:
		skipUnlessEnterprise(t)
	}
	repo := mustCreateTestRepository(t)
	name := testResourcePrefix + acctest.RandString(testRandomIDLength)
	resourceName := "github_actions_repository_policy"
	parent := fmt.Sprintf("repository = %q", repo.GetName())
	targets := ""
	if scope != actionsPolicyRepository {
		resourceName = "github_actions_organization_policy"
		parent = ""
		targets = fmt.Sprintf("repository_name { include = [%q] }", repo.GetName())
	}
	if scope == actionsPolicyEnterprise {
		resourceName = "github_actions_enterprise_policy"
		parent = fmt.Sprintf("enterprise_slug = %q", testAccConf.enterpriseSlug)
		targets += fmt.Sprintf("\norganization_name { include = [%q] }", testAccConf.meta.name)
	}
	config := func(suffix, restrictions string) string {
		return fmt.Sprintf(`resource %q "test" {
%s
name = %q
enforcement = "disabled"
conditions {
%s
workflow_path { include = [".github/workflows/*.yml"] }
}
rules {
require_lockfile = true
%s
}
}`, resourceName, parent, name+suffix, targets, restrictions)
	}
	address := resourceName + ".test"
	resource.Test(t, resource.TestCase{
		ProviderFactories: providerFactories,
		Steps: []resource.TestStep{
			{
				Config: config("", `restrict_actions_actors {
allowed_actors {
id = 5
type = "RepositoryRole"
}
}
restrict_action_events { allowed_events = ["push", "pull_request"] }`),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(address, tfjsonpath.New("policy_id"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue(address, tfjsonpath.New("node_id"), knownvalue.NotNull()),
				},
			},
			{ResourceName: address, ImportState: true, ImportStateVerify: true},
			{
				Config: config("-updated", ""),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(address, tfjsonpath.New("name"), knownvalue.StringExact(name+"-updated")),
				},
			},
		},
	})
}
