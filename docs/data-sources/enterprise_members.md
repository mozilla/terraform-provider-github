---
page_title: "github_enterprise_members (Data Source) - GitHub"
subcategory: ""
description: |-
  Data source to list all members of a GitHub enterprise.
---

# github_enterprise_members (Data Source)

Data source to list all members of a GitHub enterprise.

## Example Usage

```terraform
data "github_enterprise_members" "example" {
  enterprise_slug = "example-enterprise"
}
```

<!--
## Schema

### Required

- `enterprise_slug` (String) The slug of the enterprise whose members will be listed.

### Read-Only

- `id` (String) The ID of this resource.
- `members` (List of Object) Enterprise members. (see [below for nested schema](#nestedatt--members))

<a id="nestedatt--members"></a>
### Nested Schema for `members`

Read-Only:

- `id` (Number)
- `login` (String)
- `node_id` (String)
-->

## Schema

### Required

- `enterprise_slug` (String) The slug of the enterprise whose members will be listed.

### Read-Only

- `id` (String) The ID of this resource.
- `members` (List of Object) Enterprise members. (see [below for nested schema](#nestedatt--members))

<a id="nestedatt--members"></a>
### Nested Schema for `members`

Read-Only:

- `id` (Number) Database ID of the member.
- `login` (String) Login of the member.
- `node_id` (String) Node ID of the member.
