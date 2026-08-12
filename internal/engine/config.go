package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the on-disk .probe/config.yaml model.
type Config struct {
	Base string                 `yaml:"base,omitempty" json:"base,omitempty"`
	Auth map[string]AuthProfile `yaml:"auth,omitempty" json:"auth,omitempty"`
}

// Session tracks spike request numbering.
type Session struct {
	NextID int    `json:"next_id"`
	LastID string `json:"last_id"`
}

func (e *Engine) loadConfig(sp SpikePaths) (Config, error) {
	b, err := os.ReadFile(sp.Config)
	if err != nil {
		return Config{}, err
	}
	var wire configWire
	if err := yaml.Unmarshal(b, &wire); err != nil {
		return Config{}, fmt.Errorf("parse config.yaml: %w", err)
	}
	cfg := Config{
		Base: wire.Base,
		Auth: map[string]AuthProfile{},
	}
	for name, w := range wire.Auth {
		p := AuthProfile{
			Name:     name,
			Type:     AuthType(strings.ToLower(w.Type)),
			TokenEnv: w.TokenEnv,
			UserEnv:  w.UserEnv,
			PassEnv:  w.PassEnv,
			Header:   w.Name,
			ValueEnv: w.ValueEnv,
		}
		if err := validateAuthProfile(p); err != nil {
			return Config{}, err
		}
		cfg.Auth[name] = p
	}
	return cfg, nil
}

func (e *Engine) saveConfig(sp SpikePaths, cfg Config) error {
	if cfg.Auth == nil {
		cfg.Auth = map[string]AuthProfile{}
	}
	out := configWire{
		Base: cfg.Base,
		Auth: map[string]authWire{},
	}
	for name, p := range cfg.Auth {
		out.Auth[name] = authToWire(p)
	}
	b, err := yaml.Marshal(&out)
	if err != nil {
		return err
	}
	return writeFileAtomic(sp.Config, b, 0o644)
}

type configWire struct {
	Base string              `yaml:"base,omitempty"`
	Auth map[string]authWire `yaml:"auth,omitempty"`
}

type authWire struct {
	Type     string `yaml:"type"`
	TokenEnv string `yaml:"token_env,omitempty"`
	UserEnv  string `yaml:"user_env,omitempty"`
	PassEnv  string `yaml:"pass_env,omitempty"`
	Name     string `yaml:"name,omitempty"`
	ValueEnv string `yaml:"value_env,omitempty"`
}

func authToWire(p AuthProfile) authWire {
	return authWire{
		Type:     string(p.Type),
		TokenEnv: p.TokenEnv,
		UserEnv:  p.UserEnv,
		PassEnv:  p.PassEnv,
		Name:     p.Header,
		ValueEnv: p.ValueEnv,
	}
}

func (e *Engine) loadSession(sp SpikePaths) (Session, error) {
	b, err := os.ReadFile(sp.Session)
	if err != nil {
		return Session{}, err
	}
	var s Session
	if err := json.Unmarshal(b, &s); err != nil {
		return Session{}, fmt.Errorf("parse session.json: %w", err)
	}
	if s.NextID < 1 {
		s.NextID = 1
	}
	return s, nil
}

func (e *Engine) saveSession(sp SpikePaths, s Session) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return writeFileAtomic(sp.Session, b, 0o644)
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	ok = true
	return nil
}
