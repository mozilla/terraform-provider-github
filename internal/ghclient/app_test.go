package ghclient

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestNewAppSource(t *testing.T) {
	t.Parallel()

	privateKeyData := mustReadAppPrivateKey(t)

	cacheBasePath := mustMkdirTemp(t, "", "*")
	t.Cleanup(func() {
		_ = os.RemoveAll(cacheBasePath)
	})

	for _, tt := range []struct {
		name           string
		installationID *int64
		opts           SourceOptions
	}{
		{
			name: "default",
			opts: SourceOptions{},
		},
		{
			name:           "with_installation_id",
			installationID: new(int64(1000)),
			opts:           SourceOptions{},
		},
		{
			name: "with_cache_base_path",
			opts: SourceOptions{
				Cache:         true,
				CacheBasePath: mustMkdirTemp(t, cacheBasePath, "*"),
			},
		},
		{
			name: "with_cache_no_base_path",
			opts: SourceOptions{
				Cache: true,
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			source, err := NewAppSource("123456789", privateKeyData, tt.installationID, tt.opts)
			if err != nil {
				t.Fatalf("failed to create app source: %v", err)
			}

			if source == nil {
				t.Fatal("expected app source to be non-nil")
			}
		})
	}
}

func Test_appSource(t *testing.T) {
	t.Parallel()

	owner1 := "octocat"
	owner2 := "acme"

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, fmt.Sprintf("/orgs/%s/installation", owner1)) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id": 1000}`))
			return
		}

		if strings.HasSuffix(r.URL.Path, fmt.Sprintf("/orgs/%s/installation", owner2)) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id": 1001}`))
			return
		}

		w.WriteHeader(http.StatusNotFound)
	})

	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	source, err := NewAppSource("123456789", mustReadAppPrivateKey(t), nil, SourceOptions{BaseURL: ts.URL})
	if err != nil {
		t.Fatalf("failed to create app source: %v", err)
	}

	restClient, err := source.RESTClient()
	if err != nil {
		t.Fatalf("failed to get rest client: %v", err)
	}

	if restClient == nil {
		t.Fatal("expected rest client to be non-nil")
	}

	ownerRESTClientFirst, err := source.OwnerRESTClient(t.Context(), owner1)
	if err != nil {
		t.Fatalf("failed to get first owner rest client: %v", err)
	}

	if ownerRESTClientFirst == nil {
		t.Fatal("expected first owner rest client to be non-nil")
	}

	ownerRESTClientFirstAgain, err := source.OwnerRESTClient(t.Context(), owner1)
	if err != nil {
		t.Fatalf("failed to get first owner rest client again: %v", err)
	}

	if ownerRESTClientFirstAgain != ownerRESTClientFirst {
		t.Fatal("expected first owner rest client to be cached and reused")
	}

	ownerRESTClientSecond, err := source.OwnerRESTClient(t.Context(), owner2)
	if err != nil {
		t.Fatalf("failed to get second owner rest client: %v", err)
	}

	if ownerRESTClientSecond == nil {
		t.Fatal("expected second owner rest client to be non-nil")
	}

	if ownerRESTClientFirst == ownerRESTClientSecond {
		t.Fatal("expected different owner rest clients for different owners")
	}

	graphQLClient, err := source.GraphQLClient()
	if err != nil {
		t.Fatalf("failed to get graphql client: %v", err)
	}

	if graphQLClient == nil {
		t.Fatal("expected graphql client to be non-nil")
	}

	ownerGraphQLClientFirst, err := source.OwnerGraphQLClient(t.Context(), owner1)
	if err != nil {
		t.Fatalf("failed to get first owner graphql client: %v", err)
	}

	if ownerGraphQLClientFirst == nil {
		t.Fatal("expected first owner graphql client to be non-nil")
	}

	ownerGraphQLClientFirstAgain, err := source.OwnerGraphQLClient(t.Context(), owner1)
	if err != nil {
		t.Fatalf("failed to get first owner graphql client again: %v", err)
	}

	if ownerGraphQLClientFirstAgain != ownerGraphQLClientFirst {
		t.Fatal("expected first owner graphql client to be cached and reused")
	}

	ownerGraphQLClientSecond, err := source.OwnerGraphQLClient(t.Context(), owner2)
	if err != nil {
		t.Fatalf("failed to get second owner graphql client: %v", err)
	}

	if ownerGraphQLClientSecond == nil {
		t.Fatal("expected second owner graphql client to be non-nil")
	}

	if ownerGraphQLClientFirst == ownerGraphQLClientSecond {
		t.Fatal("expected different owner graphql clients for different owners")
	}
}

