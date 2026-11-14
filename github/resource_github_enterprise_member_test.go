package github

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	gh "github.com/google/go-github/v92/github"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"

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
			meta := enterpriseMemberTestOwner(t, mux)

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

func TestGetEnterpriseMemberStateEnterpriseUserAccount(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/graphql", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		mustWrite(w, `{"data":{"enterprise":{"id":"E_enterprise","members":{"nodes":[{"login":"jbuck","user":{"id":"U_jbuck"}}],"pageInfo":{"hasNextPage":false}},"ownerInfo":{"pendingUnaffiliatedMemberInvitations":{"nodes":[],"pageInfo":{"hasNextPage":false}}}}}}`)
	})

	state, err := getEnterpriseMemberState(t.Context(), newTestGraphQLClient(mux), 100, "example-enterprise", "jbuck")
	if err != nil {
		t.Fatalf("getting enterprise member state: %v", err)
	}
	if state.status != enterpriseMemberStatusActive {
		t.Errorf("status = %q, want %q", state.status, enterpriseMemberStatusActive)
	}
	if state.login != "jbuck" {
		t.Errorf("login = %q, want jbuck", state.login)
	}
	if state.userID != "U_jbuck" {
		t.Errorf("user ID = %q, want U_jbuck", state.userID)
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
			meta := enterpriseMemberTestOwner(t, mux)

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

func enterpriseMemberTestOwner(t *testing.T, mux *http.ServeMux) *Owner {
	t.Helper()
	mux.HandleFunc("/users/", func(w http.ResponseWriter, _ *http.Request) {
		mustWrite(w, `{"id":123,"node_id":"U_octocat","login":"octocat"}`)
	})
	mux.HandleFunc("/user/", func(w http.ResponseWriter, _ *http.Request) {
		mustWrite(w, `{"id":123,"node_id":"U_octocat","login":"octocat"}`)
	})
	client, err := gh.NewClient(gh.WithHTTPClient(&http.Client{Transport: localRoundTripper{handler: mux}}))
	if err != nil {
		t.Fatal(err)
	}
	return &Owner{v3client: client, v4client: newTestGraphQLClient(mux), maxPerPage: 100}
}

func TestEnterpriseMemberRename(t *testing.T) {
	t.Parallel()
	for _, status := range []string{enterpriseMemberStatusActive, enterpriseMemberStatusPending} {
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			mux := http.NewServeMux()
			queries := 0
			mux.HandleFunc("/graphql", func(w http.ResponseWriter, req *http.Request) {
				var body struct {
					Query     string
					Variables map[string]any
				}
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if strings.Contains(body.Query, "mutation") {
					t.Errorf("unexpected mutation: %s", body.Query)
				}
				if body.Variables["login"] != "new-login" {
					t.Errorf("membership lookup = %v, want new-login", body.Variables["login"])
				}
				queries++
				if status == enterpriseMemberStatusActive {
					mustWrite(w, `{"data":{"enterprise":{"id":"E_enterprise","members":{"nodes":[{"id":"U_octocat","login":"new-login"}],"pageInfo":{"hasNextPage":false}},"ownerInfo":{"pendingUnaffiliatedMemberInvitations":{"nodes":[],"pageInfo":{"hasNextPage":false}}}}}}`)
				} else {
					mustWrite(w, `{"data":{"enterprise":{"id":"E_enterprise","members":{"nodes":[],"pageInfo":{"hasNextPage":false}},"ownerInfo":{"pendingUnaffiliatedMemberInvitations":{"nodes":[{"id":"I_invitation","invitee":{"login":"new-login"}}],"pageInfo":{"hasNextPage":false}}}}}}`)
				}
			})
			mux.HandleFunc("/user/123", func(w http.ResponseWriter, _ *http.Request) {
				mustWrite(w, `{"id":123,"node_id":"U_octocat","login":"new-login"}`)
			})
			mux.HandleFunc("/users/", func(w http.ResponseWriter, req *http.Request) {
				t.Errorf("resolved old username: %s", req.URL.Path)
				w.WriteHeader(http.StatusNotFound)
			})
			client, err := gh.NewClient(gh.WithHTTPClient(&http.Client{Transport: localRoundTripper{handler: mux}}))
			if err != nil {
				t.Fatal(err)
			}
			owner := &Owner{v3client: client, v4client: newTestGraphQLClient(mux), maxPerPage: 100}
			r := resourceGithubEnterpriseMember()
			d := schema.TestResourceDataRaw(t, r.Schema, map[string]any{"enterprise_slug": "example-enterprise", "username": "old-login", "user_id": 123, "status": status, "reinvite": false})
			d.SetId("example-enterprise:old-login")
			if diags := r.ReadContext(t.Context(), d, owner); diags.HasError() {
				t.Fatal(diags)
			}
			if got := d.Get("username"); got != "new-login" {
				t.Errorf("username = %v", got)
			}
			if d.Id() == "" {
				t.Fatal("rename removed resource from state")
			}
			if d.Get("status") != status {
				t.Errorf("status = %v", d.Get("status"))
			}
			if queries != 1 {
				t.Errorf("queries = %d", queries)
			}
			for _, config := range []map[string]any{
				{"enterprise_slug": "example-enterprise", "username": "new-login", "reinvite": false},
				{"enterprise_slug": "example-enterprise", "user_id": 123, "reinvite": false},
			} {
				diff, err := r.Diff(t.Context(), d.State(), terraform.NewResourceConfigRaw(config), owner)
				if err != nil {
					t.Fatal(err)
				}
				if diff != nil && !diff.Empty() {
					t.Errorf("rename should have empty plan: %#v", diff)
				}
			}
		})
	}
}

