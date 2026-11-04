package github

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/google/go-github/v92/github"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceGithubApp() *schema.Resource {
	return &schema.Resource{
		ReadContext: dataSourceGithubAppRead,
		Description: "Get information about an app.",

		Schema: map[string]*schema.Schema{
			"id": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The ID of the app.",
			},
			"slug": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "The URL-friendly name of your GitHub App.",
			},
			"client_id": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The client ID of the app.",
			},
			"external_url": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The external website URL of the app.",
			},
			"html_url": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The GitHub URL of the app.",
			},
			"created_at": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The time the app was created, in RFC3339 format.",
			},
			"updated_at": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The time the app was last updated, in RFC3339 format.",
			},
			"permissions": {
				Type:        schema.TypeMap,
				Computed:    true,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Description: "The app's permissions, keyed by permission name with the access level as the value.",
			},
			"events": {
				Type:        schema.TypeList,
				Computed:    true,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Description: "The webhook events the app subscribes to. Global events (installation_target, security_advisory, and meta) are not included.",
			},
			"installations_count": {
				Type:        schema.TypeInt,
				Computed:    true,
				Description: "The number of app installations. Only returned when the app requests details about itself; otherwise defaults to zero.",
			},
			"owner": githubAppOwnerSchema(),
			"description": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The app's description.",
			},
			"name": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The app's full name.",
			},
			"node_id": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The Node ID of the app.",
			},
		},
	}
}

func dataSourceGithubAppRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	slug := d.Get("slug").(string)

	client := meta.(*Owner).v3client

	// Decode permissions as a map and include enterprise owner fields, which the SDK's App type does not preserve.
	var app struct {
		github.App
		Owner       *githubAppOwner   `json:"owner"`
		Permissions map[string]string `json:"permissions"`
	}
	endpoint := "app"
	if slug != "" {
		endpoint = "apps/" + url.PathEscape(slug)
	}
	req, err := client.NewRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return diag.FromErr(err)
	}
	if _, err = client.Do(req, &app); err != nil {
		return diag.FromErr(err)
	}

	d.SetId(strconv.FormatInt(app.GetID(), 10))
	values := map[string]any{
		"description": app.GetDescription(), "name": app.GetName(), "node_id": app.GetNodeID(),
		"client_id": app.GetClientID(), "external_url": app.GetExternalURL(), "html_url": app.GetHTMLURL(),
		"created_at": githubAppTimestamp(app.CreatedAt), "updated_at": githubAppTimestamp(app.UpdatedAt),
		"permissions": app.Permissions, "events": app.Events, "installations_count": app.GetInstallationsCount(),
		"owner": flattenGithubAppOwner(app.Owner),
	}

	for key, value := range values {
		if err := d.Set(key, value); err != nil {
			return diag.FromErr(err)
		}
	}
	return nil
}
