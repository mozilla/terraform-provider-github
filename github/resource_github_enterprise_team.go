package github

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/google/go-github/v89/github"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/customdiff"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

// enterpriseTeamOrganizationSelectionTypes are the values GitHub accepts for an enterprise team's
// organization_selection_type.
var enterpriseTeamOrganizationSelectionTypes = []string{"disabled", "selected", "all"}

// enterpriseTeamOrganizationSelectionDisabled is the value GitHub reports for a team that is not
// assigned to any organization.
const enterpriseTeamOrganizationSelectionDisabled = "disabled"

func resourceGithubEnterpriseTeam() *schema.Resource {
	return &schema.Resource{
		Description: "Creates and manages a team at the GitHub Enterprise level. Unlike an organization " +
			"team, an enterprise team is defined once for the whole enterprise and can then be assigned to " +
			"organizations within it.",
		CreateContext: resourceGithubEnterpriseTeamCreate,
		ReadContext:   resourceGithubEnterpriseTeamRead,
		UpdateContext: resourceGithubEnterpriseTeamUpdate,
		DeleteContext: resourceGithubEnterpriseTeamDelete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		// GitHub derives the slug from the team name, so renaming a team changes its slug.
		CustomizeDiff: customdiff.Sequence(
			customdiff.ComputedIf("slug", func(_ context.Context, d *schema.ResourceDiff, _ any) bool {
				return d.HasChange("name")
			}),
		),

		Schema: map[string]*schema.Schema{
			"enterprise_slug": {
				Type:             schema.TypeString,
				Required:         true,
				ForceNew:         true,
				Description:      "The slug of the enterprise, as it appears in the enterprise URL.",
				ValidateDiagFunc: validation.ToDiagFunc(validation.StringIsNotWhiteSpace),
			},
			"name": {
				Type:             schema.TypeString,
				Required:         true,
				Description:      "The name of the enterprise team.",
				ValidateDiagFunc: validation.ToDiagFunc(validation.StringIsNotWhiteSpace),
			},
			"description": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "A description of the enterprise team.",
			},
			"organization_selection_type": {
				Type:     schema.TypeString,
				Optional: true,
				Default:  enterpriseTeamOrganizationSelectionDisabled,
				Description: "Which organizations the team is available to: `disabled` for none, `selected` " +
					"for an explicit list, or `all` for every organization in the enterprise. Must be `selected` " +
					"to use `github_enterprise_team_organizations`.",
				ValidateDiagFunc: validateValueFunc(enterpriseTeamOrganizationSelectionTypes),
			},
			"group_id": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "The ID of an identity provider group to synchronise team membership from.",
			},
			"slug": {
				Type:     schema.TypeString,
				Computed: true,
				Description: "The slug of the enterprise team, derived by GitHub from the team name and " +
					"prefixed to distinguish it from organization teams.",
			},
			"team_id": {
				Type:        schema.TypeInt,
				Computed:    true,
				Description: "The numeric ID of the enterprise team.",
			},
		},
	}
}

func resourceGithubEnterpriseTeamCreate(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
	meta, _ := m.(*Owner)
	client := meta.v3client

	enterpriseSlug, _ := d.Get("enterprise_slug").(string)
	groupID, _ := d.Get("group_id").(string)

	req := github.EnterpriseTeamCreateOrUpdateRequest{
		Name:                      d.Get("name").(string),
		Description:               new(d.Get("description").(string)),
		OrganizationSelectionType: new(d.Get("organization_selection_type").(string)),
	}

	// A new team has no group to unlink, so only send group_id when one was configured. Update
	// always sends it, because an empty value is how an existing link is cleared.
	if groupID != "" {
		req.GroupID = new(groupID)
	}

	tflog.Debug(ctx, "Creating enterprise team.", map[string]any{"enterprise_slug": enterpriseSlug, "name": req.Name})

	team, _, err := client.Enterprise.CreateTeam(ctx, enterpriseSlug, req)
	if err != nil {
		return diag.FromErr(err)
	}

	id, err := buildID(enterpriseSlug, strconv.FormatInt(team.ID, 10))
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(id)

	if err := d.Set("slug", team.Slug); err != nil {
		return diag.FromErr(err)
	}

	if err := d.Set("team_id", team.ID); err != nil {
		return diag.FromErr(err)
	}

	return nil
}

