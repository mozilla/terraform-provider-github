package github

import (
	"testing"

	"github.com/google/go-github/v89/github"
)

// mustUninstallEnterpriseOrganizationApp uninstalls the app with the given client ID from an
// enterprise owned organization, resolving the installation ID first. Used to simulate an app being
// uninstalled outside Terraform.
func mustUninstallEnterpriseOrganizationApp(t *testing.T, organization, clientID string) {
	t.Helper()

	opts := &github.ListOptions{PerPage: testAccConf.meta.maxPerPage}

	for installation, err := range testAccConf.meta.v3client.Enterprise.ListAppInstallationsIter(t.Context(), testAccConf.enterpriseSlug, organization, opts) {
		if err != nil {
			t.Fatalf("failed to list app installations for organization %s: %v", organization, err)
		}

		if installation.GetClientID() != clientID {
			continue
		}

		if _, err := testAccConf.meta.v3client.Enterprise.UninstallApp(t.Context(), testAccConf.enterpriseSlug, organization, installation.GetID()); err != nil {
			t.Fatalf("failed to uninstall app %s from organization %s: %v", clientID, organization, err)
		}

		return
	}

	t.Fatalf("could not find an installation of app %s on organization %s to uninstall", clientID, organization)
}
