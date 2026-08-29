package github

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccGithubEnterpriseMember(t *testing.T) {
	t.Parallel()

	const resourceName = "github_enterprise_member.test"

	config := `
resource "github_enterprise_member" "test" {
	enterprise_slug = "%s"
	username        = "%s"
}
`

	// These subtests are not parallel: they share the enterprise and the small pool of external
	// test users, so concurrent invitations for the same user would collide.
	t.Run("invites a user to the enterprise", func(t *testing.T) {
		resource.Test(t, resource.TestCase{
			PreCheck: func() {
				skipUnlessEnterprise(t)
				// An EMU enterprise provisions members through its identity provider and cannot
				// invite outside users.
				skipIfEMUEnterprise(t)
				skipUnlessHasExternalUser1(t)
			},
			ProviderFactories: providerFactories,
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(config, testAccConf.enterpriseSlug, testAccConf.testExternalUser1),
					ConfigStateChecks: []statecheck.StateCheck{
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("enterprise_slug"), knownvalue.StringExact(testAccConf.enterpriseSlug)),
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("username"), knownvalue.StringExact(testAccConf.testExternalUser1)),
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("reinvite"), knownvalue.Bool(true)),
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("status"), knownvalue.StringExact("pending")),
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("invitation_id"), knownvalue.NotNull()),
					},
				},
				{
					ResourceName:      resourceName,
					ImportState:       true,
					ImportStateVerify: true,
				},
			},
		})
	})

	t.Run("forces a new resource when the username changes", func(t *testing.T) {
		resource.Test(t, resource.TestCase{
			PreCheck: func() {
				skipUnlessEnterprise(t)
				skipIfEMUEnterprise(t)
				skipUnlessHasExternalUser1(t)
				skipUnlessHasExternalUser2(t)
			},
			ProviderFactories: providerFactories,
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(config, testAccConf.enterpriseSlug, testAccConf.testExternalUser1),
				},
				{
					Config: fmt.Sprintf(config, testAccConf.enterpriseSlug, testAccConf.testExternalUser2),
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PreApply: []plancheck.PlanCheck{
							plancheck.ExpectResourceAction(resourceName, plancheck.ResourceActionDestroyBeforeCreate),
						},
					},
					ConfigStateChecks: []statecheck.StateCheck{
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("username"), knownvalue.StringExact(testAccConf.testExternalUser2)),
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("status"), knownvalue.StringExact("pending")),
					},
				},
			},
		})
	})

	t.Run("adopts an invitation that already exists", func(t *testing.T) {
		resource.Test(t, resource.TestCase{
			PreCheck: func() {
				skipUnlessEnterprise(t)
				skipIfEMUEnterprise(t)
				skipUnlessHasExternalUser2(t)
			},
			ProviderFactories: providerFactories,
			Steps: []resource.TestStep{
				{
					// Inviting out of band first means create runs against an invitation that
					// already exists, which the invite mutation on its own would reject.
					PreConfig: func() {
						mustInviteEnterpriseMember(t, testAccConf.enterpriseSlug, testAccConf.testExternalUser2)
					},
					Config: fmt.Sprintf(config, testAccConf.enterpriseSlug, testAccConf.testExternalUser2),
					ConfigStateChecks: []statecheck.StateCheck{
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("status"), knownvalue.StringExact("pending")),
						statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("invitation_id"), knownvalue.NotNull()),
					},
				},
			},
		})
	})
}

