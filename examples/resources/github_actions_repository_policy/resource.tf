resource "github_actions_repository_policy" "example" {
  repository  = "example"
  name        = "Workflow security"
  enforcement = "evaluate"

  conditions {
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
