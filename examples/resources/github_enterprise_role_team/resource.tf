resource "github_enterprise_team" "security" {
  enterprise_slug = "my-enterprise"
  name            = "Security managers"
}

resource "github_enterprise_role_team" "security" {
  enterprise_slug = github_enterprise_team.security.enterprise_slug
  role_id         = 8030
  team_slug       = github_enterprise_team.security.slug
}
