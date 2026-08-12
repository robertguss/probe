package engine

import (
	"context"
	"encoding/base64"
	"fmt"
	"sort"
	"strings"
)

// AuthType discriminates auth profile variants.
type AuthType string

const (
	AuthBearer AuthType = "bearer"
	AuthBasic  AuthType = "basic"
	AuthHeader AuthType = "header"
)

// AuthProfile stores env var names only — never secret values.
type AuthProfile struct {
	Name     string   `json:"name"`
	Type     AuthType `json:"type"`
	TokenEnv string   `json:"token_env,omitempty"`
	UserEnv  string   `json:"user_env,omitempty"`
	PassEnv  string   `json:"pass_env,omitempty"`
	Header   string   `json:"header,omitempty"`
	ValueEnv string   `json:"value_env,omitempty"`
}

func validateAuthProfile(p AuthProfile) error {
	switch p.Type {
	case AuthBearer:
		if strings.TrimSpace(p.TokenEnv) == "" {
			return fmt.Errorf("bearer profile %q requires token_env", p.Name)
		}
	case AuthBasic:
		if strings.TrimSpace(p.UserEnv) == "" || strings.TrimSpace(p.PassEnv) == "" {
			return fmt.Errorf("basic profile %q requires user_env and pass_env", p.Name)
		}
	case AuthHeader:
		if strings.TrimSpace(p.Header) == "" || strings.TrimSpace(p.ValueEnv) == "" {
			return fmt.Errorf("header profile %q requires name and value_env", p.Name)
		}
	default:
		return fmt.Errorf("auth profile %q: unknown type %q", p.Name, p.Type)
	}
	return nil
}

func (e *Engine) loadAuthProfiles(sp SpikePaths) (map[string]AuthProfile, error) {
	cfg, err := e.loadConfig(sp)
	if err != nil {
		return nil, err
	}
	return cfg.Auth, nil
}

func (e *Engine) materializeAuth(p AuthProfile) (AuthorizationHeader, string, error) {
	switch p.Type {
	case AuthBearer:
		v, ok := e.environ[p.TokenEnv]
		if !ok || v == "" {
			return AuthorizationHeader{}, p.TokenEnv, fmt.Errorf("missing")
		}
		return newAuthorizationHeader("Bearer " + v), "Authorization", nil
	case AuthBasic:
		user, okU := e.environ[p.UserEnv]
		pass, okP := e.environ[p.PassEnv]
		if !okU || user == "" {
			return AuthorizationHeader{}, p.UserEnv, fmt.Errorf("missing")
		}
		if !okP || pass == "" {
			return AuthorizationHeader{}, p.PassEnv, fmt.Errorf("missing")
		}
		raw := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
		return newAuthorizationHeader("Basic " + raw), "Authorization", nil
	case AuthHeader:
		v, ok := e.environ[p.ValueEnv]
		if !ok || v == "" {
			return AuthorizationHeader{}, p.ValueEnv, fmt.Errorf("missing")
		}
		return newAuthorizationHeader(v), p.Header, nil
	default:
		return AuthorizationHeader{}, "", fmt.Errorf("unknown auth type %q", p.Type)
	}
}

func (e *Engine) authEnvMissing(command, profile, envName, example string) Result {
	return e.fail(
		command,
		ExitUsage,
		"auth_env_missing",
		fmt.Sprintf("auth env %q is not set for profile %q", envName, profile),
		"fnox exec -- "+example,
		[]string{"fnox exec -- " + example},
	)
}

type authSetData struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type authListData struct {
	Profiles []AuthProfile `json:"profiles"`
}

func (e *Engine) authSet(_ context.Context, in AuthProfile) Result {
	sp, res, ok := e.requireSpike()
	if !ok {
		res.Envelope.Command = "auth.set"
		return res
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return e.usageError("auth.set", "missing profile name", "probe auth set canvas --type bearer --token-env CANVAS_TOKEN --json")
	}
	in.Type = AuthType(strings.ToLower(string(in.Type)))
	if err := validateAuthProfile(in); err != nil {
		return e.usageError("auth.set", err.Error(), "probe auth set canvas --type bearer --token-env CANVAS_TOKEN --json")
	}
	cfg, err := e.loadConfig(sp)
	if err != nil {
		return e.fail("auth.set", ExitTransport, "transport", err.Error(), "", nil)
	}
	cfg.Auth[in.Name] = in
	if err := e.saveConfig(sp, cfg); err != nil {
		return e.fail("auth.set", ExitTransport, "transport", err.Error(), "", nil)
	}
	return e.ok("auth.set", authSetData{Name: in.Name, Type: string(in.Type)})
}

func (e *Engine) authList(_ context.Context) Result {
	sp, res, ok := e.requireSpike()
	if !ok {
		res.Envelope.Command = "auth.list"
		return res
	}
	profiles, err := e.loadAuthProfiles(sp)
	if err != nil {
		return e.fail("auth.list", ExitTransport, "transport", err.Error(), "", nil)
	}
	names := make([]string, 0, len(profiles))
	for n := range profiles {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]AuthProfile, 0, len(names))
	for _, n := range names {
		out = append(out, profiles[n])
	}
	return e.ok("auth.list", authListData{Profiles: out})
}

func (e *Engine) authShow(_ context.Context, name string) Result {
	sp, res, ok := e.requireSpike()
	if !ok {
		res.Envelope.Command = "auth.show"
		return res
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return e.usageError("auth.show", "missing profile name", "probe auth show canvas --json")
	}
	profiles, err := e.loadAuthProfiles(sp)
	if err != nil {
		return e.fail("auth.show", ExitTransport, "transport", err.Error(), "", nil)
	}
	p, found := profiles[name]
	if !found {
		return e.fail("auth.show", ExitUsage, "not_found", fmt.Sprintf("auth profile %q not found", name), "probe auth list --json", []string{"probe auth list --json"})
	}
	return e.ok("auth.show", p)
}
