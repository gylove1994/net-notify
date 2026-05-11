package probe

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultURLs are used when no URLs are configured.
var DefaultURLs = []string{
	"https://www.google.com",
	"https://www.github.com",
	"https://www.baidu.com",
}

// Result holds the outcome of probing a single URL.
type Result struct {
	URL        string
	StatusCode int
	Duration   time.Duration
	Err        error
}

// OK reports success: no transport error and HTTP 2xx.
func (r Result) OK() bool {
	return r.Err == nil && r.StatusCode >= 200 && r.StatusCode < 300
}

func newClient() *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.DisableKeepAlives = true
	return &http.Client{Transport: tr}
}

// ProbeOne performs a single GET with no-cache headers and per-URL deadline.
func ProbeOne(parent context.Context, client *http.Client, url string, perURLTimeout time.Duration) Result {
	ctx, cancel := context.WithTimeout(parent, perURLTimeout)
	defer cancel()

	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Result{URL: url, Duration: time.Since(start), Err: err}
	}
	req.Header.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Expires", "0")

	resp, err := client.Do(req)
	dur := time.Since(start)
	if err != nil {
		return Result{URL: url, Duration: dur, Err: err}
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	return Result{URL: url, StatusCode: resp.StatusCode, Duration: dur, Err: nil}
}

// ProbeAll probes each URL sequentially (independent connections).
func ProbeAll(parent context.Context, urls []string, perURLTimeout time.Duration) []Result {
	client := newClient()
	out := make([]Result, 0, len(urls))
	for _, u := range urls {
		out = append(out, ProbeOne(parent, client, u, perURLTimeout))
	}
	return out
}

// AnyFail is true if any result is not OK (any_fail policy).
func AnyFail(results []Result) bool {
	for _, r := range results {
		if !r.OK() {
			return true
		}
	}
	return len(results) == 0
}

// FormatReport builds a human-readable probe report for notifications or logs.
func FormatReport(results []Result) string {
	var failed, ok strings.Builder
	for _, r := range results {
		line := formatLine(r)
		if r.OK() {
			ok.WriteString(line)
			ok.WriteByte('\n')
		} else {
			failed.WriteString(line)
			failed.WriteByte('\n')
		}
	}
	var b strings.Builder
	if failed.Len() > 0 {
		b.WriteString("失败:\n")
		b.WriteString(failed.String())
	}
	if ok.Len() > 0 {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("成功:\n")
		b.WriteString(ok.String())
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatLine(r Result) string {
	if r.Err != nil {
		return fmt.Sprintf("- %s | %s | %s", r.URL, r.Duration.Round(time.Millisecond), classifyErr(r.Err))
	}
	if !r.OK() {
		return fmt.Sprintf("- %s | HTTP %d | %s", r.URL, r.StatusCode, r.Duration.Round(time.Millisecond))
	}
	return fmt.Sprintf("- %s | HTTP %d | %s", r.URL, r.StatusCode, r.Duration.Round(time.Millisecond))
}

func classifyErr(err error) string {
	if err == nil {
		return "ok"
	}
	s := err.Error()
	switch {
	case strings.Contains(s, "context deadline exceeded"):
		return "timeout: " + s
	case strings.Contains(s, "no such host"), strings.Contains(s, "lookup "):
		return "dns: " + s
	case strings.Contains(s, "certificate"), strings.Contains(s, "tls: "):
		return "tls: " + s
	case strings.Contains(s, "connection refused"):
		return "connect: " + s
	default:
		return "error: " + s
	}
}
