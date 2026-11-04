package github

import "github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

func resourceGithubActionsOrganizationPolicy() *schema.Resource {
	return resourceGithubActionsPolicy(actionsPolicyOrganization)
}