func resourceGithubEnterpriseTeamRead(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
	meta, _ := m.(*Owner)
	client := meta.v3client

	enterpriseSlug, teamIDString, err := parseID2(d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	teamID, err := strconv.ParseInt(teamIDString, 10, 64)
	if err != nil {
		return diag.FromErr(unconvertibleIdErr(teamIDString, err))
	}

	// The slug in state identifies the team in a single request. It goes stale if the team was
	// renamed outside Terraform, and is absent right after an import, so fall back to a scan by
	// numeric ID in those cases.
	var team *github.EnterpriseTeam

	if slug, _ := d.Get("slug").(string); slug != "" {
		candidate, _, err := client.Enterprise.GetTeam(ctx, enterpriseSlug, slug)
		switch {
		case err == nil && candidate.ID == teamID:
			team = candidate
		case err != nil && !isEnterpriseTeamNotFound(err):
			return diag.FromErr(err)
		}
	}

	if team == nil {
		team, err = findEnterpriseTeamByID(ctx, client, meta.maxPerPage, enterpriseSlug, teamID)
		if err != nil {
			return diag.FromErr(err)
		}
	}

	if team == nil {
		tflog.Info(ctx, "Removing enterprise team from state because it no longer exists.", map[string]any{
			"enterprise_slug": enterpriseSlug,
			"team_id":         teamID,
		})
		d.SetId("")

		return nil
	}

	organizationSelectionType := team.GetOrganizationSelectionType()
	if organizationSelectionType == "" {
		organizationSelectionType = enterpriseTeamOrganizationSelectionDisabled
	}

	fields := map[string]any{
		"enterprise_slug":             enterpriseSlug,
		"name":                        team.Name,
		"description":                 team.GetDescription(),
		"organization_selection_type": organizationSelectionType,
		"group_id":                    team.GroupID,
		"slug":                        team.Slug,
		"team_id":                     team.ID,
	}

	for key, value := range fields {
		if err := d.Set(key, value); err != nil {
			return diag.FromErr(err)
		}
	}

	return nil
}

func resourceGithubEnterpriseTeamUpdate(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
	meta, _ := m.(*Owner)
	client := meta.v3client

	enterpriseSlug, _ := d.Get("enterprise_slug").(string)

	// The team is addressed by the slug it currently has, which is the one in state. Renaming it
	// marks slug computed, so d.Get would report the pending unknown value rather than the old one.
	oldSlug, _ := d.GetChange("slug")
	slug, _ := oldSlug.(string)

	// Every field is sent on every update. The API treats an omitted field as "leave unchanged",
	// so omitting description or group_id when empty would make them impossible to clear.
	req := github.EnterpriseTeamCreateOrUpdateRequest{
		Name:                      d.Get("name").(string),
		Description:               new(d.Get("description").(string)),
		OrganizationSelectionType: new(d.Get("organization_selection_type").(string)),
		GroupID:                   new(d.Get("group_id").(string)),
	}

	tflog.Debug(ctx, "Updating enterprise team.", map[string]any{"enterprise_slug": enterpriseSlug, "team_slug": slug})

	team, _, err := client.Enterprise.UpdateTeam(ctx, enterpriseSlug, slug, req)
	if err != nil {
		return diag.FromErr(err)
	}

	// Renaming the team changes its slug.
	if err := d.Set("slug", team.Slug); err != nil {
		return diag.FromErr(err)
	}

	return nil
}

func resourceGithubEnterpriseTeamDelete(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
	meta, _ := m.(*Owner)
	client := meta.v3client

	enterpriseSlug, teamIDString, err := parseID2(d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	slug, _ := d.Get("slug").(string)
	if slug == "" {
		teamID, err := strconv.ParseInt(teamIDString, 10, 64)
		if err != nil {
			return diag.FromErr(unconvertibleIdErr(teamIDString, err))
		}

		team, err := findEnterpriseTeamByID(ctx, client, meta.maxPerPage, enterpriseSlug, teamID)
		if err != nil {
			return diag.FromErr(err)
		}

		if team == nil {
			return nil
		}

		slug = team.Slug
	}

	tflog.Debug(ctx, "Deleting enterprise team.", map[string]any{"enterprise_slug": enterpriseSlug, "team_slug": slug})

	if _, err := client.Enterprise.DeleteTeam(ctx, enterpriseSlug, slug); err != nil && !isEnterpriseTeamNotFound(err) {
		return diag.FromErr(err)
	}

	return nil
}

// isEnterpriseTeamNotFound reports whether err is a 404 from the GitHub API.
func isEnterpriseTeamNotFound(err error) bool {
	ghErr, ok := errors.AsType[*github.ErrorResponse](err)

	return ok && ghErr.Response != nil && ghErr.Response.StatusCode == http.StatusNotFound
}
