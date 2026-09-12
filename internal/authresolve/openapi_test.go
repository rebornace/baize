package authresolve_test

import (
	"context"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/authresolve"
	"github.com/rebornace/baize/internal/identity"
)

func TestOpenAPISecurityResolverOrder(t *testing.T) {
	r := authresolve.OpenAPISecurityResolver{}
	ids := []identity.Identity{
		{ID: "env", Source: identity.SourceEnv, CredentialHeaders: map[string]string{"Authorization": "Bearer ENV"}, Scheme: "bearer"},
		{ID: "a", Source: identity.SourceLoginCapture, Scheme: "bearer", IsDefault: true, CredentialHeaders: map[string]string{"Authorization": "Bearer A"}, Subject: "a"},
		{ID: "b", Source: identity.SourceLoginCapture, Scheme: "other", CredentialHeaders: map[string]string{"Authorization": "Bearer B"}, Subject: "b"},
	}
	res := r.Resolve(context.Background(), authresolve.ResolveInput{
		Identities:      ids,
		SecuritySchemes: []string{"bearer"},
		DefaultHeaders:  map[string]string{"Authorization": "Bearer ENV"},
	})
	if res.IdentityID != "a" || res.Headers["Authorization"] != "Bearer A" {
		t.Fatalf("%+v", res)
	}
	res2 := r.Resolve(context.Background(), authresolve.ResolveInput{
		Identities:      ids,
		ForceIdentityID: "b",
		SecuritySchemes: []string{"bearer"},
	})
	if res2.IdentityID != "b" {
		t.Fatalf("%+v", res2)
	}
}

func TestOpenAPISecurityResolverDefaultHeadersFallback(t *testing.T) {
	r := authresolve.OpenAPISecurityResolver{}
	ids := []identity.Identity{
		{ID: "b", Source: identity.SourceLoginCapture, Scheme: "other", CredentialHeaders: map[string]string{"Authorization": "Bearer B"}},
	}
	res := r.Resolve(context.Background(), authresolve.ResolveInput{
		Identities:      ids,
		SecuritySchemes: []string{"bearer"},
		DefaultHeaders:  map[string]string{"Authorization": "Bearer ENV"},
	})
	if !res.OK || res.Headers["Authorization"] != "Bearer ENV" || res.IdentityID != "" {
		t.Fatalf("%+v", res)
	}
}

func TestOpenAPISecurityResolverSkipsExpired(t *testing.T) {
	r := authresolve.OpenAPISecurityResolver{}
	expired := float64(time.Now().Add(-time.Hour).Unix())
	ids := []identity.Identity{
		{
			ID:                "expired",
			Source:            identity.SourceLoginCapture,
			Scheme:            "bearer",
			IsDefault:         true,
			CredentialHeaders: map[string]string{"Authorization": "Bearer OLD"},
			ClaimsSummary:     map[string]any{"exp": expired},
		},
		{
			ID:                "fresh",
			Source:            identity.SourceLoginCapture,
			Scheme:            "bearer",
			CredentialHeaders: map[string]string{"Authorization": "Bearer NEW"},
		},
	}
	res := r.Resolve(context.Background(), authresolve.ResolveInput{
		Identities:      ids,
		SecuritySchemes: []string{"bearer"},
	})
	if res.IdentityID != "fresh" || res.Headers["Authorization"] != "Bearer NEW" {
		t.Fatalf("%+v", res)
	}
}

func TestOpenAPISecurityResolverDefaultAndLastUsed(t *testing.T) {
	r := authresolve.OpenAPISecurityResolver{}
	now := time.Now()
	ids := []identity.Identity{
		{
			ID:                "older",
			Source:            identity.SourceLoginCapture,
			Scheme:            "bearer",
			CredentialHeaders: map[string]string{"Authorization": "Bearer OLD"},
			LastUsedAt:        now.Add(-time.Hour),
		},
		{
			ID:                "recent",
			Source:            identity.SourceLoginCapture,
			Scheme:            "bearer",
			CredentialHeaders: map[string]string{"Authorization": "Bearer RECENT"},
			LastUsedAt:        now,
		},
	}
	res := r.Resolve(context.Background(), authresolve.ResolveInput{
		Identities:      ids,
		SecuritySchemes: []string{"bearer"},
	})
	if res.IdentityID != "recent" {
		t.Fatalf("%+v", res)
	}
}

func TestOpenAPISecurityResolverUniqueNonEnv(t *testing.T) {
	r := authresolve.OpenAPISecurityResolver{}
	ids := []identity.Identity{
		{ID: "env", Source: identity.SourceEnv, CredentialHeaders: map[string]string{"Authorization": "Bearer ENV"}},
		{ID: "only", Source: identity.SourceLoginCapture, CredentialHeaders: map[string]string{"Authorization": "Bearer ONLY"}},
	}
	res := r.Resolve(context.Background(), authresolve.ResolveInput{
		Identities:     ids,
		DefaultHeaders: map[string]string{"Authorization": "Bearer ENV"},
	})
	if res.IdentityID != "only" || res.Headers["Authorization"] != "Bearer ONLY" {
		t.Fatalf("%+v", res)
	}
}
