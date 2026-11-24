package github

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/google/go-github/v92/github"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

var enterpriseCustomPropertyName = regexp.MustCompile(`^[A-Za-z0-9_#$-]{1,75}$`)

func resourceGithubEnterpriseCustomProperty() *schema.Resource {
	return &schema.Resource{
		Description:   "Manages a custom property definition for repositories in a GitHub enterprise.",
		CreateContext: resourceGithubEnterpriseCustomPropertyWrite,
		ReadContext:   resourceGithubEnterpriseCustomPropertyRead,
		UpdateContext: resourceGithubEnterpriseCustomPropertyWrite,
		DeleteContext: resourceGithubEnterpriseCustomPropertyDelete,
		Importer:      &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext},
		CustomizeDiff: func(_ context.Context, d *schema.ResourceDiff, _ any) error {
			if !d.NewValueKnown("value_type") {
				return nil
			}
			if d.Get("value_type") == string(github.PropertyValueTypeMultiSelect) {
				if _, ok := d.GetOk("default_value"); ok {
					return fmt.Errorf("use default_values for multi_select properties")
				}
			} else if _, ok := d.GetOk("default_values"); ok {
				return fmt.Errorf("default_values is only supported for multi_select properties")
			}
			return nil
		},
		Schema: map[string]*schema.Schema{
			"enterprise_slug": {
				Type:             schema.TypeString,
				Required:         true,
				ForceNew:         true,
				Description:      "The slug of the enterprise.",
				ValidateDiagFunc: validation.ToDiagFunc(validation.StringIsNotWhiteSpace),
			},
			"property_name": {
				Type:        schema.TypeString,
				Required:    true,
				ForceNew:    true,
				Description: "The name of the custom property.",
				ValidateDiagFunc: validation.ToDiagFunc(validation.StringMatch(
					enterpriseCustomPropertyName, "must be 1 to 75 characters and contain only letters, digits, underscores, hyphens, dollar signs, or number signs")),
			},
			"value_type": {
				Type:        schema.TypeString,
				Optional:    true,
				Default:     string(github.PropertyValueTypeString),
				Description: "The type of the custom property. Valid values are `string`, `single_select`, `multi_select`, `true_false`, and `url`. Defaults to `string`.",
				ValidateDiagFunc: validation.ToDiagFunc(validation.StringInSlice([]string{
					string(github.PropertyValueTypeString),
					string(github.PropertyValueTypeSingleSelect),
					string(github.PropertyValueTypeMultiSelect),
					string(github.PropertyValueTypeTrueFalse),
					string(github.PropertyValueTypeURL),
				}, false)),
			},
			"required": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "Whether all repositories in the enterprise are required to have a value for the property. Defaults to `false`.",
			},
			"require_explicit_values": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "Whether actors must supply an explicit value when setting properties or creating or transferring repositories. Defaults to `false`.",
			},
			"default_value": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "The default value for a scalar property. For `true_false`, use the string `true` or `false`.",
			},
			"default_values": {
				Type:        schema.TypeList,
				Optional:    true,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Description: "The default values for a `multi_select` property.",
			},
			"description": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "A short description of the custom property.",
			},
			"allowed_values": {
				Type:        schema.TypeList,
				Optional:    true,
				MaxItems:    200,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Description: "The ordered list of allowed values for a `single_select` or `multi_select` property. GitHub allows up to 200 values.",
			},
			"values_editable_by": {
				Type:             schema.TypeString,
				Optional:         true,
				Default:          "org_actors",
				Description:      "Who can set property values on repositories. Valid values are `org_actors` and `org_and_repo_actors`. Defaults to `org_actors`.",
				ValidateDiagFunc: validation.ToDiagFunc(validation.StringInSlice([]string{"org_actors", "org_and_repo_actors"}, false)),
			},
		},
	}
}

// go-github's CustomProperty does not yet include require_explicit_values.
type enterpriseCustomProperty struct {
	github.CustomProperty
	RequireExplicitValues bool `json:"require_explicit_values"`
}

