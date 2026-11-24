// Package githubapppermissions looks up the scope of GitHub App permission keys.
package githubapppermissions

//go:generate go run generate.go

// Scope identifies the account or resource a permission applies to.
type Scope string

const (
	Enterprise   Scope = "enterprise"
	Organization Scope = "organization"
	Repository   Scope = "repository"
	User         Scope = "user"
)

// ScopeFor returns the scope for a permission key and whether it is in the catalog.
func ScopeFor(permission string) (Scope, bool) {
	scope, ok := permissionScopes[permission]
	return scope, ok
}
