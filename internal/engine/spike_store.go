package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var containNameRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// SpikeStore owns spike request/response IO under .probe/.
type SpikeStore struct {
	e  *Engine
	sp SpikePaths
}

func (e *Engine) spikeStore(sp SpikePaths) SpikeStore {
	return SpikeStore{e: e, sp: sp}
}

type queryKind int

const (
	queryByID queryKind = iota
	queryLast
	queryHint
)

// Query selects a saved exchange: ByID, Last, or Hint.
type Query struct {
	kind queryKind
	id   string
	hint string
}

func ByID(id string) Query { return Query{kind: queryByID, id: id} }

func LastQuery() Query { return Query{kind: queryLast} }

func Hint(hint string) Query { return Query{kind: queryHint, hint: hint} }

func containPath(root, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || filepath.IsAbs(name) || !containNameRe.MatchString(name) {
		return "", fmt.Errorf("invalid path name %q", name)
	}
	if strings.Contains(name, "..") {
		return "", fmt.Errorf("invalid path name %q", name)
	}
	joined := filepath.Join(root, name)
	rel, err := filepath.Rel(root, joined)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path escapes root")
	}
	return joined, nil
}

func (s SpikeStore) Commit(name string, req savedRequestFile, resp savedResponseFile, durationMS int64, policy RedactionPolicy) (string, error) {
	sess, err := s.e.loadSession(s.sp)
	if err != nil {
		return "", err
	}
	n := sess.NextID
	if n < 1 {
		n = 1
	}
	id := exchangeID(n, name)
	req.ID = id
	resp.ID = id
	req.Headers, req.URL = policy.redactForPersist(req.Headers, req.URL)
	resp.Headers, _ = policy.redactForPersist(resp.Headers, "")

	rb, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		return "", err
	}
	rb = append(rb, '\n')
	reqPath, err := containPath(s.sp.Requests, id+".json")
	if err != nil {
		return "", err
	}
	respPath, err := containPath(s.sp.Responses, id+".json")
	if err != nil {
		return "", err
	}
	if err := writeFileAtomic(reqPath, rb, 0o644); err != nil {
		return "", err
	}
	rollback := func() {
		_ = os.Remove(reqPath)
		_ = os.Remove(respPath)
	}
	sb, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		rollback()
		return "", err
	}
	sb = append(sb, '\n')
	if err := writeFileAtomic(respPath, sb, 0o644); err != nil {
		rollback()
		return "", err
	}

	entry := logEntry{
		ID:         id,
		Method:     req.Method,
		URL:        req.URL,
		Status:     resp.Status,
		DurationMS: durationMS,
		At:         s.e.now().UTC().Format(time.RFC3339Nano),
	}
	lb, err := json.Marshal(entry)
	if err != nil {
		rollback()
		return "", err
	}
	f, err := os.OpenFile(s.sp.Log, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		rollback()
		return "", err
	}
	_, werr := f.Write(append(lb, '\n'))
	_ = f.Close()
	if werr != nil {
		rollback()
		return "", werr
	}

	sess.LastID = id
	sess.NextID = n + 1
	if err := s.e.saveSession(s.sp, sess); err != nil {
		rollback()
		return "", err
	}
	return id, nil
}

func (s SpikeStore) Load(q Query, policy RedactionPolicy) (savedRequestFile, savedResponseFile, error) {
	var req savedRequestFile
	var resp savedResponseFile
	var id string
	switch q.kind {
	case queryByID:
		id = strings.TrimSuffix(strings.TrimSpace(q.id), ".json")
	case queryLast:
		sess, err := s.e.loadSession(s.sp)
		if err != nil {
			return req, resp, err
		}
		if sess.LastID == "" {
			return req, resp, os.ErrNotExist
		}
		id = sess.LastID
	case queryHint:
		resolved, err := s.resolveHint(q.hint)
		if err != nil {
			return req, resp, err
		}
		id = resolved
	default:
		return req, resp, fmt.Errorf("unknown query")
	}
	return s.loadID(id, policy)
}

func (s SpikeStore) loadID(id string, policy RedactionPolicy) (savedRequestFile, savedResponseFile, error) {
	var req savedRequestFile
	var resp savedResponseFile
	reqPath, err := containPath(s.sp.Requests, id+".json")
	if err != nil {
		return req, resp, err
	}
	respPath, err := containPath(s.sp.Responses, id+".json")
	if err != nil {
		return req, resp, err
	}
	rb, err := os.ReadFile(reqPath)
	if err != nil {
		return req, resp, err
	}
	sb, err := os.ReadFile(respPath)
	if err != nil {
		return req, resp, err
	}
	if err := json.Unmarshal(rb, &req); err != nil {
		return req, resp, err
	}
	if err := json.Unmarshal(sb, &resp); err != nil {
		return req, resp, err
	}
	req.Headers, req.URL = policy.redactForPersist(req.Headers, req.URL)
	resp.Headers, _ = policy.redactForPersist(resp.Headers, "")
	return req, resp, nil
}

func (s SpikeStore) resolveHint(hint string) (string, error) {
	matches, err := s.Find(hint)
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", os.ErrNotExist
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("ambiguous hint %q (%d matches)", hint, len(matches))
	}
	return matches[0].ID, nil
}

func (s SpikeStore) Find(hint string) ([]findMatch, error) {
	hint = strings.TrimSpace(hint)
	if hint == "" {
		return nil, fmt.Errorf("empty hint")
	}
	entries, err := os.ReadDir(s.sp.Requests)
	if err != nil {
		return nil, err
	}
	found := make([]findMatch, 0)
	h := strings.ToLower(hint)
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(ent.Name(), ".json")
		path, err := containPath(s.sp.Requests, ent.Name())
		if err != nil {
			continue
		}
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var req savedRequestFile
		if err := json.Unmarshal(b, &req); err != nil {
			continue
		}
		if strings.Contains(strings.ToLower(id), h) || strings.Contains(strings.ToLower(req.URL), h) || strings.Contains(strings.ToLower(req.Method), h) {
			found = append(found, findMatch{ID: id, Method: req.Method, URL: req.URL})
		}
	}
	return found, nil
}
