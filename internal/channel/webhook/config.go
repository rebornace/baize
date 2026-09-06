package webhook

import (
	"fmt"
	"strconv"
	"strings"
)

// instanceConfig is a resolved webhook channel instance configuration.
type instanceConfig struct {
	Name           string
	Source         string
	Account        string
	Secret         string
	OutboundSecret string
	OutboundURL    string
	Assignee       string
	AgentID        string
	SupportsVision bool
	Allowlist      map[string]bool
}

// parseConfig resolves an instance from its channel.Config map. name is the
// instance name (used as default source/account).
func parseConfig(name string, m map[string]string) (instanceConfig, error) {
	get := func(k string) string { return strings.TrimSpace(m[k]) }
	c := instanceConfig{Name: name, Allowlist: map[string]bool{}}
	c.Source = orDefault(get("source"), name)
	c.Account = orDefault(get("account"), name)
	c.Secret = get("secret")
	c.OutboundSecret = orDefault(get("outbound_secret"), c.Secret)
	c.OutboundURL = get("outbound_url")
	c.Assignee = get("assignee")
	c.AgentID = get("agent_id")
	if v := get("supports_vision"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return c, fmt.Errorf("webhook: invalid supports_vision %q: %w", v, err)
		}
		c.SupportsVision = b
	}
	if raw := get("allowlist"); raw != "" {
		for _, p := range strings.Split(raw, ",") {
			if p = strings.TrimSpace(p); p != "" {
				c.Allowlist[p] = true
			}
		}
	}
	if c.Secret == "" {
		return c, fmt.Errorf("webhook: missing required config %q for instance %q", "secret", name)
	}
	if c.OutboundURL == "" {
		return c, fmt.Errorf("webhook: missing required config %q for instance %q", "outbound_url", name)
	}
	if c.Assignee == "" {
		return c, fmt.Errorf("webhook: missing required config %q for instance %q", "assignee", name)
	}
	return c, nil
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
