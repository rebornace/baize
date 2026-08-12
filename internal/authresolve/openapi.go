package authresolve

import (
	"context"
	"time"

	"github.com/rebornace/baize/internal/identity"
)

type OpenAPISecurityResolver struct{}

func (OpenAPISecurityResolver) Resolve(_ context.Context, in ResolveInput) Result {
	if in.ForceIdentityID != "" {
		if id, ok := findByID(in.Identities, in.ForceIdentityID); ok {
			return resultFromIdentity(id)
		}
	}

	if len(in.SecuritySchemes) > 0 {
		if id, ok := pickFromCandidates(filterScheme(in.Identities, in.SecuritySchemes)); ok {
			return resultFromIdentity(id)
		}
	} else if id, ok := pickFromCandidates(filterActive(in.Identities)); ok {
		return resultFromIdentity(id)
	}

	if len(in.DefaultHeaders) > 0 {
		return Result{
			Headers: cloneHeaders(in.DefaultHeaders),
			OK:      true,
		}
	}

	return Result{}
}

func findByID(ids []identity.Identity, id string) (identity.Identity, bool) {
	for _, item := range ids {
		if item.ID == id {
			return item, true
		}
	}
	return identity.Identity{}, false
}

func filterScheme(ids []identity.Identity, schemes []string) []identity.Identity {
	var out []identity.Identity
	for _, id := range ids {
		if isExpired(id) {
			continue
		}
		if schemeMatches(id.Scheme, schemes) {
			out = append(out, id)
		}
	}
	return out
}

func filterActive(ids []identity.Identity) []identity.Identity {
	var out []identity.Identity
	for _, id := range ids {
		if !isExpired(id) {
			out = append(out, id)
		}
	}
	return out
}

func schemeMatches(scheme string, schemes []string) bool {
	for _, s := range schemes {
		if s == scheme {
			return true
		}
	}
	return false
}

func isExpired(id identity.Identity) bool {
	if id.ClaimsSummary == nil {
		return false
	}
	exp, ok := numericClaim(id.ClaimsSummary["exp"])
	if !ok {
		return false
	}
	return exp < float64(time.Now().Unix())
}

func numericClaim(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case uint:
		return float64(n), true
	default:
		return 0, false
	}
}

func pickFromCandidates(ids []identity.Identity) (identity.Identity, bool) {
	if len(ids) == 0 {
		return identity.Identity{}, false
	}

	var defaults []identity.Identity
	for _, id := range ids {
		if id.IsDefault {
			defaults = append(defaults, id)
		}
	}
	if len(defaults) == 1 {
		return defaults[0], true
	}
	if len(defaults) > 1 {
		return pickLatestLastUsed(defaults), true
	}

	if latest, ok := pickByLastUsed(ids); ok {
		return latest, true
	}

	var nonEnv []identity.Identity
	for _, id := range ids {
		if id.Source != identity.SourceEnv {
			nonEnv = append(nonEnv, id)
		}
	}
	if len(nonEnv) == 1 {
		return nonEnv[0], true
	}

	return identity.Identity{}, false
}

func pickByLastUsed(ids []identity.Identity) (identity.Identity, bool) {
	if len(ids) == 0 {
		return identity.Identity{}, false
	}
	allZero := true
	for _, id := range ids {
		if !id.LastUsedAt.IsZero() {
			allZero = false
			break
		}
	}
	if allZero {
		return identity.Identity{}, false
	}
	return pickLatestLastUsed(ids), true
}

func pickLatestLastUsed(ids []identity.Identity) identity.Identity {
	latest := ids[0]
	for _, id := range ids[1:] {
		if id.LastUsedAt.After(latest.LastUsedAt) {
			latest = id
		}
	}
	return latest
}

func resultFromIdentity(id identity.Identity) Result {
	return Result{
		Headers:    cloneHeaders(id.CredentialHeaders),
		IdentityID: id.ID,
		OK:         len(id.CredentialHeaders) > 0,
	}
}

func cloneHeaders(h map[string]string) map[string]string {
	if len(h) == 0 {
		return nil
	}
	out := make(map[string]string, len(h))
	for k, v := range h {
		out[k] = v
	}
	return out
}