func TestResourceGithubEnterpriseMemberReadExpiredInvitation(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		reinvite            bool
		initialStatus       string
		initialInvitationID string
		wantID              string
		wantStatus          string
		wantInvitationID    string
	}{
		"removes the resource from state when reinvitation is enabled": {
			reinvite:            true,
			initialStatus:       enterpriseMemberStatusPending,
			initialInvitationID: "I_invitation",
			wantID:              "",
			wantStatus:          enterpriseMemberStatusPending,
			wantInvitationID:    "I_invitation",
		},
		"retains the expired invitation when reinvitation is disabled": {
			reinvite:            false,
			initialStatus:       enterpriseMemberStatusPending,
			initialInvitationID: "I_invitation",
			wantID:              "example-enterprise:octocat",
			wantStatus:          enterpriseMemberStatusExpired,
			wantInvitationID:    "",
		},
		"retains the resource on subsequent refreshes": {
			reinvite:      false,
			initialStatus: enterpriseMemberStatusExpired,
			wantID:        "example-enterprise:octocat",
			wantStatus:    enterpriseMemberStatusExpired,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			mux := http.NewServeMux()
			mux.HandleFunc("/graphql", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				mustWrite(w, `{"data":{"enterprise":{"id":"E_enterprise","members":{"nodes":[],"pageInfo":{"hasNextPage":false}},"ownerInfo":{"pendingUnaffiliatedMemberInvitations":{"nodes":[],"pageInfo":{"hasNextPage":false}}}}}}`)
			})

			data := schema.TestResourceDataRaw(t, resourceGithubEnterpriseMember().Schema, map[string]any{
				"enterprise_slug": "example-enterprise",
				"username":        "octocat",
				"reinvite":        test.reinvite,
				"status":          test.initialStatus,
				"invitation_id":   test.initialInvitationID,
			})
			data.SetId("example-enterprise:octocat")
			meta := &Owner{v4client: newTestGraphQLClient(mux), maxPerPage: 100}

			if diags := resourceGithubEnterpriseMemberRead(t.Context(), data, meta); diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}

			if got := data.Id(); got != test.wantID {
				t.Errorf("id = %q, want %q", got, test.wantID)
			}
			if got := data.Get("status").(string); got != test.wantStatus {
				t.Errorf("status = %q, want %q", got, test.wantStatus)
			}
			if got := data.Get("invitation_id").(string); got != test.wantInvitationID {
				t.Errorf("invitation_id = %q, want %q", got, test.wantInvitationID)
			}
		})
	}
}

func TestResourceGithubEnterpriseMemberUpdateReinvite(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		reinvite         bool
		initialStatus    string
		invitationID     string
		wantRequests     int
		wantStatus       string
		wantInvitationID string
	}{
		"disabled": {
			reinvite:         false,
			initialStatus:    enterpriseMemberStatusPending,
			invitationID:     "I_invitation",
			wantRequests:     1,
			wantStatus:       enterpriseMemberStatusExpired,
			wantInvitationID: "",
		},
		"enabled": {
			reinvite:         true,
			initialStatus:    enterpriseMemberStatusExpired,
			wantRequests:     2,
			wantStatus:       enterpriseMemberStatusPending,
			wantInvitationID: "I_new_invitation",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			requests := 0
			mux := http.NewServeMux()
			mux.HandleFunc("/graphql", func(w http.ResponseWriter, _ *http.Request) {
				requests++
				w.Header().Set("Content-Type", "application/json")
				if requests == 1 {
					mustWrite(w, `{"data":{"enterprise":{"id":"E_enterprise","members":{"nodes":[],"pageInfo":{"hasNextPage":false}},"ownerInfo":{"pendingUnaffiliatedMemberInvitations":{"nodes":[],"pageInfo":{"hasNextPage":false}}}}}}`)
					return
				}
				mustWrite(w, `{"data":{"inviteEnterpriseMember":{"invitation":{"id":"I_new_invitation"}}}}`)
			})

			data := schema.TestResourceDataRaw(t, resourceGithubEnterpriseMember().Schema, map[string]any{
				"enterprise_slug": "example-enterprise",
				"username":        "octocat",
				"reinvite":        test.reinvite,
				"status":          test.initialStatus,
				"invitation_id":   test.invitationID,
			})
			data.SetId("example-enterprise:octocat")
			meta := &Owner{v4client: newTestGraphQLClient(mux), maxPerPage: 100}

			if diags := resourceGithubEnterpriseMemberUpdate(t.Context(), data, meta); diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}

			if requests != test.wantRequests {
				t.Errorf("GraphQL requests = %d, want %d", requests, test.wantRequests)
			}
			if got := data.Get("status").(string); got != test.wantStatus {
				t.Errorf("status = %q, want %q", got, test.wantStatus)
			}
			if got := data.Get("invitation_id").(string); got != test.wantInvitationID {
				t.Errorf("invitation_id = %q, want %q", got, test.wantInvitationID)
			}
		})
	}
}
