package github

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/shurcooL/githubv4"
)

func dataSourceGithubEnterpriseMembers() *schema.Resource {
	return &schema.Resource{
		ReadContext: dataSourceGithubEnterpriseMembersRead,

		Description: "Data source to list all members of a GitHub enterprise.",

		Schema: map[string]*schema.Schema{
			"enterprise_slug": {
				Description: "The slug of the enterprise whose members will be listed.",
				Type:        schema.TypeString,
				Required:    true,
			},
			"members": {
				Description: "Enterprise members.",
				Type:        schema.TypeList,
				Computed:    true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"id": {
							Description: "Database ID of the member.",
							Type:        schema.TypeInt,
							Computed:    true,
						},
						"node_id": {
							Description: "Node ID of the member.",
							Type:        schema.TypeString,
							Computed:    true,
						},
						"login": {
							Description: "Login of the member.",
							Type:        schema.TypeString,
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

func dataSourceGithubEnterpriseMembersRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	owner := meta.(*Owner)
	enterpriseSlug := d.Get("enterprise_slug").(string)

	var query struct {
		Enterprise struct {
			ID      githubv4.String
			Members struct {
				Nodes []struct {
					User struct {
						ID         githubv4.String
						DatabaseID githubv4.Int
						Login      githubv4.String
					} `graphql:"... on User"`
					EnterpriseUserAccount struct {
						Login githubv4.String
						User  struct {
							ID         githubv4.String
							DatabaseID githubv4.Int
						}
					} `graphql:"... on EnterpriseUserAccount"`
				}
				PageInfo PageInfo
			} `graphql:"members(first: $first, after: $after)"`
		} `graphql:"enterprise(slug: $slug)"`
	}

	variables := map[string]any{
		"slug":  githubv4.String(enterpriseSlug),
		"first": githubv4.Int(owner.maxPerPage),
		"after": (*githubv4.String)(nil),
	}

	members := make([]map[string]any, 0)
	for {
		if err := owner.v4client.Query(ctx, &query, variables); err != nil {
			return diag.FromErr(err)
		}

		for _, node := range query.Enterprise.Members.Nodes {
			id := node.User.DatabaseID
			nodeID := node.User.ID
			login := node.User.Login
			// Login exists on both union variants and the GraphQL decoder populates matching
			// fields in both inline fragments. The top-level ID, however, is only present for
			// a regular User, so use it to distinguish enterprise-managed accounts.
			if nodeID == "" {
				id = node.EnterpriseUserAccount.User.DatabaseID
				nodeID = node.EnterpriseUserAccount.User.ID
				login = node.EnterpriseUserAccount.Login
			}

			members = append(members, map[string]any{
				"id":      int(id),
				"node_id": string(nodeID),
				"login":   string(login),
			})
		}

		if !query.Enterprise.Members.PageInfo.HasNextPage {
			break
		}
		variables["after"] = new(query.Enterprise.Members.PageInfo.EndCursor)
	}

	if query.Enterprise.ID == "" {
		return diag.Errorf("could not find enterprise %q", enterpriseSlug)
	}

	d.SetId(string(query.Enterprise.ID))
	if err := d.Set("members", members); err != nil {
		return diag.Errorf("error setting enterprise members: %v", err)
	}

	return nil
}
