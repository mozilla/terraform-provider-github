resource "github_enterprise_team" "platform" {
  enterprise_slug = "my-enterprise"
  name            = "Platform"
}

resource "github_enterprise_team_members" "platform" {
  enterprise_slug = "my-enterprise"
  team_slug       = github_enterprise_team.platform.slug

  members = ["alice", "bob", "carol"]
}