func resourceGithubEnterpriseCustomPropertyWrite(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	enterprise := d.Get("enterprise_slug").(string)
	name := d.Get("property_name").(string)
	id, err := buildID(enterprise, name)
	if err != nil {
		return diag.FromErr(err)
	}

	// Send nulls for unset nullable fields so updates can clear previous values.
	change := map[string]any{
		"value_type":              d.Get("value_type"),
		"required":                d.Get("required"),
		"require_explicit_values": d.Get("require_explicit_values"),
		"values_editable_by":      d.Get("values_editable_by"),
		"default_value":           nil,
		"description":             nil,
		"allowed_values":          nil,
	}
	if value, ok := d.GetOk("default_value"); ok {
		change["default_value"] = value
	}
	if values, ok := d.GetOk("default_values"); ok {
		change["default_value"] = values
	}
	if value, ok := d.GetOk("description"); ok {
		change["description"] = value
	}
	if values, ok := d.GetOk("allowed_values"); ok {
		change["allowed_values"] = values
	}

	var property enterpriseCustomProperty
	if err := enterpriseCustomPropertyRequest(ctx, meta.(*Owner).v3client, http.MethodPut, enterprise, name, change, &property); err != nil {
		return diag.FromErr(err)
	}
	d.SetId(id)
	return setEnterpriseCustomPropertyState(d, enterprise, name, property)
}

func resourceGithubEnterpriseCustomPropertyRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	enterprise, name, err := parseID2(d.Id())
	if err != nil {
		return diag.FromErr(err)
	}
	var property enterpriseCustomProperty
	if err := enterpriseCustomPropertyRequest(ctx, meta.(*Owner).v3client, http.MethodGet, enterprise, name, nil, &property); err != nil {
		return diag.FromErr(deleteResourceOn404AndSwallow304OtherwiseReturnError(err, d, "enterprise custom property %s", d.Id()))
	}
	return setEnterpriseCustomPropertyState(d, enterprise, name, property)
}

func setEnterpriseCustomPropertyState(d *schema.ResourceData, enterprise, name string, property enterpriseCustomProperty) diag.Diagnostics {
	editor := property.GetValuesEditableBy()
	if editor == "" {
		editor = "org_actors"
	}
	fields := map[string]any{
		"enterprise_slug":         enterprise,
		"property_name":           name,
		"value_type":              string(property.ValueType),
		"required":                property.GetRequired(),
		"require_explicit_values": property.RequireExplicitValues,
		"description":             property.GetDescription(),
		"allowed_values":          property.AllowedValues,
		"values_editable_by":      editor,
		"default_value":           nil,
		"default_values":          nil,
	}
	if property.DefaultValue != nil {
		switch property.ValueType {
		case github.PropertyValueTypeMultiSelect:
			values, ok := property.DefaultValueStrings()
			if !ok {
				return diag.Errorf("enterprise custom property %q returned a default value that is not a list of strings", name)
			}
			fields["default_values"] = values
		default:
			value, ok := property.DefaultValue.(string)
			if !ok {
				return diag.Errorf("enterprise custom property %q returned a default value that is not a string", name)
			}
			fields["default_value"] = value
		}
	}
	for key, value := range fields {
		if err := d.Set(key, value); err != nil {
			return diag.FromErr(err)
		}
	}
	return nil
}

func resourceGithubEnterpriseCustomPropertyDelete(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	enterprise, name, err := parseID2(d.Id())
	if err != nil {
		return diag.FromErr(err)
	}
	if err := enterpriseCustomPropertyRequest(ctx, meta.(*Owner).v3client, http.MethodDelete, enterprise, name, nil, nil); err != nil {
		return diag.FromErr(deleteResourceOn404AndSwallow304OtherwiseReturnError(err, d, "enterprise custom property %s", d.Id()))
	}
	d.SetId("")
	return nil
}

func enterpriseCustomPropertyRequest(ctx context.Context, client *github.Client, method, enterprise, name string, body, result any) error {
	if strings.TrimSpace(enterprise) == "" || enterprise == "." || enterprise == ".." || !enterpriseCustomPropertyName.MatchString(name) {
		return fmt.Errorf("expected an enterprise slug and a valid custom property name, got %q and %q", enterprise, name)
	}
	path := fmt.Sprintf("enterprises/%s/properties/schema/%s", url.PathEscape(enterprise), url.PathEscape(name))
	req, err := client.NewRequest(ctx, method, path, body)
	if err != nil {
		return err
	}
	_, err = client.Do(req, result)
	return err
}
