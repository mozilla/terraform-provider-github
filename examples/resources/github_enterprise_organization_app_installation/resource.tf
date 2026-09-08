resource "github_enterprise_organization_app_installation" "ci" {
  enterprise_slug = "my-enterprise"
  organization    = "my-organization"
  client_id       = "Iv23liAbCdEfGhIjKlMn"

  repository_selection = "all"
}

# Restrict the installation to an explicit list of repositories instead.
resource "github_enterprise_organization_app_installation" "deploy" {
  enterprise_slug = "my-enterprise"
  organization    = "my-organization"
  client_id       = "Iv23liOpQrStUvWxYzAb"

  repository_selection  = "selected"
  selected_repositories = ["terraform-modules", "deploy-tooling"]
}

# Install an app that does not request any repository permissions.
resource "github_enterprise_organization_app_installation" "organization_runner" {
  enterprise_slug = "my-enterprise"
  organization    = "my-organization"
  client_id       = "Iv23liCdEfGhIjKlMnOp"

  repository_selection = "none"
}
