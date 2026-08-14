# A GitHub App installed at the enterprise level is not scoped to a single
# organization or user account, so `owner` is omitted and the provider
# authenticates directly as the configured installation.
provider "github" {
  auth_mode = "app" # or `GITHUB_AUTH_MODE=app`

  app_auth {
    id              = var.app_id              # or `GITHUB_APP_ID`
    installation_id = var.app_installation_id # or `GITHUB_APP_INSTALLATION_ID`
    pem_file        = var.app_pem_file        # or `GITHUB_APP_PEM_FILE`
  }
}

# Only resources that are not scoped to an owner can be used; these identify the
# enterprise themselves rather than relying on the provider's `owner`.
data "github_enterprise" "example" {
  slug = var.enterprise_slug
}

resource "github_enterprise_actions_permissions" "example" {
  enterprise_slug       = data.github_enterprise.example.slug
  allowed_actions       = "all"
  enabled_organizations = "all"
}