func TestEnterpriseMemberIdentityPlan(t *testing.T) {
	t.Parallel()
	r := resourceGithubEnterpriseMember()
	if err := r.InternalValidate(nil, true); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	owner := enterpriseMemberTestOwner(t, mux)
	// This login belongs to a different account than the stored member.
	d := schema.TestResourceDataRaw(t, r.Schema, map[string]any{"enterprise_slug": "example", "username": "previous", "user_id": 456})
	d.SetId("example:previous")
	for _, config := range []map[string]any{
		{"enterprise_slug": "example", "username": "octocat"},
		{"enterprise_slug": "example", "user_id": 789},
	} {
		diff, err := r.Diff(t.Context(), d.State(), terraform.NewResourceConfigRaw(config), owner)
		if err != nil {
			t.Fatal(err)
		}
		if diff == nil || !diff.RequiresNew() {
			t.Errorf("account switch should require replacement: %#v", diff)
		}
	}
	for _, config := range []map[string]any{
		{"enterprise_slug": "example"},
		{"enterprise_slug": "example", "username": "octocat", "user_id": 123},
	} {
		if diags := r.Validate(terraform.NewResourceConfigRaw(config)); !diags.HasError() {
			t.Errorf("expected invalid identity configuration: %v", config)
		}
	}
}

func TestEnterpriseMemberLegacyRead(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/graphql", func(w http.ResponseWriter, _ *http.Request) {
		mustWrite(w, `{"data":{"enterprise":{"id":"E_enterprise","members":{"nodes":[{"id":"U_octocat","login":"octocat"}],"pageInfo":{"hasNextPage":false}},"ownerInfo":{"pendingUnaffiliatedMemberInvitations":{"nodes":[],"pageInfo":{"hasNextPage":false}}}}}}`)
	})
	owner := enterpriseMemberTestOwner(t, mux)
	r := resourceGithubEnterpriseMember()
	// An imported resource contains only its legacy enterprise:username ID.
	d := schema.TestResourceDataRaw(t, r.Schema, map[string]any{})
	d.SetId("example:octocat")
	if diags := r.ReadContext(t.Context(), d, owner); diags.HasError() {
		t.Fatal(diags)
	}
	if d.Get("user_id") != 123 {
		t.Errorf("user_id = %v", d.Get("user_id"))
	}
	diff, err := r.Diff(t.Context(), d.State(), terraform.NewResourceConfigRaw(map[string]any{"enterprise_slug": "example", "username": "octocat"}), owner)
	if err != nil {
		t.Fatal(err)
	}
	if diff != nil && !diff.Empty() {
		t.Errorf("legacy upgrade should have no changes: %#v", diff)
	}
}

