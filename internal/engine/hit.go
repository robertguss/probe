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

type hitPlan struct {
	method   string
	resolved string
	example  string
	headers  map[string]string
	secrets  WireSecrets
	policy   RedactionPolicy
	body     []byte
	authName string
	timeout  time.Duration
	maxWait  time.Duration
	maxBody  int64
	retries  int
	rps      float64
	follow   bool
	dryRun   bool
	noSave   bool
	noRetry  bool
	save     string
	fields   string
	sp       SpikePaths
	hasSpike bool
}

func (e *Engine) hit(ctx context.Context, in HitInput) Result {
	plan, res, ok := e.planHit(in)
	if !ok {
		return res
	}
	planHeaders, planURL := plan.policy.redactForPersist(plan.secrets.overlayNames(plan.headers), plan.resolved)
	if plan.dryRun {
		return e.ok("hit", hitData{
			Method:  plan.method,
			URL:     planURL,
			Headers: planHeaders,
			Body:    string(plan.body),
			DryRun:  true,
		})
	}

	req, err := http.NewRequestWithContext(ctx, plan.method, plan.resolved, nil)
	if err != nil {
		return e.fail("hit", ExitTransport, "transport", err.Error(), "", nil)
	}
	if len(plan.body) > 0 {
		req.Body = io.NopCloser(bytes.NewReader(plan.body))
		req.ContentLength = int64(len(plan.body))
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(plan.body)), nil
		}
	}
	for k, v := range plan.headers {
		req.Header.Set(k, v)
	}
	plan.secrets.apply(req)

	client := e.httpClient(plan.timeout, plan.follow)
	started := e.now()
	out, err := e.doHTTP(ctx, client, req, plan.retries, plan.maxWait, plan.noRetry, plan.rps)
	dur := e.now().Sub(started)
	if err != nil {
		return e.fail("hit", ExitTransport, "transport", err.Error(), "", nil)
	}

	bodyOut, truncated := truncateBody(out.Body, plan.maxBody)
	saveName := plan.save
	if saveName == "" {
		saveName = "request"
	}
	shouldSave := plan.hasSpike && !plan.noSave

	var id string
	if shouldSave {
		reqFile := savedRequestFile{
			Method:  plan.method,
			URL:     plan.resolved,
			Headers: plan.secrets.overlayNames(plan.headers),
			Body:    string(plan.body),
			Auth:    plan.authName,
		}
		respFile := savedResponseFile{
			Status:  out.Status,
			Headers: headerMap(out.Header),
			Body:    string(bodyOut),
		}
		var persistErr error
		id, persistErr = e.spikeStore(plan.sp).Commit(saveName, reqFile, respFile, dur.Milliseconds(), plan.policy)
		if persistErr != nil {
			return e.fail("hit", ExitTransport, "transport", persistErr.Error(), "", nil)
		}
	}

	data := hitData{
		ID:         id,
		Method:     plan.method,
		URL:        planURL,
		Status:     out.Status,
		Headers:    NewRedactionPolicy().redactHeaders(headerMap(out.Header)),
		Body:       string(bodyOut),
		Attempts:   out.Attempts,
		DurationMS: dur.Milliseconds(),
		Saved:      shouldSave,
		Truncated:  truncated,
	}
	if plan.fields != "" {
		data = filterHitFields(data, plan.fields)
	}
	res = e.classifyHitResult(out.Status, out.LimitedOut, data, plan.example)
	if id != "" {
		res.Envelope.Meta.RequestID = id
	}
	return res
}

func (e *Engine) classifyHitResult(status int, limited bool, data hitData, example string) Result {
	code := classifyHTTP(status, limited)
	switch code {
	case ExitSuccess:
		return e.ok("hit", data)
	case ExitRateLimited:
		return e.failWithData("hit", ExitRateLimited, "rate_limited", "retries exhausted on HTTP 429", "wait and retry with backoff", []string{example}, data)
	case ExitHTTP4xx, ExitHTTP5xx:
		return e.failWithData("hit", code, "http_error", fmt.Sprintf("HTTP %d", status), "", nil, data)
	default:
		return e.failWithData("hit", ExitTransport, "transport", fmt.Sprintf("unexpected status %d", status), "", nil, data)
	}
}

