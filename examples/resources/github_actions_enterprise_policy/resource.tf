resource "github_actions_enterprise_policy" "example" {
  enterprise_slug = "example-enterprise"
  name            = "Workflow security"
  enforcement     = "evaluate"

  conditions {
    organization_name {
      include = ["example-org"]
    }
    repository_name {
      include = ["service-*"]
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

# Select organizations by ID and repositories by property.
resource "github_actions_enterprise_policy" "organization_ids" {
  enterprise_slug = "example-enterprise"
  name            = "Selected organization lockfiles"
  enforcement     = "disabled"

  conditions {
    organization_id {
      organization_ids = [123456]
    }
    repository_property {
      include {
        name            = "visibility"
        source          = "system"
        property_values = ["private"]
      }
    }
  }

  rules {
    require_lockfile = true
  }
}

# Target organizations using enterprise custom properties.
resource "github_actions_enterprise_policy" "organization_properties" {
  enterprise_slug = "example-enterprise"
  name            = "Engineering workflow lockfiles"
  enforcement     = "disabled"

  conditions {
    organization_property {
      include {
        name            = "division"
        property_values = ["engineering"]
      }
      exclude {
        name            = "environment"
        property_values = ["sandbox"]
      }
    }
    repository_name {
      include   = ["~ALL"]
      exclude   = ["legacy-*"]
      protected = true
    }
  }

  rules {
    require_lockfile = true
  }
}
