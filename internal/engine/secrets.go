package engine

import (
	"net/http"
	"net/url"
	"strings"
)

const redacted = "[REDACTED]"

var denylistHeaders = map[string]struct{}{
	"authorization":  {},
	"cookie":         {},
	"set-cookie":     {},
	"x-api-key":      {},
	"api-key":        {},
	"x-auth-token":   {},
	"x-access-token": {},
}

var redactQueryParams = map[string]struct{}{
	"token":        {},
	"access_token": {},
	"api_key":      {},
	"apikey":       {},
	"key":          {},
	"secret":       {},
	"password":     {},
	"auth":         {},
}

// WireSecrets is an opaque bag of header-name → secret for the HTTP adapter only.
type WireSecrets struct {
	headers map[string]string
}

func (w WireSecrets) apply(req *http.Request) {
	for k, v := range w.headers {
		req.Header.Set(k, v)
	}
}

func (w WireSecrets) overlayNames(h map[string]string) map[string]string {
	out := copyHeaders(h)
	if out == nil {
		out = map[string]string{}
	}
	for k := range w.headers {
		found := false
		for ek := range out {
			if strings.EqualFold(ek, k) {
				out[ek] = redacted
				found = true
				break
			}
		}
		if !found {
			out[k] = redacted
		}
	}
	return out
}

// RedactionPolicy scrubs secrets for dry-run stdout, spike artifacts, and catalog fixtures.
type RedactionPolicy struct {
	extra []string
}

func NewRedactionPolicy(extra ...string) RedactionPolicy {
	return RedactionPolicy{extra: extra}
}

func policyForAuth(p AuthProfile) RedactionPolicy {
	switch p.Type {
	case AuthHeader:
		return NewRedactionPolicy(p.Header)
	default:
		return NewRedactionPolicy("Authorization")
	}
}

func (p RedactionPolicy) covers(name string) bool {
	lower := strings.ToLower(name)
	if _, ok := denylistHeaders[lower]; ok {
		return true
	}
	for _, x := range p.extra {
		if strings.EqualFold(name, x) {
			return true
		}
	}
	return false
}

func (p RedactionPolicy) redactForPersist(headers map[string]string, rawURL string) (map[string]string, string) {
	return p.redactHeaders(headers), redactURL(rawURL)
}

func (p RedactionPolicy) redactHeaders(h map[string]string) map[string]string {
	if h == nil {
		return nil
	}
	out := make(map[string]string, len(h))
	for k, v := range h {
		if p.covers(k) {
			out[k] = redacted
			continue
		}
		out[k] = v
	}
	return out
}

func copyHeaders(h map[string]string) map[string]string {
	if h == nil {
		return nil
	}
	out := make(map[string]string, len(h))
	for k, v := range h {
		out[k] = v
	}
	return out
}

func secretHeaderName(name, value string) bool {
	if NewRedactionPolicy().covers(name) {
		return true
	}
	return value == redacted
}

func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	changed := false
	if u.User != nil {
		u.User = nil
		changed = true
	}
	if u.RawQuery != "" {
		q := u.Query()
		for key := range q {
			if _, ok := redactQueryParams[strings.ToLower(key)]; ok {
				for i := range q[key] {
					q[key][i] = redacted
				}
				changed = true
			}
		}
		if changed {
			u.RawQuery = q.Encode()
		}
	}
	if !changed {
		return raw
	}
	return u.String()
}

// RedactHeaders is a thin wrapper over RedactionPolicy for callers that only need header scrubbing.
func RedactHeaders(h map[string]string, extra ...string) map[string]string {
	return NewRedactionPolicy(extra...).redactHeaders(h)
}

// RedactURL redacts userinfo and obvious token query parameter values.
func RedactURL(raw string) string {
	return redactURL(raw)
}
