package github

import "github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

func resourceGithubActionsRepositoryPolicy() *schema.Resource {
	return resourceGithubActionsPolicy(actionsPolicyRepository)
}
