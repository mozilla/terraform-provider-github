package github

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/shurcooL/githubv4"
)

const (
	// enterpriseMemberStatusPending means an invitation has been sent but not yet accepted.
	enterpriseMemberStatusPending = "pending"
	// enterpriseMemberStatusActive means the user has accepted and is an enterprise member.
	enterpriseMemberStatusActive = "active"
	// enterpriseMemberStatusExpired means a previously managed invitation is no longer pending.
	enterpriseMemberStatusExpired = "expired"
)

func resourceGithubEnterpriseMember() *schema.Resource {
	return &schema.Resource{
		Description: "Invites a user to a GitHub Enterprise and manages their membership. Creating this " +
			"resource sends an enterprise invitation; destroying it cancels a still-pending invitation, or " +
			"removes the user from the enterprise if they have already accepted.",
		CreateContext: resourceGithubEnterpriseMemberCreate,
		ReadContext:   resourceGithubEnterpriseMemberRead,
		UpdateContext: resourceGithubEnterpriseMemberUpdate,
		DeleteContext: resourceGithubEnterpriseMemberDelete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"enterprise_slug": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "The slug of the enterprise to invite the user to.",
			},
			"username": {
				Type:             schema.TypeString,
				Required:         true,
				ForceNew:         true,
				DiffSuppressFunc: caseInsensitive(),
				Description:      "The login of the user to invite to the enterprise.",
			},
			"reinvite": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     true,
				Description: "Whether to send a new invitation when a previously managed invitation is no longer pending.",
			},
			"status": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The state of the membership: 'pending' while the invitation is outstanding, 'active' once it has been accepted, or 'expired' when a missing invitation is retained because 'reinvite' is false.",
			},
			"invitation_id": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The node ID of the pending enterprise invitation. Empty once the invitation has been accepted or is no longer pending.",
			},
		},
	}
}

// enterpriseMemberState is the current state of a user with respect to an enterprise,
// resolved by getEnterpriseMemberState.
type enterpriseMemberState struct {
	// enterpriseID is the node ID of the enterprise. Always set when the enterprise resolves,
	// even if the user was not found.
	enterpriseID string
	// status is enterpriseMemberStatusPending, enterpriseMemberStatusActive, or empty when the
	// API reports that the user is neither invited nor a member. enterpriseMemberStatusExpired is
	// a Terraform state value and is not returned by getEnterpriseMemberState.
	status string
	// login is the user's login as returned by the API, preserving its canonical casing.
	login string
	// invitationID is the node ID of the pending invitation. Only set when status is pending.
	invitationID string
	// userID is the node ID of the User. Only set when status is active.
	userID string
}

