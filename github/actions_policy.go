package github

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/google/go-github/v92/github"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

type actionsPolicyScope string

const (
	actionsPolicyRepository   actionsPolicyScope = "Repository"
	actionsPolicyOrganization actionsPolicyScope = "Organization"
	actionsPolicyEnterprise   actionsPolicyScope = "Enterprise"
	actionsPolicyAPIVersion                      = "2026-03-10"
)

type actionsPolicy struct {
	ID          int64               `json:"id,omitempty"`
	Name        string              `json:"name"`
	Enforcement string              `json:"enforcement"`
	Conditions  map[string]any      `json:"conditions"`
	Rules       []actionsPolicyRule `json:"rules"`
	NodeID      string              `json:"node_id,omitempty"`
	SourceType  string              `json:"source_type,omitempty"`
	Source      string              `json:"source,omitempty"`
}

type actionsPolicyRule struct {
	Type       string         `json:"type"`
	Parameters map[string]any `json:"parameters,omitempty"`
}

func resourceGithubActionsPolicy(scope actionsPolicyScope) *schema.Resource {
	r := &schema.Resource{
		Description: fmt.Sprintf("Manages a GitHub Actions policy at the %s level, including workflow lockfile requirements, allowed actors, and allowed events.", scope),
		Schema: map[string]*schema.Schema{
			"name":        {Type: schema.TypeString, Required: true, Description: "The name of the Actions policy.", ValidateDiagFunc: validation.ToDiagFunc(validation.StringLenBetween(1, 100))},
			"enforcement": {Type: schema.TypeString, Required: true, Description: "The enforcement level: `disabled`, `active`, or `evaluate`. Evaluate mode requires GitHub Enterprise Cloud.", ValidateDiagFunc: validation.ToDiagFunc(validation.StringInSlice([]string{"disabled", "active", "evaluate"}, false))},
			"policy_id":   {Type: schema.TypeInt, Computed: true, Description: "The numeric GitHub ID of the Actions policy."},
			"node_id":     {Type: schema.TypeString, Computed: true, Description: "The GraphQL node ID of the Actions policy."},
			"conditions":  actionsPolicyConditionsSchema(scope),
			"rules": actionsPolicyBlock(map[string]*schema.Schema{
				"require_lockfile": {Type: schema.TypeBool, Optional: true, Default: false, Description: "Require every targeted workflow to be onboarded to its repository's workflow dependency lockfile."},
				"restrict_actions_actors": actionsPolicyBlock(map[string]*schema.Schema{
					"allowed_actors": {Type: schema.TypeSet, Optional: true, Description: "Actors permitted to trigger workflows. An empty set blocks all actors.", Elem: &schema.Resource{Schema: map[string]*schema.Schema{
						"id":   {Type: schema.TypeInt, Required: true, Description: "The numeric actor ID. For App actors, use the GitHub App ID.", ValidateDiagFunc: validation.ToDiagFunc(validation.IntAtLeast(1))},
						"type": {Type: schema.TypeString, Required: true, Description: "The actor type: `User`, `Bot`, `Team`, `BusinessTeam`, `EnterpriseTeam`, `IntegrationInstallation`, `App`, or `RepositoryRole`.", ValidateDiagFunc: validation.ToDiagFunc(validation.StringInSlice([]string{"User", "Bot", "Team", "BusinessTeam", "EnterpriseTeam", "IntegrationInstallation", "App", "RepositoryRole"}, false))},
					}}},
				}, "Restrict who can trigger workflows. Omit this block to allow all actors."),
				"restrict_action_events": actionsPolicyBlock(map[string]*schema.Schema{
					"allowed_events": actionsPolicyStringSet("Events permitted to trigger workflows, such as `push`, `pull_request`, or `workflow_dispatch`. An empty set blocks all events."),
				}, "Restrict which events can trigger workflows. Omit this block to allow all events."),
			}, "Rules enforced by this policy. Removing a rule removes that restriction."),
		},
	}
	r.Schema["rules"].Optional = false
	r.Schema["rules"].Required = true
	switch scope {
	case actionsPolicyRepository:
		r.Schema["repository"] = &schema.Schema{Type: schema.TypeString, Required: true, ForceNew: true, Description: "The repository name, without the owner. The owner is taken from the provider configuration.", ValidateDiagFunc: validation.ToDiagFunc(validation.StringIsNotWhiteSpace)}
	case actionsPolicyEnterprise:
		r.Schema["enterprise_slug"] = &schema.Schema{Type: schema.TypeString, Required: true, ForceNew: true, Description: "The slug of the enterprise.", ValidateDiagFunc: validation.ToDiagFunc(validation.StringIsNotWhiteSpace)}
	}
	r.CustomizeDiff = func(ctx context.Context, diff *schema.ResourceDiff, m any) error {
		return validateActionsPolicyConditions(diff)
	}
	r.CreateContext = func(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
		return actionsPolicyWrite(ctx, d, m.(*Owner), scope, r.Schema, true)
	}
	r.UpdateContext = func(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
		return actionsPolicyWrite(ctx, d, m.(*Owner), scope, r.Schema, false)
	}
	r.ReadContext = func(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
		meta := m.(*Owner)
		path, err := actionsPolicyPath(d, meta, scope, true)
		if err != nil {
			return diag.FromErr(err)
		}
		var policy actionsPolicy
		if err := actionsPolicyRequest(ctx, meta.v3client, http.MethodGet, path, nil, &policy); err != nil {
			if isNotFoundError(err) {
				tflog.Info(ctx, "Removing Actions policy from state because it no longer exists", map[string]any{"id": d.Id()})
				d.SetId("")
				return nil
			}
			return diag.FromErr(err)
		}
		if policy.SourceType != "" && policy.SourceType != string(scope) {
			return diag.Errorf("Actions policy %d belongs to %s %q; manage it at its source rather than as a %s policy", policy.ID, policy.SourceType, policy.Source, scope)
		}
		return diag.FromErr(flattenActionsPolicy(d, policy, r.Schema, d.Get("policy_id").(int) == 0))
	}
	r.DeleteContext = func(ctx context.Context, d *schema.ResourceData, m any) diag.Diagnostics {
		meta := m.(*Owner)
		path, err := actionsPolicyPath(d, meta, scope, true)
		if err != nil {
			return diag.FromErr(err)
		}
		if err := actionsPolicyRequest(ctx, meta.v3client, http.MethodDelete, path, nil, nil); err != nil && !isNotFoundError(err) {
			return diag.FromErr(err)
		}
		d.SetId("")
		return nil
	}
	r.Importer = &schema.ResourceImporter{StateContext: func(ctx context.Context, d *schema.ResourceData, m any) ([]*schema.ResourceData, error) {
		id := d.Id()
		if scope != actionsPolicyOrganization {
			parent, policyID, err := parseID2(id)
			if err != nil {
				return nil, err
			}
			if strings.TrimSpace(parent) == "" {
				return nil, fmt.Errorf("actions policy import requires a non-empty repository name or enterprise slug")
			}
			field := "repository"
			if scope == actionsPolicyEnterprise {
				field = "enterprise_slug"
			}
			if err := d.Set(field, parent); err != nil {
				return nil, err
			}
			id = policyID
		}
		if err := validateActionsPolicyID(id); err != nil {
			return nil, err
		}
		return []*schema.ResourceData{d}, nil
	}}
	return r
}

