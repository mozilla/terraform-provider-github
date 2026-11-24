package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestEnterpriseCustomPropertyLifecycle(t *testing.T) {
	t.Parallel()

	const resourceType = "github_enterprise_custom_property"
	const address = resourceType + ".test"
	const config = `
resource "github_enterprise_custom_property" "test" {
  enterprise_slug = "example"
  property_name   = "compliance#tier"
%s
}
`
	var mu sync.Mutex
	var property map[string]any
	var writes []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if req.URL.EscapedPath() != "/api/v3/enterprises/example/properties/schema/compliance%23tier" {
			t.Errorf("unexpected request path %q", req.URL.EscapedPath())
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		switch req.Method {
		case http.MethodPut:
			var change map[string]any
			if err := json.NewDecoder(req.Body).Decode(&change); err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			writes = append(writes, change)
			property = change
		case http.MethodGet:
			if property == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
		case http.MethodDelete:
			if property == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			property = nil
			w.WriteHeader(http.StatusNoContent)
			return
		default:
			t.Errorf("unexpected request method %s", req.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(property); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()

	meta := &Owner{v3client: mustCreateTestGitHubClient(t, server.URL+"/api/v3/")}
	factory := func() (*schema.Provider, error) {
		p := &schema.Provider{
			ResourcesMap: map[string]*schema.Resource{resourceType: resourceGithubEnterpriseCustomProperty()},
			ConfigureContextFunc: func(context.Context, *schema.ResourceData) (any, diag.Diagnostics) {
				return meta, nil
			},
		}
		return p, p.InternalValidate()
	}
	resource.UnitTest(t, resource.TestCase{
		ProviderFactories: map[string]func() (*schema.Provider, error){"github": factory},
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(config, `
  value_type              = "multi_select"
  description             = "Compliance frameworks"
  required                = true
  allowed_values          = ["soc2", "internal", "iso27001"]
  default_values          = ["internal", "soc2"]
  values_editable_by      = "org_and_repo_actors"
  require_explicit_values = true
`),
			},
			{ResourceName: address, ImportState: true, ImportStateVerify: true},
			{
				Config: fmt.Sprintf(config, `
  value_type     = "multi_select"
  allowed_values = ["soc2", "internal", "iso27001"]
`),
			},
			{ResourceName: address, ImportState: true, ImportStateVerify: true},
		},
	})

	r := resourceGithubEnterpriseCustomProperty()
	d := r.Data(nil)
	d.SetId("example:compliance#tier")
	if diags := r.ReadContext(t.Context(), d, meta); diags.HasError() || d.Id() != "" {
		t.Errorf("read of deleted property returned diagnostics %v and ID %q", diags, d.Id())
	}
	d.SetId("example:compliance#tier")
	if diags := r.DeleteContext(t.Context(), d, meta); diags.HasError() || d.Id() != "" {
		t.Errorf("delete of missing property returned diagnostics %v and ID %q", diags, d.Id())
	}

	mu.Lock()
	defer mu.Unlock()
	if property != nil {
		t.Error("property was not deleted")
	}
	if len(writes) != 2 {
		t.Fatalf("got %d writes, want 2", len(writes))
	}
	if !reflect.DeepEqual(writes[0]["default_value"], []any{"internal", "soc2"}) || writes[0]["require_explicit_values"] != true {
		t.Errorf("unexpected create payload: %#v", writes[0])
	}
	if !reflect.DeepEqual(writes[1], map[string]any{
		"value_type":              "multi_select",
		"allowed_values":          []any{"soc2", "internal", "iso27001"},
		"required":                false,
		"require_explicit_values": false,
		"values_editable_by":      "org_actors",
		"default_value":           nil,
		"description":             nil,
	}) {
		t.Errorf("unexpected update payload: %#v", writes[1])
	}
}
