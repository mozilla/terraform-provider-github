resource "github_enterprise_team" "platform" {
  enterprise_slug = "my-enterprise"
  name            = "Platform"
  description     = "Engineers who maintain shared platform services."

  # Required before the team can be assigned to organizations with
  # github_enterprise_team_organizations.
  organization_selection_type = "selected"
}

# Membership can instead be synchronised from an identity provider group.
resource "github_enterprise_team" "security" {
  enterprise_slug = "my-enterprise"
  name            = "Security"
  group_id        = "8a7b6c5d-4e3f-2a1b-0c9d-8e7f6a5b4c3d"
}
