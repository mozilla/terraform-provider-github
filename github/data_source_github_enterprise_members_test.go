package github

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccDataSourceGithubEnterpriseMembers(t *testing.T) {
	t.Parallel()

	config := fmt.Sprintf(`
data "github_enterprise_members" "test" {
  enterprise_slug = %q
}
`, testAccConf.enterpriseSlug)

	resource.Test(t, resource.TestCase{
		PreCheck:          func() { skipUnlessEnterprise(t) },
		ProviderFactories: providerFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.github_enterprise_members.test", "enterprise_slug", testAccConf.enterpriseSlug),
					resource.TestCheckResourceAttrSet("data.github_enterprise_members.test", "members.#"),
					resource.TestCheckResourceAttrSet("data.github_enterprise_members.test", "members.0.id"),
					resource.TestCheckResourceAttrSet("data.github_enterprise_members.test", "members.0.node_id"),
					resource.TestCheckResourceAttrSet("data.github_enterprise_members.test", "members.0.login"),
				),
			},
		},
	})
}

func TestDataSourceGithubEnterpriseMembersRead(t *testing.T) {
	t.Parallel()

	requests := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/graphql", func(w http.ResponseWriter, req *http.Request) {
		requests++
		body := mustRead(req.Body)
		if !strings.Contains(body, `enterprise(slug: $slug)`) {
			t.Errorf("query does not select an enterprise by slug: %s", body)
		}
		if !strings.Contains(body, `members(first: $first, after: $after)`) {
			t.Errorf("query does not select paginated members: %s", body)
		}

		w.Header().Set("Content-Type", "application/json")
		if requests == 1 {
			mustWrite(w, `{"data":{"enterprise":{"id":"E_enterprise","members":{"nodes":[{"id":"U_regular","databaseId":101,"login":"octocat"}],"pageInfo":{"endCursor":"cursor-1","hasNextPage":true,"hasPreviousPage":false,"startCursor":"cursor-0"}}}}}`)
			return
		}

		if !strings.Contains(body, `"after":"cursor-1"`) {
			t.Errorf("second query does not use the next page cursor: %s", body)
		}
		mustWrite(w, `{"data":{"enterprise":{"id":"E_enterprise","members":{"nodes":[{"login":"monalisa_example_com","user":{"id":"U_managed","databaseId":202}}],"pageInfo":{"endCursor":"cursor-2","hasNextPage":false,"hasPreviousPage":true,"startCursor":"cursor-1"}}}}}`)
	})

	data := schema.TestResourceDataRaw(t, dataSourceGithubEnterpriseMembers().Schema, map[string]any{
		"enterprise_slug": "example-enterprise",
	})
	meta := &Owner{v4client: newTestGraphQLClient(mux), maxPerPage: 100}

	if diags := dataSourceGithubEnterpriseMembersRead(t.Context(), data, meta); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if got, want := data.Id(), "E_enterprise"; got != want {
		t.Errorf("id = %q, want %q", got, want)
	}
	if got, want := requests, 2; got != want {
		t.Errorf("GraphQL requests = %d, want %d", got, want)
	}

	members := data.Get("members").([]any)
	want := []map[string]any{
		{"id": 101, "node_id": "U_regular", "login": "octocat"},
		{"id": 202, "node_id": "U_managed", "login": "monalisa_example_com"},
	}
	if got := len(members); got != len(want) {
		t.Fatalf("members length = %d, want %d", got, len(want))
	}
	for i, member := range members {
		got := member.(map[string]any)
		for key, value := range want[i] {
			if got[key] != value {
				t.Errorf("members[%d][%q] = %#v, want %#v", i, key, got[key], value)
			}
		}
	}
}
