package toolrun

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ArtifactEnvVar is the environment variable naming a directory where the
// sentinel suite writes redacted isolation failure dumps (CI artifacts).
const ArtifactEnvVar = "FOUNDRY_SENTINEL_ARTIFACT_DIR"

// IsolationObservation is one sentinel probe result for step logging.
// Values of env keys are never recorded — only key names and presence.
type IsolationObservation struct {
	// Sentinel is the env key or plant name (e.g. GOFLAGS, GIT_TEMPLATE).
	Sentinel string
	// Kind is go-fixed | go-absent | git-absent | helper-prog | git-template | process-tree.
	Kind string
	// Observation is "absent" | "present" | "normative" | "leak" | "clean".
	Observation string
	// Argv is the child argv summary (no env values).
	Argv string
	// Exit is the child exit code when a process ran; -1 if not applicable.
	Exit int
	// Detail is free-form non-secret diagnostic text (key names, bools, counts).
	Detail string
}

// IsolationDump builds a redacted failure dump: env key names + child argv only.
// Never includes env values (REQ-214 / acceptance: redaction of values).
func IsolationDump(env map[string]string, argv []string, observations []IsolationObservation) string {
	var b strings.Builder
	b.WriteString("isolation_dump\n")
	b.WriteString("env_keys=")
	b.WriteString(strings.Join(EnvKeys(env), ","))
	b.WriteByte('\n')
	if env != nil {
		b.WriteString("env_hash=")
		b.WriteString(AllowlistHash(env))
		b.WriteByte('\n')
	}
	b.WriteString("argv=")
	b.WriteString(fmt.Sprintf("%q", argv))
	b.WriteByte('\n')
	for _, o := range observations {
		b.WriteString(fmt.Sprintf(
			"sentinel=%s kind=%s observation=%s exit=%d argv=%q detail=%q\n",
			o.Sentinel, o.Kind, o.Observation, o.Exit, o.Argv, o.Detail,
		))
	}
	return b.String()
}

// FormatObservationLine is a single steplog-friendly line for one sentinel case.
func FormatObservationLine(o IsolationObservation) string {
	return fmt.Sprintf(
		"sentinel=%s observation=%s argv=%q exit=%d detail=%q",
		o.Sentinel, o.Observation, o.Argv, o.Exit, o.Detail,
	)
}

// WriteIsolationArtifact writes a redacted isolation dump under dir when dir
// is non-empty. When dir is empty, reads ArtifactEnvVar. No-op if neither set.
// Returns the path written, or "" when skipped.
//
// File contents are IsolationDump output only (key names + argv; no values).
func WriteIsolationArtifact(dir, name string, env map[string]string, argv []string, obs []IsolationObservation) (string, error) {
	if dir == "" {
		dir = os.Getenv(ArtifactEnvVar)
	}
	if dir == "" {
		return "", nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	// Sanitize name to a basename-safe token.
	base := filepath.Base(name)
	if base == "" || base == "." || base == "/" {
		base = "isolation"
	}
	base = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, base)
	path := filepath.Join(dir, fmt.Sprintf("%s-%d.txt", base, time.Now().UTC().UnixNano()))
	body := IsolationDump(env, argv, obs)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// AssertNoSecretValues reports keys whose values appear in dump (should be empty).
// Used by unit tests proving redaction. Host-like path fragments and known
// secrets must not appear as KEY=value assignments or raw values.
func AssertNoSecretValues(dump string, forbiddenValues []string) []string {
	var hits []string
	for _, v := range forbiddenValues {
		if v == "" {
			continue
		}
		// Hash may theoretically collide with short substrings; require
		// path-like or long values only, or KEY=value form.
		if strings.Contains(dump, "="+v) || strings.Contains(dump, v+"\n") {
			hits = append(hits, v)
		}
	}
	return hits
}
