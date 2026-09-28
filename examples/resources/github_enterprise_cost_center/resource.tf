resource "github_enterprise_cost_center" "engineering" {
  enterprise_slug        = "my-enterprise"
  name                   = "Engineering"
  ai_credit_pool_enabled = true
}