func actionsPolicyBlock(fields map[string]*schema.Schema, description string) *schema.Schema {
	return &schema.Schema{Type: schema.TypeList, Optional: true, MaxItems: 1, Description: description, Elem: &schema.Resource{Schema: fields}}
}

func actionsPolicyStringSet(description string) *schema.Schema {
	return &schema.Schema{Type: schema.TypeSet, Optional: true, Description: description, Elem: &schema.Schema{Type: schema.TypeString, ValidateDiagFunc: validation.ToDiagFunc(validation.StringIsNotWhiteSpace)}}
}

func actionsPolicyConditionsSchema(scope actionsPolicyScope) *schema.Schema {
	patterns := func(description string, protected bool) *schema.Schema {
		fields := map[string]*schema.Schema{
			"include": actionsPolicyStringSet("Names or glob patterns to include. Use `~ALL` to include all targets."),
			"exclude": actionsPolicyStringSet("Names or glob patterns to exclude. Exclusions take precedence over inclusions."),
		}
		if protected {
			fields["protected"] = &schema.Schema{Type: schema.TypeBool, Optional: true, Default: false, Description: "Prevent renaming of targeted repositories."}
		}
		return actionsPolicyBlock(fields, description)
	}
	properties := func(description string, repository bool) *schema.Schema {
		property := func() *schema.Schema {
			fields := map[string]*schema.Schema{
				"name":            {Type: schema.TypeString, Required: true, Description: "The property name.", ValidateDiagFunc: validation.ToDiagFunc(validation.StringIsNotWhiteSpace)},
				"property_values": actionsPolicyStringSet("Property values to match."),
			}
			fields["property_values"].Optional = false
			fields["property_values"].Required = true
			if repository {
				fields["source"] = &schema.Schema{Type: schema.TypeString, Optional: true, Default: "custom", Description: "The property source: `custom` or `system`.", ValidateDiagFunc: validation.ToDiagFunc(validation.StringInSlice([]string{"custom", "system"}, false))}
			}
			return &schema.Schema{Type: schema.TypeSet, Optional: true, Description: "Property conditions to match.", Elem: &schema.Resource{Schema: fields}}
		}
		return actionsPolicyBlock(map[string]*schema.Schema{"include": property(), "exclude": property()}, description)
	}
	ids := func(field, description string) *schema.Schema {
		return actionsPolicyBlock(map[string]*schema.Schema{field: {Type: schema.TypeSet, Required: true, MinItems: 1, Description: "Numeric IDs to include.", Elem: &schema.Schema{Type: schema.TypeInt, ValidateDiagFunc: validation.ToDiagFunc(validation.IntAtLeast(1))}}}, description)
	}
	fields := map[string]*schema.Schema{"workflow_path": patterns("Workflow file paths or glob patterns to target. Omit to target all workflows. When removing this block, the provider explicitly resets targeting to `~ALL`.", false)}
	if scope != actionsPolicyRepository {
		fields["repository_name"] = patterns("Target repositories by name.", true)
		fields["repository_property"] = properties("Target repositories by custom or system properties.", true)
		repoTargets := []string{"conditions.0.repository_name", "conditions.0.repository_property"}
		if scope == actionsPolicyOrganization {
			fields["repository_id"] = ids("repository_ids", "Target repositories by numeric ID.")
			repoTargets = append(repoTargets, "conditions.0.repository_id")
		}
		for _, path := range repoTargets {
			fields[path[len("conditions.0."):]].ExactlyOneOf = repoTargets
		}
	}
	if scope == actionsPolicyEnterprise {
		fields["organization_name"] = patterns("Target organizations by name. `~ALL` includes all organizations; `~EMUS` targets enterprise managed user accounts.", false)
		fields["organization_id"] = ids("organization_ids", "Target organizations by numeric ID.")
		fields["organization_property"] = properties("Target organizations by property.", false)
		orgTargets := []string{"conditions.0.organization_name", "conditions.0.organization_id", "conditions.0.organization_property"}
		for _, path := range orgTargets {
			fields[path[len("conditions.0."):]].ExactlyOneOf = orgTargets
		}
	}
	result := actionsPolicyBlock(fields, "Conditions selecting the workflows and repositories to which the policy applies.")
	if scope != actionsPolicyRepository {
		result.Optional = false
		result.Required = true
		result.MinItems = 1
	}
	return result
}

