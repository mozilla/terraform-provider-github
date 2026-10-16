# Retrieve information about multiple GitHub users.
data "github_users" "example" {
  usernames = ["example1", "example2", "example3"]
}

output "valid_users" {
  value = data.github_users.example.logins
}

output "invalid_users" {
  value = data.github_users.example.unknown_logins
}

# Resolve current logins from stable numeric GitHub user IDs.
data "github_users" "by_id" {
  user_ids = [583231, 1]
}

output "logins_from_ids" {
  value = data.github_users.by_id.logins
}

output "unknown_user_ids" {
  value = data.github_users.by_id.unknown_user_ids
}
