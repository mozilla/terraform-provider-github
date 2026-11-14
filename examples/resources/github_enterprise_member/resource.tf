resource "github_enterprise_member" "alice" {
  enterprise_slug = "my-enterprise"
  username        = "alice"
}

# Invitations can be managed for a set of users.
variable "enterprise_members" {
  type    = set(string)
  default = ["bob", "carol"]
}

resource "github_enterprise_member" "members" {
  for_each = var.enterprise_members

  enterprise_slug = "my-enterprise"
  username        = each.value
}

# Prefer a stable account ID when usernames may change.
resource "github_enterprise_member" "by_id" {
  enterprise_slug = "my-enterprise"
  user_id         = 123456
}