func validateActionsPolicyConditions(diff *schema.ResourceDiff) error {
	if !diff.NewValueKnown("conditions") {
		return nil
	}
	value, ok := diff.GetOk("conditions")
	if !ok {
		return nil
	}
	conditions := value.([]any)[0].(map[string]any)
	if workflow := conditions["workflow_path"].([]any); len(workflow) > 0 {
		patterns := workflow[0].(map[string]any)
		include := patterns["include"].(*schema.Set)
		exclude := patterns["exclude"].(*schema.Set)
		if include.Len() == 0 && exclude.Len() == 0 {
			return fmt.Errorf("workflow_path must contain at least one included or excluded pattern")
		}
		if include.Contains("~ALL") && include.Len() > 1 {
			return fmt.Errorf("workflow_path include cannot combine ~ALL with other patterns")
		}
		if exclude.Contains("~ALL") {
			return fmt.Errorf("workflow_path exclude cannot contain ~ALL")
		}
	}
	return nil
}

func validateActionsPolicyID(value string) error {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return fmt.Errorf("invalid Actions policy ID %q: expected a positive integer", value)
	}
	return nil
}

func actionsPolicyPath(d *schema.ResourceData, meta *Owner, scope actionsPolicyScope, individual bool) (string, error) {
	var path string
	switch scope {
	case actionsPolicyRepository:
		path = fmt.Sprintf("repos/%s/%s/actions/policies", url.PathEscape(meta.name), url.PathEscape(d.Get("repository").(string)))
	case actionsPolicyOrganization:
		path = fmt.Sprintf("orgs/%s/actions/policies", url.PathEscape(meta.name))
	case actionsPolicyEnterprise:
		path = fmt.Sprintf("enterprises/%s/actions/policies", url.PathEscape(d.Get("enterprise_slug").(string)))
	}
	if !individual {
		return path, nil
	}
	id := d.Id()
	if scope != actionsPolicyOrganization {
		_, policyID, err := parseID2(id)
		if err != nil {
			return "", err
		}
		id = policyID
	}
	if err := validateActionsPolicyID(id); err != nil {
		return "", err
	}
	return path + "/" + id, nil
}