func (e *Engine) planHit(in HitInput) (hitPlan, Result, bool) {
	example := "probe hit GET https://example.com --json"
	method := strings.ToUpper(strings.TrimSpace(in.Method))
	if method == "" {
		return hitPlan{}, e.usageError("hit", "missing METHOD", example), false
	}
	rawURL := strings.TrimSpace(in.URL)
	if rawURL == "" {
		return hitPlan{}, e.usageError("hit", "missing URL", example), false
	}
	if in.Body != "" && in.BodyFile != "" {
		return hitPlan{}, e.usageError("hit", "use only one of --body or --file", `probe hit POST https://example.com --body '{"a":1}' --json`), false
	}

	plan := hitPlan{
		method:  method,
		example: example,
		policy:  NewRedactionPolicy(),
		dryRun:  in.DryRun,
		noSave:  in.NoSave,
		noRetry: in.NoRetry,
		save:    in.Save,
		fields:  in.Fields,
		follow:  in.Follow,
		rps:     in.RPS,
		timeout: in.Timeout,
		maxWait: in.MaxWait,
		maxBody: in.MaxBody,
		retries: in.Retries,
	}
	if plan.timeout <= 0 {
		plan.timeout = defaultTimeout
	}
	if plan.maxBody <= 0 {
		plan.maxBody = defaultMaxBody
	}
	if plan.retries < 0 {
		plan.retries = 0
	}
	if plan.maxWait <= 0 {
		plan.maxWait = defaultMaxWait
	}

	if sp2, err := e.openSpike(); err == nil {
		if _, statErr := os.Stat(sp2.Root); statErr == nil {
			plan.sp = sp2
			plan.hasSpike = true
		}
	}

	base := strings.TrimSpace(in.Base)
	if in.API != "" {
		catBase, catRPS := e.loadCatalogDefaults(in.API)
		if base == "" {
			base = catBase
		}
		if plan.rps <= 0 && catRPS > 0 {
			plan.rps = catRPS
		}
	}
	if base == "" && plan.hasSpike {
		if cfg, err := e.loadConfig(plan.sp); err == nil {
			base = cfg.Base
		}
	}

	resolved, err := resolveURL(rawURL, base)
	if err != nil {
		return hitPlan{}, e.usageError("hit", err.Error(), example), false
	}

	body, err := e.readHitBody(in)
	if err != nil {
		return hitPlan{}, e.fail("hit", ExitTransport, "transport", err.Error(), "", nil), false
	}
	plan.body = body

	headers := map[string]string{}
	for _, h := range in.Headers {
		name, val, ok := splitHeader(h)
		if !ok {
			return hitPlan{}, e.usageError("hit", "invalid --header (want Name:Value)", `probe hit GET https://example.com --header Accept:application/json --json`), false
		}
		headers[name] = val
	}
	if in.ContentType != "" {
		headers["Content-Type"] = in.ContentType
	} else if len(body) > 0 && headerLookup(headers, "Content-Type") == "" {
		headers["Content-Type"] = "application/json"
	}

	u, err := url.Parse(resolved)
	if err != nil {
		return hitPlan{}, e.usageError("hit", "invalid URL: "+err.Error(), example), false
	}
	q := u.Query()
	for _, pair := range in.Query {
		k, v, ok := strings.Cut(pair, "=")
		if !ok || k == "" {
			return hitPlan{}, e.usageError("hit", "invalid --query (want key=value)", `probe hit GET https://example.com --query page=1 --json`), false
		}
		q.Add(k, v)
	}
	u.RawQuery = q.Encode()
	plan.resolved = u.String()

	if in.Auth != "" {
		if !plan.hasSpike {
			return hitPlan{}, e.usageError("hit", "not a probe workspace (no .probe/); run init first", "probe init --json"), false
		}
		profiles, err := e.loadAuthProfiles(plan.sp)
		if err != nil {
			return hitPlan{}, e.fail("hit", ExitTransport, "transport", err.Error(), "", nil), false
		}
		p, found := profiles[in.Auth]
		if !found {
			return hitPlan{}, e.fail("hit", ExitUsage, "not_found", fmt.Sprintf("auth profile %q not found", in.Auth), "probe auth list --json", []string{"probe auth list --json"}), false
		}
		secrets, envName, err := e.materializeAuth(p)
		if err != nil {
			ex := fmt.Sprintf("probe hit %s %s --auth %s --json", method, rawURL, in.Auth)
			return hitPlan{}, e.authEnvMissing("hit", in.Auth, envName, ex), false
		}
		plan.authName = in.Auth
		plan.secrets = secrets
		plan.policy = policyForAuth(p)
		for name := range secrets.headers {
			deleteHeaderFold(headers, name)
		}
	}
	plan.headers = headers
	return plan, Result{}, true
}

func headerLookup(h map[string]string, name string) string {
	for k, v := range h {
		if strings.EqualFold(k, name) {
			return v
		}
	}
	return ""
}

func deleteHeaderFold(h map[string]string, name string) {
	for k := range h {
		if strings.EqualFold(k, name) {
			delete(h, k)
		}
	}
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