// Test_appSource_noOwner covers an app installed at the enterprise level, where there is no owner to resolve an installation from and the explicitly configured installation must be used instead. The test server fails the test on any request at all, so any attempt to resolve the installation over the API is caught.
func Test_appSource_noOwner(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request to %s; the configured installation id should be used without a lookup", r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	})

	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	source, err := NewAppSource("123456789", mustReadAppPrivateKey(t), new(int64(1000)), SourceOptions{BaseURL: ts.URL})
	if err != nil {
		t.Fatalf("failed to create app source: %v", err)
	}

	restClient, err := source.OwnerRESTClient(t.Context(), "")
	if err != nil {
		t.Fatalf("failed to get rest client without an owner: %v", err)
	}

	if restClient == nil {
		t.Fatal("expected rest client to be non-nil")
	}

	restClientAgain, err := source.OwnerRESTClient(t.Context(), "")
	if err != nil {
		t.Fatalf("failed to get rest client without an owner again: %v", err)
	}

	if restClientAgain != restClient {
		t.Fatal("expected rest client without an owner to be cached and reused")
	}

	appRESTClient, err := source.RESTClient()
	if err != nil {
		t.Fatalf("failed to get app rest client: %v", err)
	}

	if appRESTClient == restClient {
		t.Fatal("expected the app rest client to be distinct from the installation rest client")
	}

	graphQLClient, err := source.OwnerGraphQLClient(t.Context(), "")
	if err != nil {
		t.Fatalf("failed to get graphql client without an owner: %v", err)
	}

	if graphQLClient == nil {
		t.Fatal("expected graphql client to be non-nil")
	}

	graphQLClientAgain, err := source.OwnerGraphQLClient(t.Context(), "")
	if err != nil {
		t.Fatalf("failed to get graphql client without an owner again: %v", err)
	}

	if graphQLClientAgain != graphQLClient {
		t.Fatal("expected graphql client without an owner to be cached and reused")
	}

	appGraphQLClient, err := source.GraphQLClient()
	if err != nil {
		t.Fatalf("failed to get app graphql client: %v", err)
	}

	if appGraphQLClient == graphQLClient {
		t.Fatal("expected the app graphql client to be distinct from the installation graphql client")
	}
}

func Test_appSource_noOwnerOrInstallationID(t *testing.T) {
	t.Parallel()

	source, err := NewAppSource("123456789", mustReadAppPrivateKey(t), nil, SourceOptions{})
	if err != nil {
		t.Fatalf("failed to create app source: %v", err)
	}

	wantErr := "an app installation id is required when no owner is set"

	if _, err := source.OwnerRESTClient(t.Context(), ""); err == nil {
		t.Fatal("expected an error getting a rest client with no owner and no installation id")
	} else if !strings.Contains(err.Error(), wantErr) {
		t.Fatalf("expected error to contain %q, got %v", wantErr, err)
	}

	if _, err := source.OwnerGraphQLClient(t.Context(), ""); err == nil {
		t.Fatal("expected an error getting a graphql client with no owner and no installation id")
	} else if !strings.Contains(err.Error(), wantErr) {
		t.Fatalf("expected error to contain %q, got %v", wantErr, err)
	}
}
