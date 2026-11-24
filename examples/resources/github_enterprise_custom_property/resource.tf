resource "github_enterprise_custom_property" "compliance" {
  enterprise_slug = "example-enterprise"
  property_name   = "compliance"
  value_type      = "multi_select"
  description     = "Compliance frameworks that apply to a repository"

  required       = true
  allowed_values = ["internal", "soc2", "iso27001"]
  default_values = ["internal"]
}
