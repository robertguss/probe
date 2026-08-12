package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var saveNameRe = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

type initData struct {
	Path    string `json:"path"`
	Created bool   `json:"created"`
}

func (e *Engine) initSpike(_ context.Context, dir string) Result {
	cwd, err := e.cwd()
	if err != nil {
		return e.fail("init", ExitTransport, "transport", err.Error(), "", nil)
	}
	root := dir
	if root == "" {
		if e.opts.SpikeDir != "" {
			root = e.opts.SpikeDir
		} else {
			root = filepath.Join(cwd, ".probe")
		}
	} else if !filepath.IsAbs(root) {
		root = filepath.Join(cwd, root)
	}
	sp := spikePathsFor(root)
	created := false
	if _, err := os.Stat(sp.Root); os.IsNotExist(err) {
		created = true
	}
	if err := os.MkdirAll(sp.Requests, 0o755); err != nil {
		return e.fail("init", ExitTransport, "transport", err.Error(), "", nil)
	}
	if err := os.MkdirAll(sp.Responses, 0o755); err != nil {
		return e.fail("init", ExitTransport, "transport", err.Error(), "", nil)
	}
	if _, err := os.Stat(sp.Config); os.IsNotExist(err) {
		if err := e.saveConfig(sp, Config{Auth: map[string]AuthProfile{}}); err != nil {
			return e.fail("init", ExitTransport, "transport", err.Error(), "", nil)
		}
	}
	if _, err := os.Stat(sp.Session); os.IsNotExist(err) {
		if err := e.saveSession(sp, Session{NextID: 1}); err != nil {
			return e.fail("init", ExitTransport, "transport", err.Error(), "", nil)
		}
	}
	if _, err := os.Stat(sp.Notes); os.IsNotExist(err) {
		if err := os.WriteFile(sp.Notes, []byte("# probe notes\n"), 0o644); err != nil {
			return e.fail("init", ExitTransport, "transport", err.Error(), "", nil)
		}
	}
	if _, err := os.Stat(sp.Log); os.IsNotExist(err) {
		if err := os.WriteFile(sp.Log, nil, 0o644); err != nil {
			return e.fail("init", ExitTransport, "transport", err.Error(), "", nil)
		}
	}
	e.spike = sp
	return e.ok("init", initData{Path: sp.Root, Created: created})
}

type savedRequestFile struct {
	ID      string            `json:"id"`
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body,omitempty"`
	Auth    string            `json:"auth,omitempty"`
}

type savedResponseFile struct {
	ID      string            `json:"id"`
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body,omitempty"`
}

type logEntry struct {
	ID         string `json:"id"`
	Method     string `json:"method"`
	URL        string `json:"url"`
	Status     int    `json:"status"`
	DurationMS int64  `json:"duration_ms"`
	At         string `json:"at"`
}

func sanitizeSaveName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "request"
	}
	if !saveNameRe.MatchString(name) {
		var b strings.Builder
		for _, r := range name {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
				b.WriteRune(r)
			} else {
				b.WriteByte('-')
			}
		}
		name = b.String()
	}
	if name == "" {
		return "request"
	}
	return name
}

func exchangeID(n int, name string) string {
	return fmt.Sprintf("%03d-%s", n, sanitizeSaveName(name))
}

func (e *Engine) persistExchange(sp SpikePaths, id string, req savedRequestFile, resp savedResponseFile, durationMS int64) error {
	req.ID = id
	resp.ID = id
	req.URL = RedactURL(req.URL)
	req.Headers = RedactHeaders(req.Headers)
	resp.Headers = RedactHeaders(resp.Headers)

	rb, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		return err
	}
	rb = append(rb, '\n')
	if err := writeFileAtomic(filepath.Join(sp.Requests, id+".json"), rb, 0o644); err != nil {
		return err
	}
	sb, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return err
	}
	sb = append(sb, '\n')
	if err := writeFileAtomic(filepath.Join(sp.Responses, id+".json"), sb, 0o644); err != nil {
		return err
	}

	entry := logEntry{
		ID:         id,
		Method:     req.Method,
		URL:        req.URL,
		Status:     resp.Status,
		DurationMS: durationMS,
		At:         e.now().UTC().Format(time.RFC3339Nano),
	}
	lb, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(sp.Log, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(lb, '\n')); err != nil {
		return err
	}

	sess, err := e.loadSession(sp)
	if err != nil {
		return err
	}
	sess.LastID = id
	return e.saveSession(sp, sess)
}

func (e *Engine) allocateExchangeID(sp SpikePaths, name string) (string, int, error) {
	sess, err := e.loadSession(sp)
	if err != nil {
		return "", 0, err
	}
	n := sess.NextID
	if n < 1 {
		n = 1
	}
	id := exchangeID(n, name)
	sess.NextID = n + 1
	if err := e.saveSession(sp, sess); err != nil {
		return "", 0, err
	}
	return id, n, nil
}

func (e *Engine) now() time.Time {
	if e.opts.Now != nil {
		return e.opts.Now()
	}
	return time.Now()
}

func (e *Engine) note(_ context.Context, text string) Result {
	sp, res, ok := e.requireSpike()
	if !ok {
		res.Envelope.Command = "note"
		return res
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return e.usageError("note", "missing note text", `probe note "auth works with staging token" --json`)
	}
	line := fmt.Sprintf("\n- %s (%s)\n", text, e.now().UTC().Format(time.RFC3339))
	f, err := os.OpenFile(sp.Notes, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return e.fail("note", ExitTransport, "transport", err.Error(), "", nil)
	}
	defer f.Close()
	if _, err := f.WriteString(line); err != nil {
		return e.fail("note", ExitTransport, "transport", err.Error(), "", nil)
	}
	return e.ok("note", map[string]string{"path": sp.Notes, "text": text})
}

