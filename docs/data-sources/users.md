---
page_title: "github_users (Data Source) - GitHub"
description: |-
  Get information about multiple GitHub users.
---

# github_users (Data Source)

Use this data source to retrieve information about multiple GitHub users at once.

## Example Usage

```terraform
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
```

## Argument Reference

- `usernames` - (Optional) List of usernames. Exactly one of `usernames` or `user_ids` must be specified.
- `user_ids` - (Optional) List of positive numeric GitHub user IDs (not Node IDs). Exactly one of `usernames` or `user_ids` must be specified. Each ID requires a separate REST API request.

Either input accepts an empty list. Results follow the input order, omitting users that could not be found.

## Attributes Reference

- `node_ids` - list of Node IDs of users that could be found.
- `logins` - list of logins of users that could be found.
- `emails` - list of the user's publicly visible profile email (will be empty string in case if user decided not to show it).
- `unknown_logins` - list of logins without matching user when querying by `usernames`; empty when querying by `user_ids`.
- `unknown_user_ids` - list of numeric IDs without matching user when querying by `user_ids`; empty when querying by `usernames`.
