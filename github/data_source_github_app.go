package github

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"

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
			"owner": {
				Type:        schema.TypeList,
				Computed:    true,
				Description: "The user, organization, or enterprise that owns the app. Fields that do not apply to the owner type are empty.",
				Elem: &schema.Resource{Schema: map[string]*schema.Schema{
					"avatar_url": {
						Type:        schema.TypeString,
						Computed:    true,
						Description: "The owner's avatar URL.",
					},
					"created_at": {
						Type:        schema.TypeString,
						Computed:    true,
						Description: "The time the owner was created. Only returned for enterprise owners, in RFC3339 format.",
					},
					"description": {
						Type:        schema.TypeString,
						Computed:    true,
						Description: "The owner's description. Only returned for enterprise owners.",
					},
					"email": {
						Type:        schema.TypeString,
						Computed:    true,
						Description: "The owner's email.",
					},
					"events_url": {
						Type:        schema.TypeString,
						Computed:    true,
						Description: "The owner's events URL.",
					},
					"followers_url": {
						Type:        schema.TypeString,
						Computed:    true,
						Description: "The owner's followers URL.",
					},
					"following_url": {
						Type:        schema.TypeString,
						Computed:    true,
						Description: "The owner's following URL.",
					},
					"gists_url": {
						Type:        schema.TypeString,
						Computed:    true,
						Description: "The owner's gists URL.",
					},
					"gravatar_id": {
						Type:        schema.TypeString,
						Computed:    true,
						Description: "The owner's gravatar ID.",
					},
					"html_url": {
						Type:        schema.TypeString,
						Computed:    true,
						Description: "The owner's GitHub URL.",
					},
					"id": {
						Type:        schema.TypeInt,
						Computed:    true,
						Description: "The owner's ID.",
					},
					"login": {
						Type:        schema.TypeString,
						Computed:    true,
						Description: "The owner's login.",
					},
					"name": {
						Type:        schema.TypeString,
						Computed:    true,
						Description: "The owner's name.",
					},
					"node_id": {
						Type:        schema.TypeString,
						Computed:    true,
						Description: "The owner's Node ID.",
					},
					"organizations_url": {
						Type:        schema.TypeString,
						Computed:    true,
						Description: "The owner's organizations URL.",
					},
					"received_events_url": {
						Type:        schema.TypeString,
						Computed:    true,
						Description: "The owner's received events URL.",
					},
					"repos_url": {
						Type:        schema.TypeString,
						Computed:    true,
						Description: "The owner's repos URL.",
					},
					"site_admin": {
						Type:        schema.TypeBool,
						Computed:    true,
						Description: "Whether the owner is a GitHub site administrator.",
					},
					"slug": {
						Type:        schema.TypeString,
						Computed:    true,
						Description: "The owner's slug. Only returned for enterprise owners.",
					},
					"starred_at": {
						Type:        schema.TypeString,
						Computed:    true,
						Description: "The time the owner was starred.",
					},
					"starred_url": {
						Type:        schema.TypeString,
						Computed:    true,
						Description: "The owner's starred URL.",
					},
					"subscriptions_url": {
						Type:        schema.TypeString,
						Computed:    true,
						Description: "The owner's subscriptions URL.",
					},
					"type": {
						Type:        schema.TypeString,
						Computed:    true,
						Description: "The owner's type.",
					},
					"updated_at": {
						Type:        schema.TypeString,
						Computed:    true,
						Description: "The time the owner was last updated. Only returned for enterprise owners, in RFC3339 format.",
					},
					"url": {
						Type:        schema.TypeString,
						Computed:    true,
						Description: "The owner's URL.",
					},
					"user_view_type": {
						Type:        schema.TypeString,
						Computed:    true,
						Description: "The owner's user view type.",
					},
					"website_url": {
						Type:        schema.TypeString,
						Computed:    true,
						Description: "The owner's website URL. Only returned for enterprise owners.",
					},
				}},
			},
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
		"owner": []any{},
	}
	if app.Owner != nil {
		owner := app.Owner
		values["owner"] = []any{map[string]any{
			"avatar_url":          owner.GetAvatarURL(),
			"created_at":          githubAppTimestamp(owner.CreatedAt),
			"description":         owner.Description,
			"email":               owner.GetEmail(),
			"events_url":          owner.GetEventsURL(),
			"followers_url":       owner.GetFollowersURL(),
			"following_url":       owner.GetFollowingURL(),
			"gists_url":           owner.GetGistsURL(),
			"gravatar_id":         owner.GetGravatarID(),
			"html_url":            owner.GetHTMLURL(),
			"id":                  owner.GetID(),
			"login":               owner.GetLogin(),
			"name":                owner.GetName(),
			"node_id":             owner.GetNodeID(),
			"organizations_url":   owner.GetOrganizationsURL(),
			"received_events_url": owner.GetReceivedEventsURL(),
			"repos_url":           owner.GetReposURL(),
			"site_admin":          owner.GetSiteAdmin(),
			"slug":                owner.Slug,
			"starred_at":          owner.StarredAt,
			"starred_url":         owner.GetStarredURL(),
			"subscriptions_url":   owner.GetSubscriptionsURL(),
			"type":                owner.GetType(),
			"updated_at":          githubAppTimestamp(owner.UpdatedAt),
			"url":                 owner.GetURL(),
			"user_view_type":      owner.GetUserViewType(),
			"website_url":         owner.WebsiteURL,
		}}
	}
	for key, value := range values {
		if err := d.Set(key, value); err != nil {
			return diag.FromErr(err)
		}
	}
	return nil
}

type githubAppOwner struct {
	github.User
	StarredAt   string `json:"starred_at"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	WebsiteURL  string `json:"website_url"`
}

func githubAppTimestamp(timestamp *github.Timestamp) string {
	if timestamp == nil {
		return ""
	}
	return timestamp.Format(time.RFC3339)
}
