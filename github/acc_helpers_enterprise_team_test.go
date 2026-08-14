package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/go-github/v92/github"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
)

type createTestEnterpriseTeamOptionsFunc func(*github.EnterpriseTeamCreateOrUpdateRequest)

// withEnterpriseTeamOrganizationSelection sets the organization_selection_type of a test team.
// Assigning a team to organizations requires "selected".
func withEnterpriseTeamOrganizationSelection(selection string) createTestEnterpriseTeamOptionsFunc {
	return func(team *github.EnterpriseTeamCreateOrUpdateRequest) {
		team.OrganizationSelectionType = new(selection)
	}
}

// mustCreateTestEnterpriseTeam creates an enterprise team named with testResourcePrefix and
// registers its deletion for the end of the test.
func mustCreateTestEnterpriseTeam(t *testing.T, f ...createTestEnterpriseTeamOptionsFunc) *github.EnterpriseTeam {
	t.Helper()

	name := fmt.Sprintf("%s%s", testResourcePrefix, acctest.RandString(testRandomIDLength))

	req := github.EnterpriseTeamCreateOrUpdateRequest{
		Name:        name,
		Description: new("Created by an acceptance test."),
	}

	for _, fn := range f {
		if fn != nil {
			fn(&req)
		}
	}

	team, _, err := testAccConf.meta.v3client.Enterprise.CreateTeam(t.Context(), testAccConf.enterpriseSlug, req)
	if err != nil {
		t.Fatalf("failed to create test enterprise team: %v", err)
	}

	t.Cleanup(func() {
		if _, err := testAccConf.meta.v3client.Enterprise.DeleteTeam(context.Background(), testAccConf.enterpriseSlug, team.Slug); err != nil {
			if err, ok := errors.AsType[*github.ErrorResponse](err); ok && err.Response.StatusCode == http.StatusNotFound {
				return
			}

			t.Logf("failed to delete test enterprise team %s: %v", name, err)
		}
	})

	return team
}

// mustDeleteEnterpriseTeamByName deletes the enterprise team with the given name, resolving its
// slug first. Used to simulate a team being deleted outside Terraform.
func mustDeleteEnterpriseTeamByName(t *testing.T, name string) {
	t.Helper()

	opts := &github.ListOptions{PerPage: testAccConf.meta.maxPerPage}

	for team, err := range testAccConf.meta.v3client.Enterprise.ListTeamsIter(t.Context(), testAccConf.enterpriseSlug, opts) {
		if err != nil {
			t.Fatalf("failed to list enterprise teams: %v", err)
		}

		if team.Name != name {
			continue
		}

		if _, err := testAccConf.meta.v3client.Enterprise.DeleteTeam(t.Context(), testAccConf.enterpriseSlug, team.Slug); err != nil {
			t.Fatalf("failed to delete test enterprise team %s: %v", name, err)
		}

		return
	}

	t.Fatalf("could not find enterprise team %s to delete", name)
}

// mustAddEnterpriseTeamMembers adds members to an enterprise team out of band.
func mustAddEnterpriseTeamMembers(t *testing.T, team *github.EnterpriseTeam, usernames ...string) {
	t.Helper()

	if _, _, err := testAccConf.meta.v3client.Enterprise.BulkAddTeamMembers(t.Context(), testAccConf.enterpriseSlug, team.Slug, usernames); err != nil {
		t.Fatalf("failed to add members %v to test enterprise team %s: %v", usernames, team.Name, err)
	}
}
