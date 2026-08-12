package diagnostic

import (
	"net/url"
	"regexp"
	"strings"
)

// RedactedSentinel is the stable replacement for secret-bearing values in
// diagnostics (text and JSON forms). Tests assert this exact sentinel.
const RedactedSentinel = "[REDACTED]"

// secretEnvKey matches KEY=value where KEY looks credential-bearing.
// Captures the key name only for safe failure logging (never values).
var secretEnvAssign = regexp.MustCompile(
	`(?i)\b([A-Z][A-Z0-9_]*(?:SECRET|TOKEN|PASSWORD|PASSWD|API_?KEY|AUTH|CREDENTIAL|PRIVATE_?KEY|COOKIE|SESSION|ACCESS_KEY)[A-Z0-9_]*)=([^\s"'\\]+)`,
)

// secretKeyValue matches password=..., token: ..., etc. in free text.
var secretKeyValue = regexp.MustCompile(
	`(?i)\b(password|passwd|secret|token|api[_-]?key|auth|authorization|credential|private[_-]?key|cookie|session|access[_-]?key)\b\s*[=:]\s*([^\s,;\"']+)`,
)

// bearerToken matches Authorization bearer forms.
var bearerToken = regexp.MustCompile(`(?i)(Bearer\s+)(\S+)`)

// basicAuthHeader matches Basic base64 blobs in headers.
var basicAuthHeader = regexp.MustCompile(`(?i)(Basic\s+)([A-Za-z0-9+/=]+)`)

// LooksSecret reports whether key (env or field name) must never be emitted
// as a raw value in diagnostics.
func LooksSecret(key string) bool {
	k := strings.ToUpper(strings.TrimSpace(key))
	if k == "" {
		return false
	}
	switch k {
	case "PASSWORD", "PASSWD", "SECRET", "TOKEN",
		"AUTHORIZATION", "AUTH", "COOKIE", "SESSION",
		"CREDENTIAL", "CREDENTIALS":
		return true
	}
	// Suffix / substring patterns (*_PASSWORD, *_TOKEN, *_SECRET, …).
	for _, frag := range []string{
		"SECRET", "TOKEN", "PASSWORD", "PASSWD",
		"API_KEY", "APIKEY", "PRIVATE_KEY", "PRIVATEKEY",
		"CREDENTIAL", "ACCESS_KEY", "AUTH",
	} {
		if strings.Contains(k, frag) {
			return true
		}
	}
	// Proxy URLs often carry userinfo.
	if k == "HTTP_PROXY" || k == "HTTPS_PROXY" || k == "ALL_PROXY" ||
		k == "NO_PROXY" || k == "http_proxy" || k == "https_proxy" ||
		k == "all_proxy" || k == "no_proxy" || k == "GOPROXY" {
		// GOPROXY is not always secret, but may include userinfo; treat as
		// needing value redaction when userinfo present (handled in Redact/RedactURL).
		return k != "GOPROXY" && k != "NO_PROXY" && k != "no_proxy"
	}
	return false
}

// Redact replaces known-sensitive values in s with RedactedSentinel.
// Safe for empty strings; never panics. On conceptual "failure" callers should
// log key names only — this function never returns secret values.
func Redact(s string) string {
	if s == "" {
		return s
	}
	// Bearer/Basic before generic key:value so "Authorization: Bearer tok" is
	// not partially matched as authorization=Bearer leaving the token behind.
	out := bearerToken.ReplaceAllString(s, "${1}"+RedactedSentinel)
	out = basicAuthHeader.ReplaceAllString(out, "${1}"+RedactedSentinel)
	out = secretEnvAssign.ReplaceAllString(out, "${1}="+RedactedSentinel)
	out = secretKeyValue.ReplaceAllString(out, "${1}="+RedactedSentinel)
	out = redactURLsInText(out)
	return out
}

// RedactURL redacts userinfo in a URL (proxy credentials, etc.).
// Non-URL strings are returned through Redact. The sentinel is inserted
// literally (not percent-encoded) so text and JSON forms share one marker.
func RedactURL(raw string) string {
	if raw == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		return Redact(raw)
	}
	if u.User == nil {
		return raw
	}
	_, hasPass := u.User.Password()
	hasUser := u.User.Username() != ""
	if !hasUser && !hasPass {
		return raw
	}
	userinfo := RedactedSentinel
	if hasPass {
		userinfo = RedactedSentinel + ":" + RedactedSentinel
	}
	// Rebuild scheme://userinfo@rest without percent-encoding the sentinel.
	u.User = nil
	rest := u.String()
	scheme := u.Scheme + "://"
	if !strings.HasPrefix(rest, scheme) {
		return rest
	}
	return scheme + userinfo + "@" + strings.TrimPrefix(rest, scheme)
}

// RedactEnv returns a copy of env key/value pairs with secret values replaced.
// Proxy URL values have userinfo redacted. Key names are never modified.
func RedactEnv(env map[string]string) map[string]string {
	if env == nil {
		return nil
	}
	out := make(map[string]string, len(env))
	for k, v := range env {
		out[k] = RedactEnvValue(k, v)
	}
	return out
}

// RedactEnvValue redacts a single env value based on its key name and content.
func RedactEnvValue(key, value string) string {
	if value == "" {
		return value
	}
	uk := strings.ToUpper(strings.TrimSpace(key))
	switch uk {
	case "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "GOPROXY",
		"http_proxy", "https_proxy", "all_proxy":
		return RedactURL(value)
	}
	if LooksSecret(key) {
		return RedactedSentinel
	}
	// Still scrub secret-shaped content that leaked into non-secret keys.
	return Redact(value)
}

// redactURLsInText finds URL-like substrings with userinfo and redacts them.
var urlWithUserinfo = regexp.MustCompile(`(?i)\b((?:https?|socks5?|git)://)([^/@\s]+@)([^\s]+)`)

func redactURLsInText(s string) string {
	return urlWithUserinfo.ReplaceAllStringFunc(s, func(m string) string {
		return RedactURL(m)
	})
}
