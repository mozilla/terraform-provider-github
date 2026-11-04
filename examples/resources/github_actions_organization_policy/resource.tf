resource "github_actions_organization_policy" "example" {
  name        = "Workflow security"
  enforcement = "evaluate"

  conditions {
    repository_name {
      include = ["service-*"]
      exclude = ["service-legacy"]
    }
    workflow_path {
      include = [".github/workflows/*.yml"]
      exclude = [".github/workflows/legacy.yml"]
    }
  }

  rules {
    require_lockfile = true

    restrict_actions_actors {
      allowed_actors {
        id   = 5
        type = "RepositoryRole"
      }
      allowed_actors {
        id   = 12345
        type = "App"
      }
    }

    restrict_action_events {
      allowed_events = ["push", "pull_request", "workflow_dispatch"]
    }
  }
}

# Target repositories by numeric ID instead of name.
resource "github_actions_organization_policy" "selected_repositories" {
  name        = "Selected repository lockfiles"
  enforcement = "disabled"

  conditions {
    repository_id {
      repository_ids = [123456789]
    }
  }

  rules {
    require_lockfile = true
  }
}

# Use system or custom repository properties for dynamic targeting.
resource "github_actions_organization_policy" "repository_properties" {
  name        = "Private repository lockfiles"
  enforcement = "disabled"

  conditions {
    repository_property {
      include {
        name            = "visibility"
        source          = "system"
        property_values = ["private"]
      }
      exclude {
        name            = "tier"
        source          = "custom"
        property_values = ["sandbox"]
      }
    }
  }

  rules {
    require_lockfile = true
  }
}
