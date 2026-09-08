package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/google/go-github/v92/github"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

// enterpriseAppInstallationRepositorySelections are the values GitHub accepts when installing an
// app on an enterprise-owned organization.
var enterpriseAppInstallationRepositorySelections = []string{"all", "selected", "none"}

// enterpriseAppInstallationRepositorySelectionSelected is the value that restricts an installation
// to an explicit list of repositories.
const enterpriseAppInstallationRepositorySelectionSelected = "selected"

// enterpriseAppInstallationRepositorySelectionNone is used when an app requests no repository
// permissions.
const enterpriseAppInstallationRepositorySelectionNone = "none"

// maxEnterpriseAppInstallationRepositoriesPerRequest is the number of repositories the enterprise
// organization installation endpoints accept in a single request.
const maxEnterpriseAppInstallationRepositoriesPerRequest = 50

func resourceGithubEnterpriseOrganizationAppInstallation() *schema.Resource {
	return &schema.Resource{
		Description: "Manages the installation of a GitHub App on an organization owned by an " +
			"enterprise, and the repositories that installation can access. The app is installed on " +
			"the organization rather than on the enterprise account itself, because GitHub offers no " +
			"API for the latter.",
		CreateContext: resourceGithubEnterpriseOrganizationAppInstallationCreate,
		ReadContext:   resourceGithubEnterpriseOrganizationAppInstallationRead,
		UpdateContext: resourceGithubEnterpriseOrganizationAppInstallationUpdate,
		DeleteContext: resourceGithubEnterpriseOrganizationAppInstallationDelete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		CustomizeDiff: resourceGithubEnterpriseOrganizationAppInstallationCustomizeDiff,

		Schema: map[string]*schema.Schema{
			"enterprise_slug": {
				Type:             schema.TypeString,
				Required:         true,
				ForceNew:         true,
				Description:      "The slug of the enterprise, as it appears in the enterprise URL.",
				ValidateDiagFunc: validation.ToDiagFunc(validation.StringIsNotWhiteSpace),
			},
			"organization": {
				Type:             schema.TypeString,
				Required:         true,
				ForceNew:         true,
				Description:      "The login of the enterprise owned organization to install the app on.",
				DiffSuppressFunc: caseInsensitive(),
				ValidateDiagFunc: validation.ToDiagFunc(validation.StringIsNotWhiteSpace),
			},
			"client_id": {
				Type:             schema.TypeString,
				Required:         true,
				ForceNew:         true,
				Description:      "The client ID of the GitHub App to install.",
				ValidateDiagFunc: validation.ToDiagFunc(validation.StringIsNotWhiteSpace),
			},
			"repository_selection": {
				Type:     schema.TypeString,
				Required: true,
				Description: "Which repositories the installation can access: `all` for every " +
					"repository in the organization, or `selected` for the explicit list in " +
					"`selected_repositories`, or `none` when the app requests no repository permissions.",
				ValidateDiagFunc: validateValueFunc(enterpriseAppInstallationRepositorySelections),
			},
			"selected_repositories": {
				Type:     schema.TypeSet,
				Optional: true,
				Description: "The names of the repositories the installation can access. Required " +
					"when `repository_selection` is `selected`, and must be empty otherwise.",
				Elem: &schema.Schema{
					Type:             schema.TypeString,
					ValidateDiagFunc: validation.ToDiagFunc(validation.StringIsNotWhiteSpace),
				},
				Set: caseInsensitiveHash,
			},
			"installation_id": {
				Type:        schema.TypeInt,
				Computed:    true,
				Description: "The numeric ID of the app installation.",
			},
			"app_slug": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The slug of the installed app.",
			},
		},
	}
}

// resourceGithubEnterpriseOrganizationAppInstallationCustomizeDiff rejects configurations where
// repository_selection and selected_repositories disagree, which the API would otherwise reject
// at apply time.
func resourceGithubEnterpriseOrganizationAppInstallationCustomizeDiff(_ context.Context, d *schema.ResourceDiff, _ any) error {
	// An unknown value reads back as its zero value, which would fail this check for a
	// configuration that is in fact valid.
	if !d.NewValueKnown("repository_selection") || !d.NewValueKnown("selected_repositories") {
		return nil
	}

	selection, _ := d.Get("repository_selection").(string)

	count := 0
	if set, ok := d.Get("selected_repositories").(*schema.Set); ok {
		count = set.Len()
	}

	return validateEnterpriseAppInstallationRepositorySelection(selection, count)
}

