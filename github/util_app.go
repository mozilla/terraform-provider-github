package github

import (
	"time"

	"github.com/google/go-github/v92/github"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func githubAppOwnerSchema() *schema.Schema {
	return &schema.Schema{
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
	}
}

func flattenGithubAppOwner(owner *githubAppOwner) []any {
	if owner == nil {
		return []any{}
	}
	return []any{map[string]any{
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
