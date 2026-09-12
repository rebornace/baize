package webhook

import (
	"fmt"
	"strconv"
	"strings"
)

// instanceConfig is a resolved webhook channel instance configuration.
type instanceConfig struct {
	Name             string
	Source           string
	Account          string
	Secret           string
	OutboundSecret   string
	OutboundURL      string
	Assignee         string
	AgentID          string
	Allowlist        map[string]bool
	AdminURL         string
	AdapterAutostart bool
	AdapterCommand   string
	AdapterArgs      []string
	AdapterCredsDir  string
	// AdapterBaizeURL is baize's own loopback base URL passed to an autostart
	// adapter child (its -baize inbound URL). Empty falls back to the
	// BuildDeps.SelfBaseURL, then the loopback default http://127.0.0.1:8080.
	AdapterBaizeURL string
	// secretAuto is true when the HMAC secret was auto-generated (no explicit
	// config secret). Bootstrap then persists/reuses a stable on-disk secret so
	// an adapter orphaned by a previous baize run still verifies requests.
	secretAuto bool
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
	if raw := get("allowlist"); raw != "" {
		for _, p := range strings.Split(raw, ",") {
			if p = strings.TrimSpace(p); p != "" {
				c.Allowlist[p] = true
			}
		}
	}
	c.AdminURL = get("admin_url")
	c.AdapterCommand = get("adapter_command")
	c.AdapterCredsDir = get("adapter_creds_dir")
	c.AdapterBaizeURL = get("adapter_baize_url")
	if v := get("adapter_autostart"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return c, fmt.Errorf("webhook: invalid adapter_autostart %q: %w", v, err)
		}
		c.AdapterAutostart = b
	}
	if raw := get("adapter_args"); raw != "" {
		for _, a := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ' ' }) {
			if a = strings.TrimSpace(a); a != "" {
				c.AdapterArgs = append(c.AdapterArgs, a)
			}
		}
	}
	// Autostart instances are launched and health-checked by baize itself, so
	// the adapter command and its admin/healthz base URL are mandatory.
	if c.AdapterAutostart {
		if c.AdapterCommand == "" {
			return c, fmt.Errorf("webhook: adapter_command required when adapter_autostart=true (instance %q)", name)
		}
		if c.AdminURL == "" {
			return c, fmt.Errorf("webhook: admin_url required when adapter_autostart=true (instance %q)", name)
		}
	}
	if c.Secret == "" && !c.AdapterAutostart {
		return c, fmt.Errorf("webhook: missing required config %q for instance %q (or set adapter_autostart=true to generate one)", "secret", name)
	}
	// Autostart adapters share an ephemeral HMAC key with baize: when no
	// secret is configured, generate one here (inside parseConfig, so both
	// openFromConfig and direct callers/tests observe it). OutboundSecret was
	// defaulted to the (then empty) secret early in this function; backfill it
	// so the outbound side falls back to the generated secret.
	if c.Secret == "" && c.AdapterAutostart {
		secret, err := generateSecret()
		if err != nil {
			return c, fmt.Errorf("webhook: generate adapter secret: %w", err)
		}
		c.Secret = secret
		c.secretAuto = true
		if c.OutboundSecret == "" {
			c.OutboundSecret = c.Secret
		}
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