func actionsPolicyRequest(ctx context.Context, client *github.Client, method, path string, body, result any) error {
	req, err := client.NewRequest(ctx, method, path, body)
	if err != nil {
		return err
	}
	req.Header.Set("X-GitHub-Api-Version", actionsPolicyAPIVersion)
	_, err = client.Do(req, result)
	return err
}

func actionsPolicyWrite(ctx context.Context, d *schema.ResourceData, meta *Owner, scope actionsPolicyScope, fields map[string]*schema.Schema, create bool) diag.Diagnostics {
	path, err := actionsPolicyPath(d, meta, scope, !create)
	if err != nil {
		return diag.FromErr(err)
	}
	policy := actionsPolicy{Name: d.Get("name").(string), Enforcement: d.Get("enforcement").(string), Rules: []actionsPolicyRule{}, Conditions: map[string]any{}}
	if value := d.Get("conditions").([]any); len(value) > 0 {
		policy.Conditions = expandActionsPolicyObject(value[0].(map[string]any), fields["conditions"].Elem.(*schema.Resource).Schema)
	}
	// GitHub preserves workflow targeting when omitted on update. Explicitly
	// reset it when Terraform removes a previously configured workflow condition.
	if !create && d.HasChange("conditions") {
		if _, exists := policy.Conditions["workflow_path"]; !exists {
			policy.Conditions["workflow_path"] = map[string]any{"include": []any{"~ALL"}, "exclude": []any{}}
		}
	}
	rules := d.Get("rules").([]any)[0].(map[string]any)
	if rules["require_lockfile"].(bool) {
		policy.Rules = append(policy.Rules, actionsPolicyRule{Type: "require_lockfile"})
	}
	for _, name := range []string{"restrict_actions_actors", "restrict_action_events"} {
		if value := rules[name].([]any); len(value) > 0 {
			ruleSchema := fields["rules"].Elem.(*schema.Resource).Schema[name].Elem.(*schema.Resource).Schema
			policy.Rules = append(policy.Rules, actionsPolicyRule{Type: name, Parameters: expandActionsPolicyObject(value[0].(map[string]any), ruleSchema)})
		}
	}
	method := http.MethodPut
	if create {
		method = http.MethodPost
	}
	var result actionsPolicy
	if err := actionsPolicyRequest(ctx, meta.v3client, method, path, policy, &result); err != nil {
		return diag.FromErr(err)
	}
	if result.ID <= 0 {
		return diag.Errorf("writing Actions policy returned no ID")
	}
	if create {
		id := strconv.FormatInt(result.ID, 10)
		if scope != actionsPolicyOrganization {
			field := "repository"
			if scope == actionsPolicyEnterprise {
				field = "enterprise_slug"
			}
			parent := d.Get(field).(string)
			id, err = buildID(parent, id)
			if err != nil {
				return diag.FromErr(err)
			}
		}
		d.SetId(id)
	}
	return diag.FromErr(flattenActionsPolicy(d, result, fields, false))
}

