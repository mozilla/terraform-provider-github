package github

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

func resourceGithubEnterpriseCostCenter() *schema.Resource {
	return &schema.Resource{
		Description:   "Manages a cost center in a GitHub enterprise. Deleting this resource archives the cost center.",
		CreateContext: resourceGithubEnterpriseCostCenterCreate,
		ReadContext:   resourceGithubEnterpriseCostCenterRead,
		UpdateContext: resourceGithubEnterpriseCostCenterUpdate,
		DeleteContext: resourceGithubEnterpriseCostCenterDelete,
		Importer:      &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext},
		Schema: map[string]*schema.Schema{
			"enterprise_slug": {
				Type: schema.TypeString, Required: true, ForceNew: true,
				ValidateDiagFunc: validation.ToDiagFunc(validation.StringIsNotWhiteSpace),
				Description:      "The slug of the enterprise.",
			},
			"name": {
				Type: schema.TypeString, Required: true,
				ValidateDiagFunc: validation.ToDiagFunc(validation.StringLenBetween(1, 255)),
				Description:      "The name of the cost center, up to 255 characters.",
			},
			"ai_credit_pool_enabled": {
				Type: schema.TypeBool, Optional: true, Computed: true,
				Description: "Whether the cost center draws from its own capped AI credit pool. GitHub permits this only when its resources are users or enterprise teams.",
			},
			"cost_center_id": {
				Type: schema.TypeString, Computed: true,
				Description: "The GitHub ID of the cost center.",
			},
			"state": {
				Type: schema.TypeString, Computed: true,
				Description: "The state of the cost center.",
			},
			"azure_subscription": {
				Type: schema.TypeString, Computed: true,
				Description: "The Azure subscription associated with the cost center, if any.",
			},
			"ai_credit_pool_target_amount": {
				Type: schema.TypeFloat, Computed: true,
				Description: "The target amount of the AI credit pool, when reported by GitHub.",
			},
			"ai_credit_pool_current_amount": {
				Type: schema.TypeFloat, Computed: true,
				Description: "The current amount of the AI credit pool, when reported by GitHub.",
			},
		},
	}
}

func resourceGithubEnterpriseCostCenterCreate(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
	client := m.(*Owner).v3client
	enterprise := d.Get("enterprise_slug").(string)
	name := d.Get("name").(string)
	change := costCenterChange{Name: &name}
	if value, ok := d.GetOkExists("ai_credit_pool_enabled"); ok {
		enabled := value.(bool)
		change.AICreditPoolEnabled = &enabled
	}
	var center costCenter
	if err := costCenterRequest(ctx, client, http.MethodPost, costCenterPath(enterprise, ""), change, &center); err != nil {
		return diag.FromErr(fmt.Errorf("create cost center in enterprise %q: %w", enterprise, err))
	}
	if center.ID == "" {
		return diag.Errorf("create cost center in enterprise %q returned no ID", enterprise)
	}
	id, err := buildID(enterprise, center.ID)
	if err != nil {
		return diag.FromErr(err)
	}
	d.SetId(id)
	return resourceGithubEnterpriseCostCenterRead(ctx, d, m)
}

func resourceGithubEnterpriseCostCenterRead(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
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
	fields := map[string]any{
		"enterprise_slug":               enterprise,
		"cost_center_id":                id,
		"name":                          center.Name,
		"state":                         center.State,
		"azure_subscription":            center.AzureSubscription,
		"ai_credit_pool_enabled":        center.AICreditPoolEnabled,
		"ai_credit_pool_target_amount":  nil,
		"ai_credit_pool_current_amount": nil,
	}
	if center.AICreditPoolState != nil {
		if center.AICreditPoolState.TargetAmount != nil {
			fields["ai_credit_pool_target_amount"] = *center.AICreditPoolState.TargetAmount
		}
		if center.AICreditPoolState.CurrentAmount != nil {
			fields["ai_credit_pool_current_amount"] = *center.AICreditPoolState.CurrentAmount
		}
	}
	for key, value := range fields {
		if err := d.Set(key, value); err != nil {
			return diag.FromErr(err)
		}
	}
	return nil
}

func resourceGithubEnterpriseCostCenterUpdate(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
	enterprise, id, err := parseID2(d.Id())
	if err != nil {
		return diag.FromErr(err)
	}
	change := costCenterChange{}
	if d.HasChange("name") {
		name := d.Get("name").(string)
		change.Name = &name
	}
	if d.HasChange("ai_credit_pool_enabled") {
		enabled := d.Get("ai_credit_pool_enabled").(bool)
		change.AICreditPoolEnabled = &enabled
	}
	if err := costCenterRequest(ctx, m.(*Owner).v3client, http.MethodPatch, costCenterPath(enterprise, id), change, nil); err != nil {
		return diag.FromErr(fmt.Errorf("update cost center %q in enterprise %q: %w", id, enterprise, err))
	}
	return resourceGithubEnterpriseCostCenterRead(ctx, d, m)
}

func resourceGithubEnterpriseCostCenterDelete(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
	enterprise, id, err := parseID2(d.Id())
	if err != nil {
		return diag.FromErr(err)
	}
	err = costCenterRequest(ctx, m.(*Owner).v3client, http.MethodDelete, costCenterPath(enterprise, id), nil, nil)
	if err != nil && !costCenterNotFound(err) {
		return diag.FromErr(fmt.Errorf("archive cost center %q in enterprise %q: %w", id, enterprise, err))
	}
	d.SetId("")
	return nil
}
