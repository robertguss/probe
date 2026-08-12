package engine

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// HitInput is the validated boundary input for probe hit.
type HitInput struct {
	Method      string
	URL         string
	Base        string
	Auth        string
	Headers     []string
	Query       []string
	Body        string
	BodyFile    string
	ContentType string
	Timeout     time.Duration
	Follow      bool
	Save        string
	NoSave      bool
	DryRun      bool
	Fields      string
	MaxBody     int64
	Retries     int
	MaxWait     time.Duration
	NoRetry     bool
	RPS         float64
	API         string
}

type hitData struct {
	ID         string            `json:"id,omitempty"`
	Method     string            `json:"method"`
	URL        string            `json:"url"`
	Status     int               `json:"status,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	Body       string            `json:"body,omitempty"`
	Attempts   int               `json:"attempts,omitempty"`
	DurationMS int64             `json:"duration_ms,omitempty"`
	DryRun     bool              `json:"dry_run,omitempty"`
	Saved      bool              `json:"saved,omitempty"`
	Truncated  bool              `json:"truncated,omitempty"`
}

func (e *Engine) hit(ctx context.Context, in HitInput) Result {
	example := "probe hit GET https://example.com --json"

	method := strings.ToUpper(strings.TrimSpace(in.Method))
	if method == "" {
		return e.usageError("hit", "missing METHOD", example)
	}
	rawURL := strings.TrimSpace(in.URL)
	if rawURL == "" {
		return e.usageError("hit", "missing URL", example)
	}
	if in.Body != "" && in.BodyFile != "" {
		return e.usageError("hit", "use only one of --body or --file", `probe hit POST https://example.com --body '{"a":1}' --json`)
	}

	sp, hasSpike := SpikePaths{}, false
	if sp2, err := e.openSpike(); err == nil {
		if _, statErr := os.Stat(sp2.Root); statErr == nil {
			sp = sp2
			hasSpike = true
		}
	}

	base := strings.TrimSpace(in.Base)
	if base == "" && hasSpike {
		if cfg, err := e.loadConfig(sp); err == nil {
			base = cfg.Base
		}
	}

	resolved, err := resolveURL(rawURL, base)
	if err != nil {
		return e.usageError("hit", err.Error(), example)
	}

	body, err := e.readHitBody(in)
	if err != nil {
		return e.fail("hit", ExitTransport, "transport", err.Error(), "", nil)
	}

	headers := map[string]string{}
	for _, h := range in.Headers {
		name, val, ok := splitHeader(h)
		if !ok {
			return e.usageError("hit", "invalid --header (want Name:Value)", `probe hit GET https://example.com --header Accept:application/json --json`)
		}
		headers[name] = val
	}
	if in.ContentType != "" {
		headers["Content-Type"] = in.ContentType
	}

	u, err := url.Parse(resolved)
	if err != nil {
		return e.usageError("hit", "invalid URL: "+err.Error(), example)
	}
	q := u.Query()
	for _, pair := range in.Query {
		k, v, ok := strings.Cut(pair, "=")
		if !ok || k == "" {
			return e.usageError("hit", "invalid --query (want key=value)", `probe hit GET https://example.com --query page=1 --json`)
		}
		q.Add(k, v)
	}
	u.RawQuery = q.Encode()
	resolved = u.String()

	var authName string
	if in.Auth != "" {
		if !hasSpike {
			return e.usageError("hit", "not a probe workspace (no .probe/); run init first", "probe init --json")
		}
		profiles, err := e.loadAuthProfiles(sp)
		if err != nil {
			return e.fail("hit", ExitTransport, "transport", err.Error(), "", nil)
		}
		p, ok := profiles[in.Auth]
		if !ok {
			return e.fail("hit", ExitUsage, "not_found", fmt.Sprintf("auth profile %q not found", in.Auth), "probe auth list --json", []string{"probe auth list --json"})
		}
		h, envName, err := e.materializeAuth(p)
		if err != nil {
			ex := fmt.Sprintf("probe hit %s %s --auth %s --json", method, rawURL, in.Auth)
			return e.authEnvMissing("hit", in.Auth, envName, ex)
		}
		authName = in.Auth
		headers[authHeaderNameOr(p)] = h.materialize()
	}

	timeout := in.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	maxBody := in.MaxBody
	if maxBody <= 0 {
		maxBody = 1 << 20
	}
	retries := in.Retries
	if retries < 0 {
		retries = 0
	}
	maxWait := in.MaxWait
	if maxWait <= 0 {
		maxWait = 60 * time.Second
	}

	planHeaders := RedactHeaders(headers)
	planURL := RedactURL(resolved)

	if in.DryRun {
		data := hitData{
			Method:  method,
			URL:     planURL,
			Headers: planHeaders,
			Body:    string(body),
			DryRun:  true,
		}
		return e.ok("hit", data)
	}

	req, err := http.NewRequestWithContext(ctx, method, resolved, nil)
	if err != nil {
		return e.fail("hit", ExitTransport, "transport", err.Error(), "", nil)
	}
	if len(body) > 0 {
		req.Body = io.NopCloser(bytes.NewReader(body))
		req.ContentLength = int64(len(body))
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(body)), nil
		}
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := e.httpClient(timeout, in.Follow)
	started := e.now()
	out, err := e.doHTTP(ctx, client, req, retries, maxWait, in.NoRetry, in.RPS)
	dur := e.now().Sub(started)
	if err != nil {
		return e.fail("hit", ExitTransport, "transport", err.Error(), "", nil)
	}

	bodyOut, truncated := truncateBody(out.Body, maxBody)
	saveName := in.Save
	if saveName == "" {
		saveName = "request"
	}
	shouldSave := hasSpike && !in.NoSave

	var id string
	if shouldSave {
		var allocErr error
		id, _, allocErr = e.allocateExchangeID(sp, saveName)
		if allocErr != nil {
			return e.fail("hit", ExitTransport, "transport", allocErr.Error(), "", nil)
		}
		reqFile := savedRequestFile{
			Method:  method,
			URL:     resolved,
			Headers: headers,
			Body:    string(body),
			Auth:    authName,
		}
		respFile := savedResponseFile{
			Status:  out.Status,
			Headers: headerMap(out.Header),
			Body:    string(bodyOut),
		}
		if err := e.persistExchange(sp, id, reqFile, respFile, dur.Milliseconds()); err != nil {
			return e.fail("hit", ExitTransport, "transport", err.Error(), "", nil)
		}
	}

	data := hitData{
		ID:         id,
		Method:     method,
		URL:        planURL,
		Status:     out.Status,
		Headers:    RedactHeaders(headerMap(out.Header)),
		Body:       string(bodyOut),
		Attempts:   out.Attempts,
		DurationMS: dur.Milliseconds(),
		Saved:      shouldSave,
		Truncated:  truncated,
	}
	if in.Fields != "" {
		data = filterHitFields(data, in.Fields)
	}

	code := classifyHTTP(out.Status, out.LimitedOut)
	res := Result{}
	switch code {
	case ExitSuccess:
		res = e.ok("hit", data)
	case ExitRateLimited:
		res = e.fail("hit", ExitRateLimited, "rate_limited", "retries exhausted on HTTP 429", "wait and retry with backoff", []string{example})
		res.Envelope.Data = data
	case ExitHTTP4xx:
		res = e.fail("hit", ExitHTTP4xx, "http_error", fmt.Sprintf("HTTP %d", out.Status), "", nil)
		res.Envelope.Data = data
	case ExitHTTP5xx:
		res = e.fail("hit", ExitHTTP5xx, "http_error", fmt.Sprintf("HTTP %d", out.Status), "", nil)
		res.Envelope.Data = data
	default:
		res = e.fail("hit", ExitTransport, "transport", fmt.Sprintf("unexpected status %d", out.Status), "", nil)
		res.Envelope.Data = data
	}
	if id != "" {
		res.Envelope.Meta.RequestID = id
	}
	return res
}

