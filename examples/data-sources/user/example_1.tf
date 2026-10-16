# Retrieve information about a GitHub user.
data "github_user" "example" {
  username = "example"
}

# Retrieve information using a stable numeric GitHub user ID.
data "github_user" "by_id" {
  user_id = 583231
}

output "login_from_id" {
  value = data.github_user.by_id.login
}

# Retrieve information about the currently authenticated user.
data "github_user" "current" {
  username = ""
}

output "current_github_login" {
  value = data.github_user.current.login
}

