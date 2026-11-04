package github

import "github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

func resourceGithubActionsEnterprisePolicy() *schema.Resource {
	return resourceGithubActionsPolicy(actionsPolicyEnterprise)
}