// getEnterpriseMemberState looks up whether username is an enterprise member or has a pending
// invitation. Both are fetched in one query because the enterprise ID is needed either way.
func getEnterpriseMemberState(ctx context.Context, client *githubv4.Client, maxPerPage int, enterpriseSlug, username string) (*enterpriseMemberState, error) {
	var query struct {
		Enterprise struct {
			ID      githubv4.String
			Members struct {
				Nodes []struct {
					User struct {
						ID    githubv4.String
						Login githubv4.String
					} `graphql:"... on User"`
					EnterpriseUserAccount struct {
						Login githubv4.String
						User  struct {
							ID githubv4.String
						}
					} `graphql:"... on EnterpriseUserAccount"`
				}
				PageInfo PageInfo
			} `graphql:"members(first: $first, after: $membersCursor, query: $login)"`
			OwnerInfo struct {
				PendingUnaffiliatedMemberInvitations struct {
					Nodes []struct {
						ID      githubv4.String
						Invitee struct {
							Login githubv4.String
						}
					}
					PageInfo PageInfo
				} `graphql:"pendingUnaffiliatedMemberInvitations(first: $first, after: $invitationsCursor, query: $login)"`
			}
		} `graphql:"enterprise(slug: $slug)"`
	}

	variables := map[string]any{
		"slug":              githubv4.String(enterpriseSlug),
		"login":             githubv4.String(username),
		"first":             githubv4.Int(maxPerPage),
		"membersCursor":     (*githubv4.String)(nil),
		"invitationsCursor": (*githubv4.String)(nil),
	}

	state := &enterpriseMemberState{}

	for {
		if err := client.Query(ctx, &query, variables); err != nil {
			return nil, err
		}

		state.enterpriseID = string(query.Enterprise.ID)

		// The 'query' argument is a fuzzy search, so match the login exactly. A user cannot be
		// both a member and a pending invitee, so returning on the first match is safe.
		for _, node := range query.Enterprise.Members.Nodes {
			login := string(node.User.Login)
			userID := string(node.User.ID)
			if userID == "" {
				// The decoder can populate login in both inline fragments. The top-level
				// User ID distinguishes a User from an EnterpriseUserAccount, which
				// wraps the underlying User.
				login = string(node.EnterpriseUserAccount.Login)
				userID = string(node.EnterpriseUserAccount.User.ID)
			}

			if strings.EqualFold(login, username) {
				state.status = enterpriseMemberStatusActive
				state.login = login
				state.userID = userID
				return state, nil
			}
		}

		for _, node := range query.Enterprise.OwnerInfo.PendingUnaffiliatedMemberInvitations.Nodes {
			if strings.EqualFold(string(node.Invitee.Login), username) {
				state.status = enterpriseMemberStatusPending
				state.login = string(node.Invitee.Login)
				state.invitationID = string(node.ID)
				return state, nil
			}
		}

		membersPage := query.Enterprise.Members.PageInfo
		invitationsPage := query.Enterprise.OwnerInfo.PendingUnaffiliatedMemberInvitations.PageInfo
		if !membersPage.HasNextPage && !invitationsPage.HasNextPage {
			return state, nil
		}

		// The two connections are paginated independently. Whichever one is exhausted keeps its
		// cursor and re-returns its final page, which is harmless because it holds no match.
		if membersPage.HasNextPage {
			variables["membersCursor"] = new(membersPage.EndCursor)
		}
		if invitationsPage.HasNextPage {
			variables["invitationsCursor"] = new(invitationsPage.EndCursor)
		}
	}
}

// isEnterpriseNotFoundError reports whether err is GitHub's response to an enterprise slug that
// does not resolve.
func isEnterpriseNotFoundError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "Could not resolve to a Business")
}

func resourceGithubEnterpriseMemberCreate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	owner := meta.(*Owner)
	client := owner.v4client

	enterpriseSlug := d.Get("enterprise_slug").(string)
	username := d.Get("username").(string)

	state, err := getEnterpriseMemberState(ctx, client, owner.maxPerPage, enterpriseSlug, username)
	if err != nil {
		return diag.FromErr(err)
	}

	switch state.status {
	case enterpriseMemberStatusActive, enterpriseMemberStatusPending:
		// Adopt the existing membership or invitation so that create is idempotent.
		tflog.Info(ctx, "Not inviting user because they already have an enterprise membership", map[string]any{
			"enterprise_slug": enterpriseSlug,
			"username":        username,
			"status":          state.status,
		})
	default:
		var mutation struct {
			InviteEnterpriseMember struct {
				Invitation struct {
					ID githubv4.String
				}
			} `graphql:"inviteEnterpriseMember(input: $input)"`
		}

		invitee := githubv4.String(username)
		input := githubv4.InviteEnterpriseMemberInput{
			EnterpriseID: githubv4.ID(state.enterpriseID),
			Invitee:      &invitee,
		}

		if err := client.Mutate(ctx, &mutation, input, nil); err != nil {
			return diag.FromErr(err)
		}

		state.status = enterpriseMemberStatusPending
		state.invitationID = string(mutation.InviteEnterpriseMember.Invitation.ID)
	}

	id, err := buildID(enterpriseSlug, username)
	if err != nil {
		return diag.FromErr(err)
	}
	d.SetId(id)

	if err := d.Set("status", state.status); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("invitation_id", state.invitationID); err != nil {
		return diag.FromErr(err)
	}

	return nil
}

func resourceGithubEnterpriseMemberRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	owner := meta.(*Owner)
	client := owner.v4client

	enterpriseSlug, username, err := parseID2(d.Id())
	if err != nil {
		return diag.FromErr(err)
	}

	state, err := getEnterpriseMemberState(ctx, client, owner.maxPerPage, enterpriseSlug, username)
	if err != nil {
		if isEnterpriseNotFoundError(err) {
			tflog.Info(ctx, "Removing enterprise member from state because the enterprise no longer exists in GitHub", map[string]any{
				"enterprise_slug": enterpriseSlug,
				"username":        username,
			})
			d.SetId("")
			return nil
		}
		return diag.FromErr(err)
	}

	if state.status == "" {
		invitationWasManaged := d.Get("invitation_id").(string) != "" || d.Get("status").(string) == enterpriseMemberStatusExpired
		if !d.Get("reinvite").(bool) && invitationWasManaged {
			tflog.Info(ctx, "Retaining expired enterprise invitation in state because reinvitation is disabled", map[string]any{
				"enterprise_slug": enterpriseSlug,
				"username":        username,
			})
			if err := d.Set("status", enterpriseMemberStatusExpired); err != nil {
				return diag.FromErr(err)
			}
			if err := d.Set("invitation_id", ""); err != nil {
				return diag.FromErr(err)
			}
			return nil
		}

		tflog.Info(ctx, "Removing enterprise member from state because they are neither a member nor an invitee", map[string]any{
			"enterprise_slug": enterpriseSlug,
			"username":        username,
		})
		d.SetId("")
		return nil
	}

	if err := d.Set("enterprise_slug", enterpriseSlug); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("username", state.login); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("status", state.status); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("invitation_id", state.invitationID); err != nil {
		return diag.FromErr(err)
	}

	return nil
}

func resourceGithubEnterpriseMemberUpdate(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	if d.Get("reinvite").(bool) {
		return resourceGithubEnterpriseMemberCreate(ctx, d, meta)
	}

	return resourceGithubEnterpriseMemberRead(ctx, d, meta)
}

func resourceGithubEnterpriseMemberDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	owner := meta.(*Owner)
	client := owner.v4client

	enterpriseSlug := d.Get("enterprise_slug").(string)
	username := d.Get("username").(string)

	// Look up the current state rather than trusting it, because the invitation may have been
	// accepted, cancelled or expired since the last refresh.
	state, err := getEnterpriseMemberState(ctx, client, owner.maxPerPage, enterpriseSlug, username)
	if err != nil {
		if isEnterpriseNotFoundError(err) {
			return nil
		}
		return diag.FromErr(err)
	}

	switch state.status {
	case enterpriseMemberStatusPending:
		tflog.Info(ctx, "Cancelling pending enterprise invitation", map[string]any{
			"enterprise_slug": enterpriseSlug,
			"username":        username,
		})

		var mutation struct {
			CancelEnterpriseMemberInvitation struct {
				ClientMutationID githubv4.String
			} `graphql:"cancelEnterpriseMemberInvitation(input: $input)"`
		}

		input := githubv4.CancelEnterpriseMemberInvitationInput{
			InvitationID: githubv4.ID(state.invitationID),
		}

		if err := client.Mutate(ctx, &mutation, input, nil); err != nil {
			return diag.FromErr(err)
		}
	case enterpriseMemberStatusActive:
		if state.userID == "" {
			return diag.FromErr(fmt.Errorf("cannot remove %q from enterprise %q: the underlying user account could not be resolved", username, enterpriseSlug))
		}

		tflog.Info(ctx, "Removing user from enterprise", map[string]any{
			"enterprise_slug": enterpriseSlug,
			"username":        username,
		})

		var mutation struct {
			RemoveEnterpriseMember struct {
				ClientMutationID githubv4.String
			} `graphql:"removeEnterpriseMember(input: $input)"`
		}

		input := githubv4.RemoveEnterpriseMemberInput{
			EnterpriseID: githubv4.ID(state.enterpriseID),
			UserID:       githubv4.ID(state.userID),
		}

		if err := client.Mutate(ctx, &mutation, input, nil); err != nil {
			return diag.FromErr(err)
		}
	default:
		tflog.Info(ctx, "Not removing user from enterprise because they are neither a member nor an invitee", map[string]any{
			"enterprise_slug": enterpriseSlug,
			"username":        username,
		})
	}

	return nil
}
