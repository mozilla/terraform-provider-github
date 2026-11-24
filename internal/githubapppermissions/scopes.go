// Package githubapppermissions looks up the scope of GitHub App permission keys.
package githubapppermissions

import "maps"

//go:generate go run generate.go

// Scope identifies the account or resource a permission applies to.
type Scope string

const (
	Enterprise   Scope = "enterprise"
	Organization Scope = "organization"
	Repository   Scope = "repository"
	User         Scope = "user"
)

// supplementalPermissionScopes covers permissions missing from GitHub's REST permissions catalog.
var supplementalPermissionScopes = map[string]Scope{
	// GitHub lists "Agent tasks" as a repository permission for listing tasks.
	// https://docs.github.com/en/rest/agent-tasks/agent-tasks?apiVersion=2026-03-10#list-tasks-for-repository
	"agent_tasks": Repository,

	// GitHub's guide to creating custom organization roles at the enterprise level.
	// https://docs.github.com/en/enterprise-cloud@latest/admin/managing-accounts-and-repositories/managing-roles-in-your-enterprise/create-custom-roles#creating-organization-custom-roles
	"enterprise_custom_org_roles": Enterprise,

	// GitHub's guide to managing users at the enterprise level.
	// https://docs.github.com/en/enterprise-cloud@latest/admin/managing-accounts-and-repositories/managing-users-in-your-enterprise
	"enterprise_people": Enterprise,

	// GitHub lists merge_queues in the installation token request schema.
	// https://docs.github.com/en/enterprise-cloud@latest/rest/apps/apps?apiVersion=2026-03-10#create-an-installation-access-token-for-an-app
	"merge_queues": Repository,

	// GitHub documents "Events" as a user permission for private events.
	// https://docs.github.com/en/rest/activity/events?apiVersion=2026-03-10#list-events-for-the-authenticated-user
	// GitHub lists the user_events key under account permissions.
	// https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens#account-permissions
	"user_events": User,
}

// ScopeFor returns the scope for a permission key and whether it is in the catalog.
func ScopeFor(permission string) (Scope, bool) {
	scope, ok := permissionScopes[permission]
	if !ok {
		scope, ok = supplementalPermissionScopes[permission]
	}
	return scope, ok
}

// Scopes returns a copy of the bundled permission-to-scope mapping.
func Scopes() map[string]Scope {
	scopes := maps.Clone(supplementalPermissionScopes)
	maps.Copy(scopes, permissionScopes)
	return scopes
}
