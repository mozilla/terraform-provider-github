resource "github_organization_ruleset" "repository_policy" {
  name        = "Protect production repositories"
  target      = "repository"
  enforcement = "active"

  bypass_actors {
    actor_type  = "OrganizationAdmin"
    bypass_mode = "always"
  }

  conditions {
    repository_name {
      include = ["production-*"]
      exclude = []
    }
  }

  rules {
    repository_delete   = true
    repository_transfer = true

    repository_name {
      pattern = "^production-[a-z0-9-]+$"
    }

    repository_visibility {
      public   = false
      internal = false
      private  = true
    }
  }
}
