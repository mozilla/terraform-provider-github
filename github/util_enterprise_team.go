package github

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-github/v89/github"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// findEnterpriseTeamByID returns the enterprise team with the given numeric ID, or nil when the
// enterprise has no such team. The API offers no get-by-ID endpoint, so this scans every team in
// the enterprise; prefer Enterprise.GetTeam whenever a slug is available.
func findEnterpriseTeamByID(ctx context.Context, client *github.Client, maxPerPage int, enterpriseSlug string, id int64) (*github.EnterpriseTeam, error) {
	for team, err := range client.Enterprise.ListTeamsIter(ctx, enterpriseSlug, &github.ListOptions{PerPage: maxPerPage}) {
		if err != nil {
			return nil, fmt.Errorf("could not list teams for enterprise %q: %w", enterpriseSlug, err)
		}

		if team.ID == id {
			return team, nil
		}
	}

	return nil, nil
}

// listEnterpriseTeamMembers returns the logins of every member of an enterprise team.
func listEnterpriseTeamMembers(ctx context.Context, client *github.Client, maxPerPage int, enterpriseSlug, teamSlug string) ([]string, error) {
	logins := make([]string, 0)

	for user, err := range client.Enterprise.ListTeamMembersIter(ctx, enterpriseSlug, teamSlug, &github.ListOptions{PerPage: maxPerPage}) {
		if err != nil {
			return nil, fmt.Errorf("could not list members of enterprise team %q: %w", teamSlug, err)
		}

		if login := user.GetLogin(); login != "" {
			logins = append(logins, login)
		}
	}

	return logins, nil
}

// listEnterpriseTeamOrganizations returns the lowercased logins of every organization an
// enterprise team is assigned to.
func listEnterpriseTeamOrganizations(ctx context.Context, client *github.Client, maxPerPage int, enterpriseSlug, teamSlug string) ([]string, error) {
	logins := make([]string, 0)

	for org, err := range client.Enterprise.ListAssignmentsIter(ctx, enterpriseSlug, teamSlug, &github.ListOptions{PerPage: maxPerPage}) {
		if err != nil {
			return nil, fmt.Errorf("could not list organizations assigned to enterprise team %q: %w", teamSlug, err)
		}

		if login := org.GetLogin(); login != "" {
			logins = append(logins, strings.ToLower(login))
		}
	}

	return logins, nil
}

// caseInsensitiveHash hashes set elements by their lowercased value so that differences in casing
// between configuration and the API do not produce spurious diffs. GitHub logins and slugs are
// case-insensitive.
func caseInsensitiveHash(v any) int {
	s, _ := v.(string)
	return schema.HashString(strings.ToLower(s))
}
