package engine

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type roundTripOutcome struct {
	Status     int
	Header     http.Header
	Body       []byte
	Attempts   int
	LimitedOut bool // retries exhausted on 429
}

func (e *Engine) httpClient(timeout time.Duration, follow bool) *http.Client {
	transport := e.opts.HTTP
	if transport == nil {
		transport = http.DefaultTransport
	}
	c := &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}
	if !follow {
		c.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
	return c
}

func parseRetryAfter(h http.Header, now time.Time) (time.Duration, bool) {
	raw := strings.TrimSpace(h.Get("Retry-After"))
	if raw == "" {
		return 0, false
	}
	if secs, err := strconv.Atoi(raw); err == nil {
		if secs < 0 {
			secs = 0
		}
		return time.Duration(secs) * time.Second, true
	}
	if t, err := http.ParseTime(raw); err == nil {
		d := t.Sub(now)
		if d < 0 {
			d = 0
		}
		return d, true
	}
	return 0, false
}

func (e *Engine) sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (e *Engine) doHTTP(ctx context.Context, client *http.Client, req *http.Request, retries int, maxWait time.Duration, noRetry bool, rps float64) (roundTripOutcome, error) {
	e.hitMu.Lock()
	defer e.hitMu.Unlock()

	if rps > 0 {
		minGap := time.Duration(float64(time.Second) / rps)
		if !e.lastHitAt.IsZero() {
			wait := minGap - e.now().Sub(e.lastHitAt)
			if err := e.sleep(ctx, wait); err != nil {
				return roundTripOutcome{}, err
			}
		}
	}

	maxAttempts := 1
	if !noRetry && retries > 0 {
		maxAttempts = retries + 1
	}

	var (
		out    roundTripOutcome
		waited time.Duration
	)

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		out.Attempts = attempt
		cloned := req.Clone(ctx)
		if req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				return out, err
			}
			cloned.Body = body
		}

		e.lastHitAt = e.now()
		resp, err := client.Do(cloned)
		if err != nil {
			return out, err
		}
		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return out, readErr
		}

		out.Status = resp.StatusCode
		out.Header = resp.Header.Clone()
		out.Body = body

		retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable
		if !retryable || attempt == maxAttempts {
			if retryable && attempt == maxAttempts && resp.StatusCode == http.StatusTooManyRequests {
				out.LimitedOut = true
			}
			return out, nil
		}

		delay := time.Duration(attempt) * 200 * time.Millisecond
		if d, ok := parseRetryAfter(resp.Header, e.now()); ok {
			delay = d
		}
		if maxWait > 0 && waited+delay > maxWait {
			remain := maxWait - waited
			if remain < 0 {
				remain = 0
			}
			if err := e.sleep(ctx, remain); err != nil {
				return out, err
			}
			waited += remain
			if resp.StatusCode == http.StatusTooManyRequests {
				out.LimitedOut = true
			}
			return out, nil
		}
		if err := e.sleep(ctx, delay); err != nil {
			return out, err
		}
		waited += delay
		if maxWait > 0 && waited >= maxWait {
			if resp.StatusCode == http.StatusTooManyRequests {
				out.LimitedOut = true
			}
			return out, nil
		}
	}
	return out, nil
}

func headerMap(h http.Header) map[string]string {
	if h == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(h))
	for k, vals := range h {
		out[k] = strings.Join(vals, ", ")
	}
	return out
}

func classifyHTTP(status int, rateLimited bool) ExitCode {
	if rateLimited {
		return ExitRateLimited
	}
	switch {
	case status >= 200 && status < 400:
		return ExitSuccess
	case status >= 400 && status < 500:
		return ExitHTTP4xx
	case status >= 500:
		return ExitHTTP5xx
	default:
		return ExitTransport
	}
}

func truncateBody(b []byte, max int64) ([]byte, bool) {
	if max <= 0 || int64(len(b)) <= max {
		return b, false
	}
	return b[:max], true
}
