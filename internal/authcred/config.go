package authcred

import "errors"

var ErrInvalidAuth = errors.New("invalid_auth")

const (
	ModeStatic      = "static"
	ModePassthrough = "passthrough"
	ModeVaultRef    = "vault_ref"
)

type Config struct {
	Mode        string    `json:"mode,omitempty" yaml:"mode"`
	Static      Static    `json:"static,omitempty" yaml:"static"`
	Passthrough PassThru  `json:"passthrough,omitempty" yaml:"passthrough"`
	VaultRef    VaultRef  `json:"vault_ref,omitempty" yaml:"vault_ref"`
}

type Static struct {
	Headers map[string]string `json:"headers,omitempty" yaml:"headers"`
}

type PassThru struct {
	Headers []string `json:"headers,omitempty" yaml:"headers"`
}

type VaultRef struct {
	Headers map[string]string `json:"headers,omitempty" yaml:"headers"`
}

func ResolveDefaults(cfg Config) (map[string]string, error) {
	switch NormalizeMode(cfg.Mode) {
	case ModeStatic:
		return resolveStatic(cfg.Static)
	case ModeVaultRef:
		return resolveVault(cfg.VaultRef)
	case ModePassthrough:
		return nil, nil
	default:
		return nil, ErrInvalidAuth
	}
}

func resolveStatic(s Static) (map[string]string, error) {
	if len(s.Headers) == 0 {
		return map[string]string{}, nil
	}
	out := make(map[string]string, len(s.Headers))
	for k, v := range s.Headers {
		expanded, err := expandEnv(v)
		if err != nil {
			return nil, err
		}
		out[k] = expanded
	}
	return out, nil
}

func resolveVault(v VaultRef) (map[string]string, error) {
	if len(v.Headers) == 0 {
		return map[string]string{}, nil
	}
	out := make(map[string]string, len(v.Headers))
	for k, ref := range v.Headers {
		val, err := resolveVaultRef(ref)
		if err != nil {
			return nil, err
		}
		out[k] = val
	}
	return out, nil
}