// Singleton Terraform blocks are API objects; sets are API arrays.
func expandActionsPolicyObject(value map[string]any, fields map[string]*schema.Schema) map[string]any {
	result := make(map[string]any)
	for name, field := range fields {
		v := value[name]
		switch field.Type {
		case schema.TypeList:
			if list := v.([]any); len(list) > 0 {
				result[name] = expandActionsPolicyObject(list[0].(map[string]any), field.Elem.(*schema.Resource).Schema)
			}
		case schema.TypeSet:
			list := v.(*schema.Set).List()
			if nested, ok := field.Elem.(*schema.Resource); ok {
				for i, item := range list {
					list[i] = expandActionsPolicyObject(item.(map[string]any), nested.Schema)
				}
			}
			result[name] = list
		default:
			result[name] = v
		}
	}
	return result
}

func flattenActionsPolicyObject(value map[string]any, fields map[string]*schema.Schema) map[string]any {
	result := make(map[string]any)
	for name, field := range fields {
		v, exists := value[name]
		if !exists || v == nil {
			if field.Default != nil {
				result[name] = field.Default
			}
			continue
		}
		switch field.Type {
		case schema.TypeList:
			result[name] = []any{flattenActionsPolicyObject(v.(map[string]any), field.Elem.(*schema.Resource).Schema)}
		case schema.TypeSet:
			if nested, ok := field.Elem.(*schema.Resource); ok {
				list := v.([]any)
				items := make([]any, 0, len(list))
				for _, item := range list {
					items = append(items, flattenActionsPolicyObject(item.(map[string]any), nested.Schema))
				}
				result[name] = items
			} else {
				result[name] = v
			}
		default:
			result[name] = v
		}
	}
	return result
}

func flattenActionsPolicy(d *schema.ResourceData, policy actionsPolicy, fields map[string]*schema.Schema, importing bool) error {
	rules := map[string]any{"require_lockfile": false, "restrict_actions_actors": []any{}, "restrict_action_events": []any{}}
	for _, rule := range policy.Rules {
		switch rule.Type {
		case "require_lockfile":
			rules["require_lockfile"] = true
		case "restrict_actions_actors", "restrict_action_events":
			ruleFields := fields["rules"].Elem.(*schema.Resource).Schema[rule.Type].Elem.(*schema.Resource).Schema
			rules[rule.Type] = []any{flattenActionsPolicyObject(rule.Parameters, ruleFields)}
		default:
			return fmt.Errorf("actions policy contains unsupported rule type %q; update the provider before managing this policy", rule.Type)
		}
	}
	conditions := flattenActionsPolicyObject(policy.Conditions, fields["conditions"].Elem.(*schema.Resource).Schema)
	// An explicit all-workflows condition is how the API resets targeting. Keep
	// an omitted Terraform block omitted when it means the same thing. Imports
	// retain the API condition because they do not yet have configured targeting.
	if _, configured := d.GetOk("conditions.0.workflow_path"); !configured && !importing {
		if workflow, ok := policy.Conditions["workflow_path"].(map[string]any); ok {
			include, _ := workflow["include"].([]any)
			exclude, _ := workflow["exclude"].([]any)
			if len(exclude) == 0 && (len(include) == 0 || (len(include) == 1 && include[0] == "~ALL")) {
				delete(conditions, "workflow_path")
			}
		}
	}
	conditionList := []any{}
	if len(conditions) > 0 {
		conditionList = append(conditionList, conditions)
	}
	for name, value := range map[string]any{"name": policy.Name, "enforcement": policy.Enforcement, "policy_id": policy.ID, "node_id": policy.NodeID, "rules": []any{rules}, "conditions": conditionList} {
		if err := d.Set(name, value); err != nil {
			return fmt.Errorf("setting Actions policy %s: %w", name, err)
		}
	}
	return nil
}
