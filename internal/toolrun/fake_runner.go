package toolrun

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// FakeRunner is a test double for Runner. It records LookPath/Run calls and
// serves scripted stdout by binary basename or absolute path.
//
// It also records descriptor-bound starts (RecordBoundStart) so tests can
// assert that pathname Dir is always empty (Section 34.4).
//
// Production code must not use FakeRunner (test files only by convention;
// the type is exported for sibling package tests of generate later).
type FakeRunner struct {
	mu sync.Mutex

	// PathMap maps lookup names ("go", "git") or absolute paths → resolved path.
	// Missing entries make LookPath fail.
	PathMap map[string]string

	// Outputs maps resolved binary path → stdout for Run (first match).
	// When absent, Run returns an error.
	Outputs map[string]string

	// ExitErr maps resolved binary path → error returned from Run after capture.
	ExitErr map[string]error

	// ObservedEnv records env slices passed to Run (for bleed assertions).
	ObservedEnv [][]string
	// ObservedArgs records (binary, args) pairs.
	ObservedArgs [][]string

	// ObservedDirs records Dir values from RecordBoundStart / bound protocol.
	// Every entry must be "" (pathname Dir is prohibited).
	ObservedDirs []string
	// BoundStarts counts RecordBoundStart invocations.
	BoundStarts int
}

// RecordBoundStart records a descriptor-bound child start for Dir assertions.
// dir must be empty; tests fail closed if a pathname Dir is observed.
func (f *FakeRunner) RecordBoundStart(dir, binary string, args []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ObservedDirs = append(f.ObservedDirs, dir)
	f.BoundStarts++
	argRec := append([]string{binary}, args...)
	f.ObservedArgs = append(f.ObservedArgs, argRec)
}

// AllDirsEmpty reports whether every observed bound-start Dir was empty.
func (f *FakeRunner) AllDirsEmpty() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, d := range f.ObservedDirs {
		if d != "" {
			return false
		}
	}
	return true
}

// BoundStartCount returns the number of recorded bound starts.
func (f *FakeRunner) BoundStartCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.BoundStarts
}

// LookPath implements Runner.
func (f *FakeRunner) LookPath(file string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.PathMap == nil {
		return "", fmt.Errorf("look path %q: not found", file)
	}
	if p, ok := f.PathMap[file]; ok {
		return p, nil
	}
	// Also allow basenames of absolute scripted paths.
	if p, ok := f.PathMap[filepath.Base(file)]; ok {
		return p, nil
	}
	return "", fmt.Errorf("look path %q: not found", file)
}

// Run implements Runner.
func (f *FakeRunner) Run(_ context.Context, binary string, args []string, env []string) ([]byte, []byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	envCopy := append([]string(nil), env...)
	f.ObservedEnv = append(f.ObservedEnv, envCopy)
	argRec := append([]string{binary}, args...)
	f.ObservedArgs = append(f.ObservedArgs, argRec)

	out, ok := f.Outputs[binary]
	if !ok {
		out, ok = f.Outputs[filepath.Base(binary)]
	}
	if !ok {
		return nil, []byte("fake: no scripted output"), fmt.Errorf("fake runner: no output for %q", binary)
	}
	var err error
	if f.ExitErr != nil {
		if e, exists := f.ExitErr[binary]; exists {
			err = e
		} else if e, exists := f.ExitErr[filepath.Base(binary)]; exists {
			err = e
		}
	}
	return []byte(out), nil, err
}

// LastEnvMap returns the env of the most recent Run as a map.
func (f *FakeRunner) LastEnvMap() map[string]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.ObservedEnv) == 0 {
		return nil
	}
	return envSliceToMap(f.ObservedEnv[len(f.ObservedEnv)-1])
}

func envSliceToMap(env []string) map[string]string {
	m := make(map[string]string, len(env))
	for _, e := range env {
		k, v, ok := splitEnvKV(e)
		if !ok {
			continue
		}
		if _, exists := m[k]; !exists {
			m[k] = v
		}
	}
	return m
}

func splitEnvKV(e string) (k, v string, ok bool) {
	for i := 0; i < len(e); i++ {
		if e[i] == '=' {
			return e[:i], e[i+1:], true
		}
	}
	return "", "", false
}

// WriteFakeBinary creates an executable script at dir/name that prints
// stdoutText to stdout and exits 0. Used when tests exercise OSRunner with
// real PATH resolution (not FakeRunner).
func WriteFakeBinary(dir, name, stdoutText string) (string, error) {
	path := filepath.Join(dir, name)
	// Avoid shell metacharacters in payload; tests pass fixed version strings.
	body := "#!/bin/sh\nprintf '%s\\n' '" + shellSingleQuote(stdoutText) + "'\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		return "", err
	}
	return path, nil
}

// WriteNonExecutable creates a non-executable file at dir/name.
func WriteNonExecutable(dir, name, content string) (string, error) {
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func shellSingleQuote(s string) string {
	// Replace ' with '\'' for single-quoted sh strings.
	var b []byte
	for i := 0; i < len(s); i++ {
		if s[i] == '\'' {
			b = append(b, '\'', '\\', '\'', '\'')
		} else {
			b = append(b, s[i])
		}
	}
	return string(b)
}
