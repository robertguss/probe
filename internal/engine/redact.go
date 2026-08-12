package engine

import (
	"net/url"
	"strings"
)

const redacted = "[REDACTED]"

var redactHeaderNames = map[string]struct{}{
	"authorization": {},
	"cookie":        {},
	"set-cookie":    {},
}

var redactQueryParams = map[string]struct{}{
	"token":         {},
	"access_token":  {},
	"api_key":       {},
	"apikey":        {},
	"key":           {},
	"secret":        {},
	"password":      {},
	"auth":          {},
}

// RedactHeaders returns a copy with Authorization, Cookie, and Set-Cookie
// values replaced by [REDACTED]. Matching is case-insensitive on names.
func RedactHeaders(h map[string]string) map[string]string {
	if h == nil {
		return nil
	}
	out := make(map[string]string, len(h))
	for k, v := range h {
		if _, ok := redactHeaderNames[strings.ToLower(k)]; ok {
			out[k] = redacted
			continue
		}
		out[k] = v
	}
	return out
}

// RedactURL redacts obvious token query parameter values.
func RedactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.RawQuery == "" {
		return raw
	}
	q := u.Query()
	changed := false
	for key := range q {
		if _, ok := redactQueryParams[strings.ToLower(key)]; ok {
			for i := range q[key] {
				q[key][i] = redacted
			}
			changed = true
		}
	}
	if !changed {
		return raw
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// AuthorizationHeader holds a materialized auth header value for transport only.
type AuthorizationHeader struct {
	raw string
}

func (h AuthorizationHeader) String() string { return redacted }

func (h AuthorizationHeader) MarshalJSON() ([]byte, error) {
	return []byte(`"` + redacted + `"`), nil
}

func (h AuthorizationHeader) materialize() string { return h.raw }

func newAuthorizationHeader(raw string) AuthorizationHeader {
	return AuthorizationHeader{raw: raw}
}
