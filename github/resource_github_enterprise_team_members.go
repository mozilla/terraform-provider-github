package github

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

func resourceGithubEnterpriseTeamMembers() *schema.Resource {
	return &schema.Resource{
		Description: "Authoritatively manages the members of a GitHub enterprise team. Any member of the " +
			"team that is not listed here is removed from it. Enterprise teams have no per-member role, so " +
			"members are given as a plain set of usernames.",
		CreateContext: resourceGithubEnterpriseTeamMembersCreate,
		ReadContext:   resourceGithubEnterpriseTeamMembersRead,
		UpdateContext: resourceGithubEnterpriseTeamMembersUpdate,
		DeleteContext: resourceGithubEnterpriseTeamMembersDelete,
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
				Description:      "The slug of the enterprise team to manage membership for.",
				ValidateDiagFunc: validation.ToDiagFunc(validation.StringIsNotWhiteSpace),
			},
			"members": {
				Type:        schema.TypeSet,
				Required:    true,
				Description: "The usernames of every user that should be a member of the team.",
				Elem: &schema.Schema{
					Type:             schema.TypeString,
					ValidateDiagFunc: validation.ToDiagFunc(validation.StringIsNotWhiteSpace),
				},
				Set: caseInsensitiveHash,
			},
		},
	}
}

func resourceGithubEnterpriseTeamMembersCreate(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
	meta, _ := m.(*Owner)

	enterpriseSlug, _ := d.Get("enterprise_slug").(string)
	teamSlug, _ := d.Get("team_slug").(string)

	id, err := buildID(enterpriseSlug, teamSlug)
	if err != nil {
		return diag.FromErr(err)
	}

	if err := updateEnterpriseTeamMembers(ctx, meta, enterpriseSlug, teamSlug, enterpriseTeamSetValues(d, "members")); err != nil {
		return diag.FromErr(err)
	}

	d.SetId(id)

	return nil
}

func resourceGithubEnterpriseTeamMembersRead(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
	meta, _ := m.(*Owner)

	enterpriseSlug, teamSlug, err := parseID2(d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	members, err := listEnterpriseTeamMembers(ctx, meta.v3client, meta.maxPerPage, enterpriseSlug, teamSlug)
	if err != nil {
		if isEnterpriseTeamNotFound(err) {
			tflog.Info(ctx, "Removing enterprise team members from state because the team no longer exists.", map[string]any{
				"enterprise_slug": enterpriseSlug,
				"team_slug":       teamSlug,
			})
			d.SetId("")

			return nil
		}

		return diag.FromErr(err)
	}

	fields := map[string]any{
		"enterprise_slug": enterpriseSlug,
		"team_slug":       teamSlug,
		"members":         members,
	}

	for key, value := range fields {
		if err := d.Set(key, value); err != nil {
			return diag.FromErr(err)
		}
	}

	return nil
}

func resourceGithubEnterpriseTeamMembersUpdate(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
	meta, _ := m.(*Owner)

	enterpriseSlug, teamSlug, err := parseID2(d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	if err := updateEnterpriseTeamMembers(ctx, meta, enterpriseSlug, teamSlug, enterpriseTeamSetValues(d, "members")); err != nil {
		return diag.FromErr(err)
	}

	return nil
}

func resourceGithubEnterpriseTeamMembersDelete(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
	meta, _ := m.(*Owner)

	enterpriseSlug, teamSlug, err := parseID2(d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	if err := updateEnterpriseTeamMembers(ctx, meta, enterpriseSlug, teamSlug, nil); err != nil {
		if isEnterpriseTeamNotFound(err) {
			return nil
		}

		return diag.FromErr(err)
	}

	return nil
}

// updateEnterpriseTeamMembers reconciles the members of an enterprise team against want, adding
// and removing in at most one bulk call each. The bulk endpoints are additive and subtractive
// rather than a replacement, so the current membership has to be read first.
func updateEnterpriseTeamMembers(ctx context.Context, meta *Owner, enterpriseSlug, teamSlug string, want []string) error {
	client := meta.v3client

	current, err := listEnterpriseTeamMembers(ctx, client, meta.maxPerPage, enterpriseSlug, teamSlug)
	if err != nil {
		return err
	}

	add := enterpriseTeamMissingFrom(want, current)
	remove := enterpriseTeamMissingFrom(current, want)

	tflog.Debug(ctx, "Updating enterprise team members.", map[string]any{
		"enterprise_slug": enterpriseSlug,
		"team_slug":       teamSlug,
		"add":             add,
		"remove":          remove,
	})

	if len(add) > 0 {
		if _, _, err := client.Enterprise.BulkAddTeamMembers(ctx, enterpriseSlug, teamSlug, add); err != nil {
			return fmt.Errorf("could not add members to enterprise team %q: %w", teamSlug, err)
		}
	}

	if len(remove) > 0 {
		if _, _, err := client.Enterprise.BulkRemoveTeamMembers(ctx, enterpriseSlug, teamSlug, remove); err != nil {
			return fmt.Errorf("could not remove members from enterprise team %q: %w", teamSlug, err)
		}
	}

	return nil
}

// enterpriseTeamSetValues reads a set of strings from the resource data, lowercased and sorted so
// that comparisons and log output are stable. GitHub logins and slugs are case-insensitive.
func enterpriseTeamSetValues(d *schema.ResourceData, key string) []string {
	set, ok := d.Get(key).(*schema.Set)
	if !ok {
		return nil
	}

	values := make([]string, 0, set.Len())

	for _, item := range set.List() {
		if s, ok := item.(string); ok && s != "" {
			values = append(values, strings.ToLower(s))
		}
	}

	slices.Sort(values)

	return slices.Compact(values)
}

// enterpriseTeamMissingFrom returns the values in want that do not appear in have.
func enterpriseTeamMissingFrom(want, have []string) []string {
	missing := make([]string, 0)

	for _, value := range want {
		if !slices.Contains(have, value) {
			missing = append(missing, value)
		}
	}

	return missing
}
