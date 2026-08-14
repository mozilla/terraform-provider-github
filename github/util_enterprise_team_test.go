package github

import (
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func TestEnterpriseTeamMissingFrom(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		want []string
		have []string
		out  []string
	}{
		"nothing wanted":     {want: nil, have: []string{"alice"}, out: []string{}},
		"nothing present":    {want: []string{"alice"}, have: nil, out: []string{"alice"}},
		"fully overlapping":  {want: []string{"alice", "bob"}, have: []string{"bob", "alice"}, out: []string{}},
		"partial overlap":    {want: []string{"alice", "bob"}, have: []string{"alice"}, out: []string{"bob"}},
		"disjoint":           {want: []string{"carol"}, have: []string{"alice"}, out: []string{"carol"}},
		"extras are ignored": {want: []string{"alice"}, have: []string{"alice", "bob"}, out: []string{}},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := enterpriseTeamMissingFrom(tc.want, tc.have)
			if !slices.Equal(got, tc.out) {
				t.Errorf("enterpriseTeamMissingFrom(%v, %v) = %v, want %v", tc.want, tc.have, got, tc.out)
			}
		})
	}
}

func TestEnterpriseTeamSetValues(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		in  []any
		out []string
	}{
		"empty":                 {in: []any{}, out: []string{}},
		"sorted and lowercased": {in: []any{"Bob", "alice"}, out: []string{"alice", "bob"}},
		"duplicates collapse":   {in: []any{"Alice", "alice"}, out: []string{"alice"}},
		"blanks are dropped":    {in: []any{"", "alice"}, out: []string{"alice"}},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			d := schema.TestResourceDataRaw(t, resourceGithubEnterpriseTeamMembers().Schema, map[string]any{
				"members": tc.in,
			})

			got := enterpriseTeamSetValues(d, "members")
			if !slices.Equal(got, tc.out) {
				t.Errorf("enterpriseTeamSetValues(%v) = %v, want %v", tc.in, got, tc.out)
			}
		})
	}
}

func TestCaseInsensitiveHash(t *testing.T) {
	t.Parallel()

	// Differences in casing must not change the hash, otherwise a set element written as "Alice"
	// in configuration would not match "alice" as reported by the API.
	if caseInsensitiveHash("Alice") != caseInsensitiveHash("alice") {
		t.Error("caseInsensitiveHash should ignore casing")
	}

	if caseInsensitiveHash("alice") == caseInsensitiveHash("bob") {
		t.Error("caseInsensitiveHash should distinguish different values")
	}
}
