package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitAuthPersistRoundTrip(t *testing.T) {
	dir := t.TempDir()
	var out, errBuf bytes.Buffer
	e := New(Options{
		Stdout:   &out,
		Stderr:   &errBuf,
		Getwd:    func() (string, error) { return dir, nil },
		SpikeDir: filepath.Join(dir, ".probe"),
		Environ:  []string{},
	})

	res := e.Run(context.Background(), []string{"init", "--json"})
	if res.ExitCode != ExitSuccess {
		t.Fatalf("init exit=%d stderr=%s out=%s", res.ExitCode, errBuf.String(), out.String())
	}
	sp := spikePathsFor(filepath.Join(dir, ".probe"))
	for _, p := range []string{sp.Config, sp.Session, sp.Notes, sp.Log, sp.Requests, sp.Responses} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("missing %s: %v", p, err)
		}
	}

	out.Reset()
	errBuf.Reset()
	res = e.Run(context.Background(), []string{
		"auth", "set", "canvas",
		"--type", "bearer",
		"--token-env", "CANVAS_TOKEN",
		"--json",
	})
	if res.ExitCode != ExitSuccess {
		t.Fatalf("auth set exit=%d stderr=%s", res.ExitCode, errBuf.String())
	}

	cfg, err := e.loadConfig(sp)
	if err != nil {
		t.Fatal(err)
	}
	p, ok := cfg.Auth["canvas"]
	if !ok || p.Type != AuthBearer || p.TokenEnv != "CANVAS_TOKEN" {
		t.Fatalf("config auth=%#v", cfg.Auth)
	}
	raw, _ := os.ReadFile(sp.Config)
	if strings.Contains(string(raw), "Bearer ") {
		t.Fatalf("secret-like value in config: %s", raw)
	}

	out.Reset()
	errBuf.Reset()
	e2 := New(Options{
		Stdout:   &out,
		Stderr:   &errBuf,
		Getwd:    func() (string, error) { return dir, nil },
		SpikeDir: filepath.Join(dir, ".probe"),
		Environ:  []string{"CANVAS_TOKEN=super-secret-value"},
	})
	res = e2.Run(context.Background(), []string{"auth", "show", "canvas", "--json"})
	if res.ExitCode != ExitSuccess {
		t.Fatalf("auth show exit=%d", res.ExitCode)
	}
	if strings.Contains(out.String(), "super-secret-value") {
		t.Fatalf("secret leaked in auth show: %s", out.String())
	}
	if !strings.Contains(out.String(), "CANVAS_TOKEN") {
		t.Fatalf("expected env name in show: %s", out.String())
	}

	out.Reset()
	res = e2.Run(context.Background(), []string{"auth", "list", "--json"})
	if res.ExitCode != ExitSuccess {
		t.Fatalf("auth list exit=%d", res.ExitCode)
	}

	id, err := e2.persistExchange(sp, "courses", savedRequestFile{
		Method:  "GET",
		URL:     "https://ex.test/x?token=sekrit",
		Headers: map[string]string{"Authorization": "Bearer super-secret-value", "Accept": "application/json"},
		Auth:    "canvas",
	}, savedResponseFile{
		Status:  200,
		Headers: map[string]string{"Set-Cookie": "sid=abc", "Content-Type": "application/json"},
		Body:    `{"ok":true}`,
	}, 12)
	if err != nil {
		t.Fatal(err)
	}
	if id != "001-courses" {
		t.Fatalf("id=%q", id)
	}

	id2, err := e2.persistExchange(sp, "header", savedRequestFile{
		Method:  "GET",
		URL:     "https://ex.test/y",
		Headers: map[string]string{"X-API-Key": "super-secret-apikey-value", "Accept": "application/json"},
		Auth:    "custom",
	}, savedResponseFile{
		Status:  200,
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    `{}`,
	}, 3, "X-API-Key")
	if err != nil {
		t.Fatal(err)
	}
	headerBytes, err := os.ReadFile(filepath.Join(sp.Requests, id2+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(headerBytes), "super-secret-apikey-value") {
		t.Fatalf("custom header secret in request artifact: %s", headerBytes)
	}
	if !strings.Contains(string(headerBytes), "[REDACTED]") {
		t.Fatalf("expected custom header redaction: %s", headerBytes)
	}

	reqBytes, err := os.ReadFile(filepath.Join(sp.Requests, "001-courses.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(reqBytes), "super-secret-value") || strings.Contains(string(reqBytes), "sekrit") {
		t.Fatalf("secret in request artifact: %s", reqBytes)
	}
	if !strings.Contains(string(reqBytes), "[REDACTED]") {
		t.Fatalf("expected redaction marker: %s", reqBytes)
	}

	out.Reset()
	errBuf.Reset()
	res = e2.Run(context.Background(), []string{"last", "--json"})
	if res.ExitCode != ExitSuccess {
		t.Fatalf("last exit=%d stderr=%s", res.ExitCode, errBuf.String())
	}
	var env Envelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if !env.OK || env.Meta.RequestID != "001-courses" {
		t.Fatalf("last envelope=%+v", env)
	}

	out.Reset()
	res = e2.Run(context.Background(), []string{"note", "pending via Link", "--json"})
	if res.ExitCode != ExitSuccess {
		t.Fatalf("note exit=%d", res.ExitCode)
	}
	notes, _ := os.ReadFile(sp.Notes)
	if !strings.Contains(string(notes), "pending via Link") {
		t.Fatalf("notes missing text: %s", notes)
	}

	out.Reset()
	res = e2.Run(context.Background(), []string{"summary", "--json"})
	if res.ExitCode != ExitSuccess {
		t.Fatalf("summary exit=%d", res.ExitCode)
	}
}

func TestInitIdempotent(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	e := New(Options{
		Stdout:   &out,
		Getwd:    func() (string, error) { return dir, nil },
		SpikeDir: filepath.Join(dir, ".probe"),
	})
	r1 := e.Run(context.Background(), []string{"init", "--json"})
	r2 := e.Run(context.Background(), []string{"init", "--json"})
	if r1.ExitCode != ExitSuccess || r2.ExitCode != ExitSuccess {
		t.Fatalf("exits %d %d", r1.ExitCode, r2.ExitCode)
	}
}