type summaryData struct {
	LastID   string `json:"last_id,omitempty"`
	NextID   int    `json:"next_id"`
	Requests int    `json:"requests"`
}

func (e *Engine) summary(_ context.Context) Result {
	sp, res, ok := e.requireSpike()
	if !ok {
		res.Envelope.Command = "summary"
		return res
	}
	sess, err := e.loadSession(sp)
	if err != nil {
		return e.fail("summary", ExitTransport, "transport", err.Error(), "", nil)
	}
	entries, err := os.ReadDir(sp.Requests)
	if err != nil {
		return e.fail("summary", ExitTransport, "transport", err.Error(), "", nil)
	}
	n := 0
	for _, ent := range entries {
		if !ent.IsDir() && strings.HasSuffix(ent.Name(), ".json") {
			n++
		}
	}
	return e.ok("summary", summaryData{LastID: sess.LastID, NextID: sess.NextID, Requests: n})
}

func (e *Engine) replay(ctx context.Context, id string) Result {
	sp, res, ok := e.requireSpike()
	if !ok {
		res.Envelope.Command = "replay"
		return res
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return e.usageError("replay", "missing request id", "probe replay 001-courses --json")
	}
	id = strings.TrimSuffix(id, ".json")
	path := filepath.Join(sp.Requests, id+".json")
	if _, err := os.Stat(path); err != nil {
		matches, _ := filepath.Glob(filepath.Join(sp.Requests, "*"+id+"*.json"))
		if len(matches) == 1 {
			id = strings.TrimSuffix(filepath.Base(matches[0]), ".json")
			path = matches[0]
		} else {
			return e.fail("replay", ExitUsage, "not_found", fmt.Sprintf("request %q not found", id), "probe last --json", []string{"probe last --json"})
		}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return e.fail("replay", ExitTransport, "transport", err.Error(), "", nil)
	}
	var req savedRequestFile
	if err := json.Unmarshal(b, &req); err != nil {
		return e.fail("replay", ExitTransport, "transport", err.Error(), "", nil)
	}
	in := HitInput{
		Method:  req.Method,
		URL:     req.URL,
		Auth:    req.Auth,
		Body:    req.Body,
		Save:    "replay",
		Retries: 2,
		Timeout: 30 * time.Second,
		MaxWait: 60 * time.Second,
		MaxBody: 1 << 20,
		Follow:  strings.EqualFold(req.Method, "GET"),
	}
	for k, v := range req.Headers {
		if strings.EqualFold(k, "Authorization") || strings.EqualFold(k, "Cookie") {
			continue
		}
		in.Headers = append(in.Headers, k+":"+v)
	}
	return e.hit(ctx, in)
}

func (e *Engine) find(_ context.Context, hint string) Result {
	sp, res, ok := e.requireSpike()
	if !ok {
		res.Envelope.Command = "find"
		return res
	}
	hint = strings.TrimSpace(hint)
	if hint == "" {
		return e.usageError("find", "missing path hint", "probe find courses --json")
	}
	entries, err := os.ReadDir(sp.Requests)
	if err != nil {
		return e.fail("find", ExitTransport, "transport", err.Error(), "", nil)
	}
	type hit struct {
		ID     string `json:"id"`
		Method string `json:"method"`
		URL    string `json:"url"`
	}
	var found []hit
	h := strings.ToLower(hint)
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(ent.Name(), ".json")
		b, err := os.ReadFile(filepath.Join(sp.Requests, ent.Name()))
		if err != nil {
			continue
		}
		var req savedRequestFile
		if err := json.Unmarshal(b, &req); err != nil {
			continue
		}
		if strings.Contains(strings.ToLower(id), h) || strings.Contains(strings.ToLower(req.URL), h) || strings.Contains(strings.ToLower(req.Method), h) {
			found = append(found, hit{ID: id, Method: req.Method, URL: req.URL})
		}
	}
	return e.ok("find", map[string]any{"hint": hint, "matches": found})
}

func (e *Engine) lastExchange(_ context.Context) Result {
	sp, res, ok := e.requireSpike()
	if !ok {
		res.Envelope.Command = "last"
		return res
	}
	sess, err := e.loadSession(sp)
	if err != nil {
		return e.fail("last", ExitTransport, "transport", err.Error(), "", nil)
	}
	if sess.LastID == "" {
		return e.fail("last", ExitUsage, "not_found", "no recorded exchange yet", "probe hit GET https://example.com --json", []string{"probe hit GET https://example.com --json"})
	}
	rb, err := os.ReadFile(filepath.Join(sp.Requests, sess.LastID+".json"))
	if err != nil {
		return e.fail("last", ExitTransport, "transport", err.Error(), "", nil)
	}
	sb, err := os.ReadFile(filepath.Join(sp.Responses, sess.LastID+".json"))
	if err != nil {
		return e.fail("last", ExitTransport, "transport", err.Error(), "", nil)
	}
	var req savedRequestFile
	var resp savedResponseFile
	if err := json.Unmarshal(rb, &req); err != nil {
		return e.fail("last", ExitTransport, "transport", err.Error(), "", nil)
	}
	if err := json.Unmarshal(sb, &resp); err != nil {
		return e.fail("last", ExitTransport, "transport", err.Error(), "", nil)
	}
	out := e.ok("last", map[string]any{
		"id":       sess.LastID,
		"request":  req,
		"response": resp,
	})
	out.Envelope.Meta.RequestID = sess.LastID
	return out
}
