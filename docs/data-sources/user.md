---
page_title: "github_user (Data Source) - GitHub"
description: |-
  Get information on a GitHub user.
---

# github_user (Data Source)

Use this data source to retrieve information about a GitHub user.

## Example Usage

```terraform
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
```

## Argument Reference

- `username` - (Optional) The username. Use an empty string `""` to retrieve information about the currently authenticated user. Exactly one of `username` or `user_id` must be specified.
- `user_id` - (Optional) The positive numeric GitHub user ID (not the Node ID). Looks up the user's current login and profile. Exactly one of `username` or `user_id` must be specified.

## Attributes Reference

- `id` - the ID of the user.
- `node_id` - the Node ID of the user.
- `login` - the user's login.
- `avatar_url` - the user's avatar URL.
- `gravatar_id` - the user's gravatar ID.
- `site_admin` - whether the user is a GitHub admin.
- `name` - the user's full name.
- `company` - the user's company name.
- `blog` - the user's blog location.
- `location` - the user's location.
- `email` - the user's email.
- `gpg_keys` - list of user's GPG keys.
- `ssh_keys` - list of user's SSH keys.
- `bio` - the user's bio.
- `public_repos` - the number of public repositories.
- `public_gists` - the number of public gists.
- `followers` - the number of followers.
- `following` - the number of following users.
- `created_at` - the creation date.
- `updated_at` - the update date.
- `suspended_at` - the suspended date if the user is suspended.
