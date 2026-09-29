package github

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

func resourceGithubEnterpriseCostCenterResources() *schema.Resource {
	resourceSet := func(description string) *schema.Schema {
		return &schema.Schema{
			Type: schema.TypeSet, Optional: true, Description: description,
			Elem: &schema.Schema{Type: schema.TypeString, ValidateDiagFunc: validation.ToDiagFunc(validation.StringIsNotWhiteSpace)},
			Set:  caseInsensitiveHash,
		}
	}
	return &schema.Resource{
		Description:   "Authoritatively manages the resources assigned to an enterprise cost center. Resources absent from these sets are removed from the cost center; destroying this resource removes all assignments.",
		CreateContext: resourceGithubEnterpriseCostCenterResourcesCreate,
		ReadContext:   resourceGithubEnterpriseCostCenterResourcesRead,
		UpdateContext: resourceGithubEnterpriseCostCenterResourcesUpdate,
		DeleteContext: resourceGithubEnterpriseCostCenterResourcesDelete,
		Importer:      &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext},
		Schema: map[string]*schema.Schema{
			"enterprise_slug": {
				Type: schema.TypeString, Required: true, ForceNew: true,
				ValidateDiagFunc: validation.ToDiagFunc(validation.StringIsNotWhiteSpace),
				Description:      "The slug of the enterprise.",
			},
			"cost_center_id": {
				Type: schema.TypeString, Required: true, ForceNew: true,
				ValidateDiagFunc: validation.ToDiagFunc(validation.StringIsNotWhiteSpace),
				Description:      "The GitHub ID of the cost center.",
			},
			"users":            resourceSet("Usernames assigned to the cost center."),
			"organizations":    resourceSet("Organization slugs assigned to the cost center."),
			"repositories":     resourceSet("Repositories assigned to the cost center, as owner/name."),
			"enterprise_teams": resourceSet("Enterprise team slugs assigned to the cost center."),
		},
	}
}

func costCenterDesiredResources(d *schema.ResourceData) costCenterResourcesChange {
	return costCenterResourcesChange{
		Users:           enterpriseTeamSetValues(d, "users"),
		Organizations:   enterpriseTeamSetValues(d, "organizations"),
		Repositories:    enterpriseTeamSetValues(d, "repositories"),
		EnterpriseTeams: enterpriseTeamSetValues(d, "enterprise_teams"),
	}
}

func costCenterActualResources(resources []costCenterResource) (costCenterResourcesChange, error) {
	var actual costCenterResourcesChange
	for _, resource := range resources {
		switch strings.ToLower(resource.Type) {
		case "user":
			actual.Users = append(actual.Users, resource.Name)
		case "organization", "org":
			actual.Organizations = append(actual.Organizations, resource.Name)
		case "repo", "repository":
			actual.Repositories = append(actual.Repositories, resource.Name)
		case "team", "enterpriseteam", "enterprise_team":
			actual.EnterpriseTeams = append(actual.EnterpriseTeams, resource.Name)
		default:
			return actual, fmt.Errorf("unknown cost center resource type %q", resource.Type)
		}
	}
	return actual, nil
}

func costCenterResourceDifference(a, b costCenterResourcesChange) costCenterResourcesChange {
	return costCenterResourcesChange{
		Users:           enterpriseTeamMissingFrom(a.Users, b.Users),
		Organizations:   enterpriseTeamMissingFrom(a.Organizations, b.Organizations),
		Repositories:    enterpriseTeamMissingFrom(a.Repositories, b.Repositories),
		EnterpriseTeams: enterpriseTeamMissingFrom(a.EnterpriseTeams, b.EnterpriseTeams),
	}
}

func (change costCenterResourcesChange) empty() bool {
	return len(change.Users)+len(change.Organizations)+len(change.Repositories)+len(change.EnterpriseTeams) == 0
}