// validateEnterpriseAppInstallationRepositorySelection reports whether a repository selection and
// a count of selected repositories are consistent with one another.
func validateEnterpriseAppInstallationRepositorySelection(selection string, selectedCount int) error {
	if selection == enterpriseAppInstallationRepositorySelectionSelected {
		if selectedCount == 0 {
			return fmt.Errorf("`selected_repositories` must not be empty when `repository_selection` is %q", selection)
		}

		return nil
	}

	if selectedCount > 0 {
		return fmt.Errorf("`selected_repositories` must be empty when `repository_selection` is %q", selection)
	}

	return nil
}

func resourceGithubEnterpriseOrganizationAppInstallationCreate(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
	meta, _ := m.(*Owner)
	client := meta.v3client

	enterpriseSlug, _ := d.Get("enterprise_slug").(string)
	organization, _ := d.Get("organization").(string)
	clientID, _ := d.Get("client_id").(string)

	// The install endpoint only accepts the first batch of repositories; the rest are granted with
	// follow up requests once the installation exists.
	repositories := enterpriseAppInstallationRepositories(d)
	initial, remainder := splitEnterpriseAppInstallationRepositories(repositories)

	req := github.InstallAppRequest{
		ClientID:            clientID,
		RepositorySelection: d.Get("repository_selection").(string),
		Repositories:        initial,
	}

	tflog.Debug(ctx, "Installing app on enterprise owned organization.", map[string]any{
		"enterprise_slug": enterpriseSlug,
		"organization":    organization,
		"client_id":       clientID,
	})

	installation, _, err := client.Enterprise.InstallApp(ctx, enterpriseSlug, organization, req)
	if err != nil {
		return diag.FromErr(err)
	}

	id, err := buildID(enterpriseSlug, organization, clientID)
	if err != nil {
		return diag.FromErr(err)
	}

	// Record the installation before granting the remaining repositories, so that a failure part
	// way through leaves a resource that can be repaired rather than an installation Terraform has
	// no reference to.
	d.SetId(id)

	if err := d.Set("installation_id", installation.GetID()); err != nil {
		return diag.FromErr(err)
	}

	if err := d.Set("app_slug", installation.GetAppSlug()); err != nil {
		return diag.FromErr(err)
	}

	if err := addEnterpriseAppInstallationRepositories(ctx, client, enterpriseSlug, organization, installation.GetID(), remainder); err != nil {
		return diag.FromErr(err)
	}

	return nil
}