func authHeaderNameOr(p AuthProfile) string {
	if p.Type == AuthHeader && p.Header != "" {
		return p.Header
	}
	return "Authorization"
}

func resolveURL(raw, base string) (string, error) {
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			return "", fmt.Errorf("invalid URL: %w", err)
		}
		if u.Scheme == "" || u.Host == "" {
			return "", fmt.Errorf("invalid URL %q", raw)
		}
		return u.String(), nil
	}
	if base == "" {
		return "", fmt.Errorf("relative path %q requires --base or config base", raw)
	}
	b, err := url.Parse(base)
	if err != nil || b.Scheme == "" || b.Host == "" {
		return "", fmt.Errorf("invalid base URL %q", base)
	}
	ref, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid path %q", raw)
	}
	return b.ResolveReference(ref).String(), nil
}

func splitHeader(h string) (string, string, bool) {
	name, val, ok := strings.Cut(h, ":")
	if !ok {
		return "", "", false
	}
	name = strings.TrimSpace(name)
	val = strings.TrimSpace(val)
	if name == "" {
		return "", "", false
	}
	return name, val, true
}

func (e *Engine) readHitBody(in HitInput) ([]byte, error) {
	if in.Body != "" {
		return []byte(in.Body), nil
	}
	if in.BodyFile == "" {
		return nil, nil
	}
	if in.BodyFile == "-" {
		return io.ReadAll(e.opts.Stdin)
	}
	return os.ReadFile(in.BodyFile)
}

func filterHitFields(data hitData, fields string) hitData {
	want := map[string]bool{}
	for _, f := range strings.Split(fields, ",") {
		f = strings.TrimSpace(strings.ToLower(f))
		if f != "" {
			want[f] = true
		}
	}
	out := hitData{ID: data.ID, Method: data.Method, URL: data.URL, DryRun: data.DryRun, Saved: data.Saved}
	if want["status"] {
		out.Status = data.Status
	}
	if want["headers"] {
		out.Headers = data.Headers
	}
	if want["body"] {
		out.Body = data.Body
	}
	if want["attempts"] {
		out.Attempts = data.Attempts
	}
	if want["duration_ms"] || want["duration"] {
		out.DurationMS = data.DurationMS
	}
	if want["truncated"] {
		out.Truncated = data.Truncated
	}
	return out
}
