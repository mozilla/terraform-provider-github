package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/google/go-github/v89/github"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

func resourceGithubEnterpriseRoleTeam() *schema.Resource {
	return &schema.Resource{
		Description: "Manage an association between an enterprise role and an enterprise team.",

		CreateContext: resourceGithubEnterpriseRoleTeamCreate,
		ReadContext:   resourceGithubEnterpriseRoleTeamRead,
		DeleteContext: resourceGithubEnterpriseRoleTeamDelete,
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
			"role_id": {
				Type:             schema.TypeInt,
				Required:         true,
				ForceNew:         true,
				Description:      "The unique identifier of the enterprise role.",
				ValidateDiagFunc: validation.ToDiagFunc(validation.IntAtLeast(1)),
			},
			"team_slug": {
				Type:             schema.TypeString,
				Required:         true,
				ForceNew:         true,
				Description:      "The slug of the enterprise team to assign the role to.",
				ValidateDiagFunc: validation.ToDiagFunc(validation.StringIsNotWhiteSpace),
			},
		},
	}
}

func resourceGithubEnterpriseRoleTeamCreate(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
	meta, _ := m.(*Owner)

	enterpriseSlug, _ := d.Get("enterprise_slug").(string)
	roleID := int64(d.Get("role_id").(int))
	teamSlug, _ := d.Get("team_slug").(string)

	if err := assignEnterpriseRoleToTeam(ctx, meta.v3client, enterpriseSlug, teamSlug, roleID); err != nil {
		return diag.FromErr(err)
	}

	id, err := buildID(enterpriseSlug, strconv.FormatInt(roleID, 10), teamSlug)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(id)

	return nil
}

func resourceGithubEnterpriseRoleTeamRead(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
	meta, _ := m.(*Owner)

	enterpriseSlug, roleIDString, teamSlug, err := parseID3(d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	roleID, err := strconv.ParseInt(roleIDString, 10, 64)
	if err != nil {
		return diag.FromErr(unconvertibleIdErr(roleIDString, err))
	}

	assigned, err := enterpriseRoleAssignedToTeam(ctx, meta.v3client, meta.maxPerPage, enterpriseSlug, teamSlug, roleID)
	if err != nil {
		if isNotFoundError(err) {
			assigned = false
		} else {
			return diag.FromErr(err)
		}
	}

	if !assigned {
		tflog.Info(ctx, "Removing enterprise role team association from state because it no longer exists.", map[string]any{
			"enterprise_slug": enterpriseSlug,
			"role_id":         roleID,
			"team_slug":       teamSlug,
		})
		d.SetId("")

		return nil
	}

	fields := map[string]any{
		"enterprise_slug": enterpriseSlug,
		"role_id":         roleID,
		"team_slug":       teamSlug,
	}

	for key, value := range fields {
		if err := d.Set(key, value); err != nil {
			return diag.FromErr(err)
		}
	}

	return nil
}

func resourceGithubEnterpriseRoleTeamDelete(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
	meta, _ := m.(*Owner)

	enterpriseSlug, roleIDString, teamSlug, err := parseID3(d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	roleID, err := strconv.ParseInt(roleIDString, 10, 64)
	if err != nil {
		return diag.FromErr(unconvertibleIdErr(roleIDString, err))
	}

	if err := removeEnterpriseRoleFromTeam(ctx, meta.v3client, enterpriseSlug, teamSlug, roleID); err != nil && !isNotFoundError(err) {
		return diag.FromErr(err)
	}

	return nil
}

func assignEnterpriseRoleToTeam(ctx context.Context, client *github.Client, enterpriseSlug, teamSlug string, roleID int64) error {
	endpoint := fmt.Sprintf("enterprises/%s/enterprise-roles/teams/%s/%d", enterpriseSlug, teamSlug, roleID)
	req, err := client.NewRequest(ctx, http.MethodPut, endpoint, nil)
	if err != nil {
		return err
	}

	if _, err := client.Do(req, nil); err != nil {
		return fmt.Errorf("could not assign enterprise role %d to team %q: %w", roleID, teamSlug, err)
	}

	return nil
}

func removeEnterpriseRoleFromTeam(ctx context.Context, client *github.Client, enterpriseSlug, teamSlug string, roleID int64) error {
	endpoint := fmt.Sprintf("enterprises/%s/enterprise-roles/teams/%s/%d", enterpriseSlug, teamSlug, roleID)
	req, err := client.NewRequest(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}

	if _, err := client.Do(req, nil); err != nil {
		return fmt.Errorf("could not remove enterprise role %d from team %q: %w", roleID, teamSlug, err)
	}

	return nil
}

func enterpriseRoleAssignedToTeam(ctx context.Context, client *github.Client, perPage int, enterpriseSlug, teamSlug string, roleID int64) (bool, error) {
	page := 0

	for {
		endpoint := fmt.Sprintf("enterprises/%s/enterprise-roles/%d/teams", enterpriseSlug, roleID)
		query := url.Values{}
		if perPage > 0 {
			query.Set("per_page", strconv.Itoa(perPage))
		}
		if page > 0 {
			query.Set("page", strconv.Itoa(page))
		}
		if encoded := query.Encode(); encoded != "" {
			endpoint += "?" + encoded
		}

		req, err := client.NewRequest(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return false, err
		}

		var teams []*github.EnterpriseTeam
		resp, err := client.Do(req, &teams)
		if err != nil {
			return false, err
		}

		for _, team := range teams {
			if team.GetSlug() == teamSlug {
				return true, nil
			}
		}

		if resp.NextPage == 0 {
			return false, nil
		}
		page = resp.NextPage
	}
}

func isNotFoundError(err error) bool {
	ghErr, ok := errors.AsType[*github.ErrorResponse](err)

	return ok && ghErr.Response != nil && ghErr.Response.StatusCode == http.StatusNotFound
}
