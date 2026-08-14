resource "github_enterprise_team" "platform" {
  enterprise_slug             = "my-enterprise"
  name                        = "Platform"
  organization_selection_type = "selected"
}

resource "github_enterprise_team_organizations" "platform" {
  enterprise_slug = "my-enterprise"
  team_slug       = github_enterprise_team.platform.slug

  organization_slugs = ["my-org", "my-other-org"]
}
