package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	gh "github.com/google/go-github/v92/github"
)

// The go-github cost center types do not yet include the fields in the 2026-03-10 API.
const costCenterAPIVersion = "2026-03-10"

type costCenter struct {
	ID                  string               `json:"id"`
	Name                string               `json:"name"`
	State               string               `json:"state"`
	AzureSubscription   string               `json:"azure_subscription"`
	Resources           []costCenterResource `json:"resources"`
	AICreditPoolEnabled bool                 `json:"ai_credit_pool_enabled"`
	AICreditPoolState   *struct {
		TargetAmount  *float64 `json:"target_amount"`
		CurrentAmount *float64 `json:"current_amount"`
	} `json:"ai_credit_pool_state"`
}

type costCenterResource struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

type costCenterChange struct {
	Name                *string `json:"name,omitempty"`
	AICreditPoolEnabled *bool   `json:"ai_credit_pool_enabled,omitempty"`
}

type costCenterResourcesChange struct {
	Users           []string `json:"users,omitempty"`
	Organizations   []string `json:"organizations,omitempty"`
	Repositories    []string `json:"repositories,omitempty"`
	EnterpriseTeams []string `json:"enterprise_teams,omitempty"`
}

func costCenterPath(enterprise, id string) string {
	path := "enterprises/" + url.PathEscape(enterprise) + "/settings/billing/cost-centers"
	if id != "" {
		path += "/" + url.PathEscape(id)
	}
	return path
}

func costCenterRequest(ctx context.Context, client *gh.Client, method, path string, body, result any) error {
	req, err := client.NewRequest(ctx, method, path, body)
	if err != nil {
		return err
	}
	req.Header.Set("X-GitHub-Api-Version", costCenterAPIVersion)
	_, err = client.Do(req, result)
	return err
}

func getCostCenter(ctx context.Context, client *gh.Client, enterprise, id string) (*costCenter, error) {
	var center costCenter
	if err := costCenterRequest(ctx, client, http.MethodGet, costCenterPath(enterprise, id), nil, &center); err != nil {
		return nil, fmt.Errorf("get cost center %q in enterprise %q: %w", id, enterprise, err)
	}
	return &center, nil
}

func costCenterNotFound(err error) bool {
	var ghErr *gh.ErrorResponse
	return errors.As(err, &ghErr) && ghErr.Response != nil && ghErr.Response.StatusCode == http.StatusNotFound
}
