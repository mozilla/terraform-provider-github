package github

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

func resourceGithubEnterpriseTeamOrganizations() *schema.Resource {
	return &schema.Resource{
		Description: "Authoritatively manages the organizations a GitHub enterprise team is assigned to. " +
			"Any organization the team is assigned to that is not listed here is unassigned. The team's " +
			"`organization_selection_type` must be `selected`.",
		CreateContext: resourceGithubEnterpriseTeamOrganizationsCreate,
		ReadContext:   resourceGithubEnterpriseTeamOrganizationsRead,
		UpdateContext: resourceGithubEnterpriseTeamOrganizationsUpdate,
		DeleteContext: resourceGithubEnterpriseTeamOrganizationsDelete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"enterprise_slug": {
				Type:             schema.TypeString,
				Required:         true,
				ForceNew:         true,
				Description:      "The slug of the enterprise, as it appears in the enterprise URL.",
				ValidateDiagFunc: validation.ToDiagFunc(validation.StringIsNotWhiteSpace),
			},
			"team_slug": {
				Type:             schema.TypeString,
				Required:         true,
				ForceNew:         true,
				Description:      "The slug of the enterprise team to assign to organizations.",
				ValidateDiagFunc: validation.ToDiagFunc(validation.StringIsNotWhiteSpace),
			},
			"organization_slugs": {
				Type:        schema.TypeSet,
				Required:    true,
				MinItems:    1,
				Description: "The slugs of every organization the enterprise team should be assigned to.",
				Elem: &schema.Schema{
					Type:             schema.TypeString,
					ValidateDiagFunc: validation.ToDiagFunc(validation.StringIsNotWhiteSpace),
				},
				Set: caseInsensitiveHash,
			},
		},
	}
}

func resourceGithubEnterpriseTeamOrganizationsCreate(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
	meta, _ := m.(*Owner)

	enterpriseSlug, _ := d.Get("enterprise_slug").(string)
	teamSlug, _ := d.Get("team_slug").(string)

	id, err := buildID(enterpriseSlug, teamSlug)
	if err != nil {
		return diag.FromErr(err)
	}

	want := enterpriseTeamSetValues(d, "organization_slugs")
	if err := updateEnterpriseTeamOrganizations(ctx, meta, enterpriseSlug, teamSlug, want); err != nil {
		return diag.FromErr(err)
	}

	d.SetId(id)

	return nil
}

func resourceGithubEnterpriseTeamOrganizationsRead(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
	meta, _ := m.(*Owner)

	enterpriseSlug, teamSlug, err := parseID2(d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	organizations, err := listEnterpriseTeamOrganizations(ctx, meta.v3client, meta.maxPerPage, enterpriseSlug, teamSlug)
	if err != nil {
		if isEnterpriseTeamNotFound(err) {
			tflog.Info(ctx, "Removing enterprise team organizations from state because the team no longer exists.", map[string]any{
				"enterprise_slug": enterpriseSlug,
				"team_slug":       teamSlug,
			})
			d.SetId("")

			return nil
		}

		return diag.FromErr(err)
	}

	fields := map[string]any{
		"enterprise_slug":    enterpriseSlug,
		"team_slug":          teamSlug,
		"organization_slugs": organizations,
	}

	for key, value := range fields {
		if err := d.Set(key, value); err != nil {
			return diag.FromErr(err)
		}
	}

	return nil
}

func resourceGithubEnterpriseTeamOrganizationsUpdate(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
	meta, _ := m.(*Owner)

	enterpriseSlug, teamSlug, err := parseID2(d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	want := enterpriseTeamSetValues(d, "organization_slugs")
	if err := updateEnterpriseTeamOrganizations(ctx, meta, enterpriseSlug, teamSlug, want); err != nil {
		return diag.FromErr(err)
	}

	return nil
}

func resourceGithubEnterpriseTeamOrganizationsDelete(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
	meta, _ := m.(*Owner)

	enterpriseSlug, teamSlug, err := parseID2(d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	if err := updateEnterpriseTeamOrganizations(ctx, meta, enterpriseSlug, teamSlug, nil); err != nil {
		if isEnterpriseTeamNotFound(err) {
			return nil
		}

		return diag.FromErr(err)
	}

	return nil
}

// updateEnterpriseTeamOrganizations reconciles the organizations an enterprise team is assigned to
// against want, in at most one bulk call each way. The assignment endpoints are additive and
// subtractive rather than a replacement, so the current assignments have to be read first.
func updateEnterpriseTeamOrganizations(ctx context.Context, meta *Owner, enterpriseSlug, teamSlug string, want []string) error {
	client := meta.v3client

	current, err := listEnterpriseTeamOrganizations(ctx, client, meta.maxPerPage, enterpriseSlug, teamSlug)
	if err != nil {
		return err
	}

	add := enterpriseTeamMissingFrom(want, current)
	remove := enterpriseTeamMissingFrom(current, want)

	tflog.Debug(ctx, "Updating enterprise team organizations.", map[string]any{
		"enterprise_slug": enterpriseSlug,
		"team_slug":       teamSlug,
		"add":             add,
		"remove":          remove,
	})

	if len(add) > 0 {
		if _, _, err := client.Enterprise.AddMultipleAssignments(ctx, enterpriseSlug, teamSlug, add); err != nil {
			return fmt.Errorf("could not assign enterprise team %q to organizations: %w", teamSlug, err)
		}
	}

	if len(remove) > 0 {
		if _, _, err := client.Enterprise.RemoveMultipleAssignments(ctx, enterpriseSlug, teamSlug, remove); err != nil {
			return fmt.Errorf("could not unassign enterprise team %q from organizations: %w", teamSlug, err)
		}
	}

	return nil
}
