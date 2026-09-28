resource "github_enterprise_cost_center" "engineering" {
  enterprise_slug = "my-enterprise"
  name            = "Engineering"
}

resource "github_enterprise_cost_center_resources" "engineering" {
  enterprise_slug = "my-enterprise"
  cost_center_id  = github_enterprise_cost_center.engineering.cost_center_id

  users            = ["octocat"]
  enterprise_teams = ["ent:platform"]
}