func TestEnterpriseMemberAccountSwitchCreate(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/graphql", func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			Query     string
			Variables map[string]any
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if strings.Contains(body.Query, "mutation") {
			t.Errorf("existing new account should be adopted: %s", body.Query)
		}
		if body.Variables["login"] != "octocat" {
			t.Errorf("looked up old account: %v", body.Variables)
		}
		mustWrite(w, `{"data":{"enterprise":{"id":"E_enterprise","members":{"nodes":[{"id":"U_octocat","login":"octocat"}],"pageInfo":{"hasNextPage":false}},"ownerInfo":{"pendingUnaffiliatedMemberInvitations":{"nodes":[],"pageInfo":{"hasNextPage":false}}}}}}`)
	})
	owner := enterpriseMemberTestOwner(t, mux)
	r := resourceGithubEnterpriseMember()
	d := schema.TestResourceDataRaw(t, r.Schema, map[string]any{"enterprise_slug": "example", "username": "previous", "user_id": 456})
	d.SetId("example:previous")
	diff, err := r.Diff(t.Context(), d.State(), terraform.NewResourceConfigRaw(map[string]any{"enterprise_slug": "example", "username": "octocat"}), owner)
	if err != nil {
		t.Fatal(err)
	}
	if diff == nil || !diff.RequiresNew() {
		t.Fatal("expected replacement")
	}
	state, diags := r.Apply(t.Context(), nil, diff, owner)
	if diags.HasError() {
		t.Fatal(diags)
	}
	if state.Attributes["user_id"] != "123" || state.Attributes["username"] != "octocat" {
		t.Errorf("replacement adopted wrong account: %#v", state.Attributes)
	}
}

func TestEnterpriseMemberDeleteAfterUsernameReuse(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/user/123", func(w http.ResponseWriter, _ *http.Request) {
		mustWrite(w, `{"id":123,"node_id":"U_original","login":"new-login"}`)
	})
	mux.HandleFunc("/users/", func(w http.ResponseWriter, req *http.Request) {
		t.Errorf("must not look up reused old username: %s", req.URL.Path)
		mustWrite(w, `{"id":456,"node_id":"U_other","login":"old-login"}`)
	})
	mutations := 0
	mux.HandleFunc("/graphql", func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			Query     string
			Variables map[string]json.RawMessage
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if strings.Contains(body.Query, "mutation") {
			mutations++
			var input struct{ UserID string }
			if err := json.Unmarshal(body.Variables["input"], &input); err != nil {
				t.Error(err)
			}
			if input.UserID != "U_original" {
				t.Errorf("removed account = %q, want U_original", input.UserID)
			}
			mustWrite(w, `{"data":{"removeEnterpriseMember":{"clientMutationId":null}}}`)
			return
		}
		mustWrite(w, `{"data":{"enterprise":{"id":"E_enterprise","members":{"nodes":[{"id":"U_original","login":"new-login"}],"pageInfo":{"hasNextPage":false}},"ownerInfo":{"pendingUnaffiliatedMemberInvitations":{"nodes":[],"pageInfo":{"hasNextPage":false}}}}}}`)
	})
	client, err := gh.NewClient(gh.WithHTTPClient(&http.Client{Transport: localRoundTripper{handler: mux}}))
	if err != nil {
		t.Fatal(err)
	}
	owner := &Owner{v3client: client, v4client: newTestGraphQLClient(mux), maxPerPage: 100}
	r := resourceGithubEnterpriseMember()
	d := schema.TestResourceDataRaw(t, r.Schema, map[string]any{"enterprise_slug": "example", "user_id": 123, "username": "old-login"})
	d.SetId("example:old-login")
	if diags := r.DeleteContext(t.Context(), d, owner); diags.HasError() {
		t.Fatal(diags)
	}
	if mutations != 1 {
		t.Errorf("delete mutations = %d", mutations)
	}
}
