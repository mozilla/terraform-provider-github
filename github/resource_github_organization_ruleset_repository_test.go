package github

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-github/v92/github"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func TestOrganizationRulesetRepositoryTarget(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		change    func(map[string]any)
		wantError string
	}{
		{name: "active"},
		{name: "disabled", change: func(c map[string]any) { c["enforcement"] = "disabled" }},
		{name: "evaluate", change: func(c map[string]any) { c["enforcement"] = "evaluate" }, wantError: "enforcement evaluate"},
		{name: "ref name", change: func(c map[string]any) {
			c["conditions"].([]any)[0].(map[string]any)["ref_name"] = []any{map[string]any{"include": []any{"~ALL"}, "exclude": []any{}}}
		}, wantError: "ref_name must not be set"},
		{name: "repository IDs", change: func(c map[string]any) {
			c["conditions"] = []any{map[string]any{"repository_id": []any{123}}}
		}},
		{name: "repository properties", change: func(c map[string]any) {
			c["conditions"] = []any{map[string]any{"repository_property": []any{map[string]any{
				"include": []any{map[string]any{"name": "environment", "property_values": []any{"production"}, "source": "custom"}},
			}}}}
		}},
		{name: "pull request bypass", change: func(c map[string]any) {
			c["bypass_actors"] = []any{map[string]any{"actor_type": "Team", "actor_id": 123, "bypass_mode": "pull_request"}}
		}, wantError: "bypass_mode pull_request"},
		{name: "exempt bypass", change: func(c map[string]any) {
			c["bypass_actors"] = []any{map[string]any{"actor_type": "Team", "actor_id": 123, "bypass_mode": "exempt"}}
		}},
		{name: "ref creation", change: func(c map[string]any) {
			c["rules"] = []any{map[string]any{"creation": true}}
		}, wantError: `rule "creation" is not valid`},
		{name: "ref deletion", change: func(c map[string]any) {
			c["rules"] = []any{map[string]any{"deletion": true}}
		}, wantError: `rule "deletion" is not valid`},
		{name: "push rule", change: func(c map[string]any) {
			c["rules"] = []any{map[string]any{"max_file_size": []any{map[string]any{"max_file_size": 10}}}}
		}, wantError: `rule "max_file_size" is not valid`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			config := map[string]any{
				"name": "repository-policy", "target": "repository", "enforcement": "active",
				"conditions":    []any{map[string]any{"repository_name": []any{map[string]any{"include": []any{"~ALL"}, "exclude": []any{}}}}},
				"bypass_actors": []any{map[string]any{"actor_type": "OrganizationAdmin", "bypass_mode": "always"}},
				"rules": []any{map[string]any{
					"repository_create": true, "repository_delete": true, "repository_transfer": true,
					"repository_name":       []any{map[string]any{"pattern": "^service-", "negate": false}},
					"repository_visibility": []any{map[string]any{"internal": false, "private": true, "public": false}},
				}},
			}
			if tt.change != nil {
				tt.change(config)
			}
			r := resourceGithubOrganizationRuleset()
			c := terraform.NewResourceConfigRaw(config)
			if diags := r.Validate(c); diags.HasError() {
				t.Fatalf("schema validation failed: %v", diags)
			}
			_, err := r.Diff(t.Context(), nil, c, nil)
			if tt.wantError == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("expected %q, got %v", tt.wantError, err)
			}
		})
	}
}

func TestOrganizationRulesetRepositoryRulesRejectOtherTargets(t *testing.T) {
	t.Parallel()
	for _, target := range []string{"branch", "tag", "push"} {
		for _, rule := range repositoryOnlyRules {
			t.Run(target+"/"+string(rule), func(t *testing.T) {
				t.Parallel()
				var value any = true
				switch rule {
				case github.RulesetRuleTypeRepositoryName:
					value = []any{map[string]any{"pattern": "^service-"}}
				case github.RulesetRuleTypeRepositoryVisibility:
					value = []any{map[string]any{"internal": false, "private": true, "public": false}}
				}
				config := terraform.NewResourceConfigRaw(map[string]any{
					"name": "test", "target": target, "enforcement": "active",
					"rules": []any{map[string]any{string(rule): value}},
				})
				_, err := resourceGithubOrganizationRuleset().Diff(t.Context(), nil, config, nil)
				if err == nil || !strings.Contains(err.Error(), "is not valid for "+target+" target") {
					t.Fatalf("expected invalid rule error, got %v", err)
				}
			})
		}
	}
}

func TestOrganizationRulesetRepositoryRulesRoundTrip(t *testing.T) {
	t.Parallel()
	const payload = `[{"type":"repository_create"},{"type":"repository_delete"},{"type":"repository_name","parameters":{"negate":true,"pattern":"^test-"}},{"type":"repository_transfer"},{"type":"repository_visibility","parameters":{"internal":false,"private":true,"public":true}}]`
	var rules github.RepositoryRulesetRules
	if err := json.Unmarshal([]byte(payload), &rules); err != nil {
		t.Fatal(err)
	}
	d := schema.TestResourceDataRaw(t, resourceGithubOrganizationRuleset().Schema, nil)
	if err := d.Set("rules", flattenRules(t.Context(), &rules, true)); err != nil {
		t.Fatal(err)
	}
	expanded := expandRules(d.Get("rules").([]any), true)
	got, err := json.Marshal(expanded)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != payload {
		t.Fatalf("unexpected API rules: %s", got)
	}
	// A refresh after removing policy rules must clear their previous state.
	if err := d.Set("rules", flattenRules(t.Context(), &github.RepositoryRulesetRules{}, true)); err != nil {
		t.Fatal(err)
	}
	got, err = json.Marshal(expandRules(d.Get("rules").([]any), true))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "[]" {
		t.Fatalf("rules were not cleared: %s", got)
	}
	for name := range flattenRules(t.Context(), &rules, false)[0].(map[string]any) {
		if strings.HasPrefix(name, "repository_") {
			t.Fatalf("organization-only rule leaked into repository schema: %s", name)
		}
	}
}

func TestOrganizationRulesetRepositoryVisibilityRoundTrip(t *testing.T) {
	t.Parallel()
	for _, public := range []bool{false, true} {
		for _, internal := range []bool{false, true} {
			for _, private := range []bool{false, true} {
				t.Run(fmt.Sprintf("public=%t/internal=%t/private=%t", public, internal, private), func(t *testing.T) {
					t.Parallel()
					// Include all false, as returned by the live repository policy API.
					payload := fmt.Sprintf(`[{"type":"repository_visibility","parameters":{"internal":%t,"private":%t,"public":%t}}]`, internal, private, public)
					var rules github.RepositoryRulesetRules
					if err := json.Unmarshal([]byte(payload), &rules); err != nil {
						t.Fatal(err)
					}
					d := schema.TestResourceDataRaw(t, resourceGithubOrganizationRuleset().Schema, nil)
					if err := d.Set("rules", flattenRules(t.Context(), &rules, true)); err != nil {
						t.Fatal(err)
					}
					got, err := json.Marshal(expandRules(d.Get("rules").([]any), true))
					if err != nil {
						t.Fatal(err)
					}
					if string(got) != payload {
						t.Fatalf("want %s, got %s", payload, got)
					}
				})
			}
		}
	}
}