// The 50-resource limit applies to the combined total across all resource types.
func (change costCenterResourcesChange) batches() []costCenterResourcesChange {
	var batches []costCenterResourcesChange
	for !change.empty() {
		remaining := 50
		take := func(resources *[]string) []string {
			n := min(len(*resources), remaining)
			batch := (*resources)[:n]
			*resources = (*resources)[n:]
			remaining -= n
			return batch
		}
		batches = append(batches, costCenterResourcesChange{
			Users:           take(&change.Users),
			Organizations:   take(&change.Organizations),
			Repositories:    take(&change.Repositories),
			EnterpriseTeams: take(&change.EnterpriseTeams),
		})
	}
	return batches
}

func reconcileCostCenterResources(ctx context.Context, owner *Owner, enterprise, id string, desired costCenterResourcesChange) error {
	center, err := getCostCenter(ctx, owner.v3client, enterprise, id)
	if err != nil {
		return err
	}
	actual, err := costCenterActualResources(center.Resources)
	if err != nil {
		return err
	}
	remove := costCenterResourceDifference(actual, desired)
	add := costCenterResourceDifference(desired, actual)
	path := costCenterPath(enterprise, id) + "/resource"
	for _, batch := range remove.batches() {
		if err := costCenterRequest(ctx, owner.v3client, http.MethodDelete, path, batch, nil); err != nil {
			return fmt.Errorf("remove resources from cost center %q: %w", id, err)
		}
	}
	for _, batch := range add.batches() {
		if err := costCenterRequest(ctx, owner.v3client, http.MethodPost, path, batch, nil); err != nil {
			return fmt.Errorf("add resources to cost center %q: %w", id, err)
		}
	}
	return nil
}

func resourceGithubEnterpriseCostCenterResourcesCreate(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
	enterprise := d.Get("enterprise_slug").(string)
	id := d.Get("cost_center_id").(string)
	if err := reconcileCostCenterResources(ctx, m.(*Owner), enterprise, id, costCenterDesiredResources(d)); err != nil {
		return diag.FromErr(err)
	}
	stateID, err := buildID(enterprise, id)
	if err != nil {
		return diag.FromErr(err)
	}
	d.SetId(stateID)
	return resourceGithubEnterpriseCostCenterResourcesRead(ctx, d, m)
}

func resourceGithubEnterpriseCostCenterResourcesRead(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
	enterprise, id, err := parseID2(d.Id())
	if err != nil {
		return diag.FromErr(err)
	}
	center, err := getCostCenter(ctx, m.(*Owner).v3client, enterprise, id)
	if costCenterNotFound(err) {
		d.SetId("")
		return nil
	}
	if err != nil {
		return diag.FromErr(err)
	}
	if center.State == "deleted" {
		d.SetId("")
		return nil
	}
	actual, err := costCenterActualResources(center.Resources)
	if err != nil {
		return diag.FromErr(err)
	}
	fields := map[string]any{
		"enterprise_slug":  enterprise,
		"cost_center_id":   id,
		"users":            enterpriseTeamPreserveSetValueCase(d, "users", actual.Users),
		"organizations":    enterpriseTeamPreserveSetValueCase(d, "organizations", actual.Organizations),
		"repositories":     enterpriseTeamPreserveSetValueCase(d, "repositories", actual.Repositories),
		"enterprise_teams": enterpriseTeamPreserveSetValueCase(d, "enterprise_teams", actual.EnterpriseTeams),
	}
	for key, value := range fields {
		if err := d.Set(key, value); err != nil {
			return diag.FromErr(err)
		}
	}
	return nil
}

func resourceGithubEnterpriseCostCenterResourcesUpdate(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
	enterprise, id, err := parseID2(d.Id())
	if err != nil {
		return diag.FromErr(err)
	}
	if err := reconcileCostCenterResources(ctx, m.(*Owner), enterprise, id, costCenterDesiredResources(d)); err != nil {
		return diag.FromErr(err)
	}
	return resourceGithubEnterpriseCostCenterResourcesRead(ctx, d, m)
}

func resourceGithubEnterpriseCostCenterResourcesDelete(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
	enterprise, id, err := parseID2(d.Id())
	if err != nil {
		return diag.FromErr(err)
	}
	err = reconcileCostCenterResources(ctx, m.(*Owner), enterprise, id, costCenterResourcesChange{})
	if err != nil && !costCenterNotFound(err) {
		return diag.FromErr(err)
	}
	d.SetId("")
	return nil
}