func resourceGithubEnterpriseOrganizationAppInstallationRead(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
	meta, _ := m.(*Owner)
	client := meta.v3client

	enterpriseSlug, organization, clientID, err := parseID3(d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	// A 404 means the organization or the enterprise is gone, which is as good as the installation
	// being gone as far as this resource is concerned.
	installation, err := findEnterpriseAppInstallationByClientID(ctx, client, meta.maxPerPage, enterpriseSlug, organization, clientID)
	if err != nil {
		if !isEnterpriseAppInstallationNotFound(err) {
			return diag.FromErr(err)
		}

		installation = nil
	}

	if installation == nil {
		tflog.Info(ctx, "Removing enterprise organization app installation from state because it no longer exists.", map[string]any{
			"enterprise_slug": enterpriseSlug,
			"organization":    organization,
			"client_id":       clientID,
		})
		d.SetId("")

		return nil
	}

	// The API only reports the selected repositories for an installation that is restricted to
	// them; for an "all" installation the list endpoint is not meaningful.
	selectedRepositories := make([]string, 0)

	if installation.GetRepositorySelection() == enterpriseAppInstallationRepositorySelectionSelected {
		selectedRepositories, err = listEnterpriseAppInstallationRepositories(ctx, client, meta.maxPerPage, enterpriseSlug, organization, installation.GetID())
		if err != nil {
			return diag.FromErr(err)
		}
	}

	repositorySelection := normalizeEnterpriseAppInstallationRepositorySelection(
		installation.GetRepositorySelection(),
		len(selectedRepositories),
	)

	fields := map[string]any{
		"enterprise_slug":       enterpriseSlug,
		"organization":          organization,
		"client_id":             clientID,
		"repository_selection":  repositorySelection,
		"selected_repositories": selectedRepositories,
		"installation_id":       installation.GetID(),
		"app_slug":              installation.GetAppSlug(),
	}

	for key, value := range fields {
		if err := d.Set(key, value); err != nil {
			return diag.FromErr(err)
		}
	}

	return nil
}

func resourceGithubEnterpriseOrganizationAppInstallationUpdate(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
	meta, _ := m.(*Owner)
	client := meta.v3client

	enterpriseSlug, organization, _, err := parseID3(d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	installationID, _ := d.Get("installation_id").(int)

	// Changing repository_selection replaces the whole selection, so it also covers any change to
	// selected_repositories made in the same apply.
	if d.HasChange("repository_selection") {
		selection, _ := d.Get("repository_selection").(string)
		clientID, _ := d.Get("client_id").(string)

		var remainder []string
		if selection == enterpriseAppInstallationRepositorySelectionNone {
			tflog.Debug(ctx, "Updating enterprise organization app installation with no repository permissions.", map[string]any{
				"enterprise_slug":      enterpriseSlug,
				"organization":         organization,
				"installation_id":      installationID,
				"repository_selection": selection,
			})

			req := github.InstallAppRequest{
				ClientID:            clientID,
				RepositorySelection: selection,
			}
			if _, _, err := client.Enterprise.InstallApp(ctx, enterpriseSlug, organization, req); err != nil {
				return diag.FromErr(err)
			}

			return nil
		}

		req := github.UpdateAppInstallationRepositoriesRequest{
			RepositorySelection: new(selection),
		}

		if selection == enterpriseAppInstallationRepositorySelectionSelected {
			req.Repositories, remainder = splitEnterpriseAppInstallationRepositories(enterpriseAppInstallationRepositories(d))
		}

		tflog.Debug(ctx, "Updating repository selection of enterprise organization app installation.", map[string]any{
			"enterprise_slug":      enterpriseSlug,
			"organization":         organization,
			"installation_id":      installationID,
			"repository_selection": selection,
		})

		if _, _, err := client.Enterprise.UpdateAppInstallationRepositories(ctx, enterpriseSlug, organization, int64(installationID), req); err != nil {
			return diag.FromErr(err)
		}

		if err := addEnterpriseAppInstallationRepositories(ctx, client, enterpriseSlug, organization, int64(installationID), remainder); err != nil {
			return diag.FromErr(err)
		}

		return nil
	}

	if d.HasChange("selected_repositories") {
		oldValue, newValue := d.GetChange("selected_repositories")

		oldSet := enterpriseAppInstallationRepositorySet(oldValue)
		newSet := enterpriseAppInstallationRepositorySet(newValue)

		// Grant before revoking. The API refuses to remove the last repository from an
		// installation, so shrinking and growing the selection in the other order can fail.
		add := expandStringList(newSet.Difference(oldSet).List())
		if err := addEnterpriseAppInstallationRepositories(ctx, client, enterpriseSlug, organization, int64(installationID), add); err != nil {
			return diag.FromErr(err)
		}

		remove := expandStringList(oldSet.Difference(newSet).List())
		if err := removeEnterpriseAppInstallationRepositories(ctx, client, enterpriseSlug, organization, int64(installationID), remove); err != nil {
			return diag.FromErr(err)
		}
	}

	return nil
}

// normalizeEnterpriseAppInstallationRepositorySelection translates GitHub's representation of an
// installation with no repository permissions. GitHub accepts "none" when installing the app, but
// reads that state back as "selected" with an empty repository list.
func normalizeEnterpriseAppInstallationRepositorySelection(selection string, selectedCount int) string {
	if selection == enterpriseAppInstallationRepositorySelectionSelected && selectedCount == 0 {
		return enterpriseAppInstallationRepositorySelectionNone
	}

	return selection
}

func resourceGithubEnterpriseOrganizationAppInstallationDelete(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
	meta, _ := m.(*Owner)
	client := meta.v3client

	enterpriseSlug, organization, clientID, err := parseID3(d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	installationID, _ := d.Get("installation_id").(int)

	tflog.Debug(ctx, "Uninstalling app from enterprise owned organization.", map[string]any{
		"enterprise_slug": enterpriseSlug,
		"organization":    organization,
		"installation_id": installationID,
		"client_id":       clientID,
	})

	if _, err := client.Enterprise.UninstallApp(ctx, enterpriseSlug, organization, int64(installationID)); err != nil && !isEnterpriseAppInstallationNotFound(err) {
		return diag.FromErr(err)
	}

	return nil
}

// enterpriseAppInstallationRepositories reads the configured repository names, preserving the case
// they were written in. GitHub matches repository names case insensitively, but echoing back what
// was configured keeps API errors easier to trace.
func enterpriseAppInstallationRepositories(d *schema.ResourceData) []string {
	return expandStringList(enterpriseAppInstallationRepositorySet(d.Get("selected_repositories")).List())
}

// enterpriseAppInstallationRepositorySet coerces a selected_repositories value into a set, so that
// the set arithmetic in Update stays safe when the value is absent rather than empty.
func enterpriseAppInstallationRepositorySet(value any) *schema.Set {
	if set, ok := value.(*schema.Set); ok && set != nil {
		return set
	}

	return schema.NewSet(caseInsensitiveHash, nil)
}

// splitEnterpriseAppInstallationRepositories divides repositories into a batch the install and
// toggle endpoints accept in one request, and the remainder to be granted separately.
func splitEnterpriseAppInstallationRepositories(repositories []string) (initial, remainder []string) {
	if len(repositories) <= maxEnterpriseAppInstallationRepositoriesPerRequest {
		return repositories, nil
	}

	return repositories[:maxEnterpriseAppInstallationRepositoriesPerRequest],
		repositories[maxEnterpriseAppInstallationRepositoriesPerRequest:]
}

// addEnterpriseAppInstallationRepositories grants an installation access to repositories, in
// batches the API accepts.
func addEnterpriseAppInstallationRepositories(ctx context.Context, client *github.Client, enterpriseSlug, organization string, installationID int64, repositories []string) error {
	for batch := range slices.Chunk(repositories, maxEnterpriseAppInstallationRepositoriesPerRequest) {
		tflog.Debug(ctx, "Granting enterprise organization app installation access to repositories.", map[string]any{
			"enterprise_slug": enterpriseSlug,
			"organization":    organization,
			"installation_id": installationID,
			"repositories":    batch,
		})

		req := github.AppInstallationRepositoriesRequest{Repositories: batch}
		if _, _, err := client.Enterprise.AddRepositoriesToAppInstallation(ctx, enterpriseSlug, organization, installationID, req); err != nil {
			return fmt.Errorf("could not grant app installation %d access to repositories in organization %q: %w", installationID, organization, err)
		}
	}

	return nil
}

// removeEnterpriseAppInstallationRepositories revokes an installation's access to repositories, in
// batches the API accepts.
func removeEnterpriseAppInstallationRepositories(ctx context.Context, client *github.Client, enterpriseSlug, organization string, installationID int64, repositories []string) error {
	for batch := range slices.Chunk(repositories, maxEnterpriseAppInstallationRepositoriesPerRequest) {
		tflog.Debug(ctx, "Revoking enterprise organization app installation access to repositories.", map[string]any{
			"enterprise_slug": enterpriseSlug,
			"organization":    organization,
			"installation_id": installationID,
			"repositories":    batch,
		})

		req := github.AppInstallationRepositoriesRequest{Repositories: batch}
		if _, _, err := client.Enterprise.RemoveRepositoriesFromAppInstallation(ctx, enterpriseSlug, organization, installationID, req); err != nil {
			return fmt.Errorf("could not revoke app installation %d access to repositories in organization %q: %w", installationID, organization, err)
		}
	}

	return nil
}

// findEnterpriseAppInstallationByClientID returns the installation of the app with the given client
// ID on an enterprise owned organization, or nil when the app is not installed. The API offers no
// lookup by client ID, so this scans every installation on the organization.
func findEnterpriseAppInstallationByClientID(ctx context.Context, client *github.Client, maxPerPage int, enterpriseSlug, organization, clientID string) (*github.Installation, error) {
	for installation, err := range client.Enterprise.ListAppInstallationsIter(ctx, enterpriseSlug, organization, &github.ListOptions{PerPage: maxPerPage}) {
		if err != nil {
			return nil, fmt.Errorf("could not list app installations for organization %q: %w", organization, err)
		}

		if installation.GetClientID() == clientID {
			return installation, nil
		}
	}

	return nil, nil
}

// listEnterpriseAppInstallationRepositories returns the names of every repository an installation
// can access.
func listEnterpriseAppInstallationRepositories(ctx context.Context, client *github.Client, maxPerPage int, enterpriseSlug, organization string, installationID int64) ([]string, error) {
	names := make([]string, 0)

	for repository, err := range client.Enterprise.ListRepositoriesForOrgAppInstallationIter(ctx, enterpriseSlug, organization, installationID, &github.ListOptions{PerPage: maxPerPage}) {
		if err != nil {
			return nil, fmt.Errorf("could not list repositories for app installation %d in organization %q: %w", installationID, organization, err)
		}

		if name := repository.Name; name != "" {
			names = append(names, name)
		}
	}

	return names, nil
}

// isEnterpriseAppInstallationNotFound reports whether err is a 404 from the GitHub API.
func isEnterpriseAppInstallationNotFound(err error) bool {
	ghErr, ok := errors.AsType[*github.ErrorResponse](err)

	return ok && ghErr.Response != nil && ghErr.Response.StatusCode == http.StatusNotFound
}
