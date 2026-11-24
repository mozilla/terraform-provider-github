data "github_app_permissions" "catalog" {}

output "actions_scope" {
  value = data.github_app_permissions.catalog.permission_scopes["actions"]
}
