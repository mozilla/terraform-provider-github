package github

import (
	"context"

	"github.com/google/go-github/v89/github"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceGithubOrganizationOutsideCollaborators() *schema.Resource {
	return &schema.Resource{
		ReadContext: dataSourceGithubOrganizationOutsideCollaboratorsRead,

		Description: "Data source to list all outside collaborators in a GitHub organization.",

		Schema: map[string]*schema.Schema{
			"organization": {
				Description: "The GitHub organization whose outside collaborators will be listed.",
				Type:        schema.TypeString,
				Required:    true,
			},
			"outside_collaborators": {
				Description: "Outside collaborators in the organization.",
				Type:        schema.TypeList,
				Computed:    true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"id": {
							Description: "ID of the outside collaborator.",
							Type:        schema.TypeInt,
							Computed:    true,
						},
						"node_id": {
							Description: "Node ID of the outside collaborator.",
							Type:        schema.TypeString,
							Computed:    true,
						},
						"login": {
							Description: "Login of the outside collaborator.",
							Type:        schema.TypeString,
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

func dataSourceGithubOrganizationOutsideCollaboratorsRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	owner := meta.(*Owner)
	organization := d.Get("organization").(string)

	outsideCollaborators := make([]map[string]any, 0)
	options := &github.ListOutsideCollaboratorsOptions{
		ListOptions: github.ListOptions{PerPage: owner.maxPerPage},
	}
	for collaborator, err := range owner.v3client.Organizations.ListOutsideCollaboratorsIter(ctx, organization, options) {
		if err != nil {
			return diag.FromErr(err)
		}

		outsideCollaborators = append(outsideCollaborators, map[string]any{
			"id":      collaborator.GetID(),
			"node_id": collaborator.GetNodeID(),
			"login":   collaborator.GetLogin(),
		})
	}

	d.SetId(organization)
	if err := d.Set("outside_collaborators", outsideCollaborators); err != nil {
		return diag.Errorf("error setting organization outside collaborators: %v", err)
	}

	return nil
}
