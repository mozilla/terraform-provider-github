package github

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/shurcooL/githubv4"
)

func dataSourceGithubUsers() *schema.Resource {
	return &schema.Resource{
		ReadContext: dataSourceGithubUsersRead,

		Schema: map[string]*schema.Schema{
			"usernames": {
				Type: schema.TypeList,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
				Optional:     true,
				ExactlyOneOf: []string{"usernames", "user_ids"},
			},
			"user_ids": {
				Type:         schema.TypeList,
				Optional:     true,
				ExactlyOneOf: []string{"usernames", "user_ids"},
				Elem: &schema.Schema{
					Type:             schema.TypeInt,
					ValidateDiagFunc: validation.ToDiagFunc(validation.IntAtLeast(1)),
				},
			},
			"unknown_user_ids": {
				Type:     schema.TypeList,
				Computed: true,
				Elem:     &schema.Schema{Type: schema.TypeInt},
			},
			"logins": {
				Type: schema.TypeList,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
				Computed: true,
			},
			"emails": {
				Type: schema.TypeList,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
				Computed: true,
			},
			"node_ids": {
				Type: schema.TypeList,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
				Computed: true,
			},
			"unknown_logins": {
				Type: schema.TypeList,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
				Computed: true,
			},
		},
	}
}

func dataSourceGithubUsersRead(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	usernames := expandStringList(d.Get("usernames").([]any))

	identifiers := usernames
	var logins, emails, nodeIDs, unknownLogins []string
	var unknownUserIDs []int
	userIDs := d.Get("user_ids").([]any)
	if len(userIDs) > 0 {
		identifiers = make([]string, 0, len(userIDs))
		client := meta.(*Owner).v3client
		for _, value := range userIDs {
			userID := value.(int)
			identifiers = append(identifiers, strconv.Itoa(userID))
			user, response, err := client.Users.GetByID(ctx, int64(userID))
			if err != nil {
				if response != nil && response.StatusCode == http.StatusNotFound {
					unknownUserIDs = append(unknownUserIDs, userID)
					continue
				}
				return diag.FromErr(err)
			}
			logins = append(logins, user.GetLogin())
			emails = append(emails, user.GetEmail())
			nodeIDs = append(nodeIDs, user.GetNodeID())
		}
	} else {
		// Create GraphQL variables and query struct
		type (
			UserFragment struct {
				Id    string
				Login string
				Email string
			}
		)
		var fields []reflect.StructField
		variables := make(map[string]any)
		for idx, username := range usernames {
			label := fmt.Sprintf("User%d", idx)
			variables[label] = githubv4.String(username)
			fields = append(fields, reflect.StructField{
				Name: label, Type: reflect.TypeFor[UserFragment](), Tag: reflect.StructTag(fmt.Sprintf("graphql:\"%[1]s: user(login: $%[1]s)\"", label)),
			})
		}
		query := reflect.New(reflect.StructOf(fields)).Elem()

		if len(usernames) > 0 {
			client := meta.(*Owner).v4client
			err := client.Query(ctx, query.Addr().Interface(), variables)
			if err != nil && !strings.Contains(err.Error(), "Could not resolve to a User with the login of") {
				return diag.FromErr(err)
			}
		}

		for idx, username := range usernames {
			label := fmt.Sprintf("User%d", idx)
			user := query.FieldByName(label).Interface().(UserFragment)
			if user.Login != "" {
				logins = append(logins, user.Login)
				emails = append(emails, user.Email)
				nodeIDs = append(nodeIDs, user.Id)
			} else {
				unknownLogins = append(unknownLogins, username)
			}
		}
	}

	d.SetId(buildChecksumID(identifiers))
	if err := d.Set("logins", logins); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("emails", emails); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("node_ids", nodeIDs); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("unknown_logins", unknownLogins); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("unknown_user_ids", unknownUserIDs); err != nil {
		return diag.FromErr(err)
	}

	return nil
}
