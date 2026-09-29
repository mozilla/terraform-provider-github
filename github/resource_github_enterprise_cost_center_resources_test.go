package github

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestCostCenterResourcesBatchRequests(t *testing.T) {
	t.Parallel()
	for _, count := range []int{0, 1, 49, 50, 51, 100, 151} {
		for _, mixed := range []bool{false, true} {
			for _, failAt := range []int{0, 2} {
				t.Run(fmt.Sprintf("count=%d/mixed=%t/failAt=%d", count, mixed, failAt), func(t *testing.T) {
					t.Parallel()
					var actual []costCenterResource
					var desired costCenterResourcesChange
					want := map[string]costCenterResourcesChange{}
					for _, method := range []string{http.MethodDelete, http.MethodPost} {
						var change costCenterResourcesChange
						for i := 0; i < count; i++ {
							kind := 0
							if mixed {
								kind = i % 4
							}
							name := fmt.Sprintf("%s-%d", strings.ToLower(method), i)
							fields := []*[]string{&change.Users, &change.Organizations, &change.Repositories, &change.EnterpriseTeams}
							*fields[kind] = append(*fields[kind], name)
							if method == http.MethodDelete {
								actual = append(actual, costCenterResource{Type: []string{"User", "Organization", "Repo", "Team"}[kind], Name: name})
							}
						}
						want[method] = change
						if method == http.MethodPost {
							desired = change
						}
					}
					got := map[string]costCenterResourcesChange{}
					var methods []string
					const path = "/enterprises/example/settings/billing/cost-centers/cc-1"
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
						if req.URL.Path == path && req.Method == http.MethodGet {
							json.NewEncoder(w).Encode(costCenter{Resources: actual})
							return
						}
						if req.URL.Path != path+"/resource" || (req.Method != http.MethodPost && req.Method != http.MethodDelete) {
							t.Errorf("unexpected request: %s %s", req.Method, req.URL.Path)
							w.WriteHeader(http.StatusNotFound)
							return
						}
						var batch costCenterResourcesChange
						if err := json.NewDecoder(req.Body).Decode(&batch); err != nil {
							t.Error(err)
							w.WriteHeader(http.StatusBadRequest)
							return
						}
						size := len(batch.Users) + len(batch.Organizations) + len(batch.Repositories) + len(batch.EnterpriseTeams)
						if size == 0 || size > 50 {
							t.Errorf("batch size = %d, want 1..50", size)
						}
						methods = append(methods, req.Method)
						if failAt > 0 && len(methods) == failAt {
							w.WriteHeader(http.StatusBadRequest)
							fmt.Fprint(w, `{"message":"batch failed"}`)
							return
						}
						all := got[req.Method]
						all.Users = append(all.Users, batch.Users...)
						all.Organizations = append(all.Organizations, batch.Organizations...)
						all.Repositories = append(all.Repositories, batch.Repositories...)
						all.EnterpriseTeams = append(all.EnterpriseTeams, batch.EnterpriseTeams...)
						got[req.Method] = all
						w.WriteHeader(http.StatusNoContent)
					}))
					defer server.Close()
					owner := &Owner{v3client: mustCreateTestGitHubClient(t, server.URL+"/")}
					err := reconcileCostCenterResources(t.Context(), owner, "example", "cc-1", desired)
					if failAt > 0 && count > 0 {
						if err == nil || !strings.Contains(err.Error(), "batch failed") {
							t.Fatalf("error = %v, want batch failure", err)
						}
						if len(methods) != failAt {
							t.Fatalf("requests = %d, want %d", len(methods), failAt)
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					var wantMethods []string
					for _, method := range []string{http.MethodDelete, http.MethodPost} {
						for i := 0; i < (count+49)/50; i++ {
							wantMethods = append(wantMethods, method)
						}
						if !reflect.DeepEqual(got[method], want[method]) {
							t.Errorf("%s resources = %#v, want %#v", method, got[method], want[method])
						}
					}
					if !slices.Equal(methods, wantMethods) {
						t.Errorf("methods = %v, want %v", methods, wantMethods)
					}
				})
			}
		}
	}
}
