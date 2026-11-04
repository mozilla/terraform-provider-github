package github

import "testing"

func TestAccGithubActionsEnterprisePolicy(t *testing.T) {
	t.Parallel()
	t.Run("creates imports updates and deletes a policy", func(t *testing.T) {
		testAccActionsPolicyLifecycle(t, actionsPolicyEnterprise)
	})
}
