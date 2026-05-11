package policy

import (
	"fmt"
	"strings"

	"github.com/gylove1994/net-notify/internal/probe"
)

// When selects how a group's probe results contribute to alerting.
type When int

const (
	WhenAnyFail When = iota
	WhenAllFail
)

// ParseWhen maps JSON / CLI strings to When. Empty string means any_fail.
func ParseWhen(s string) (When, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "any_fail", "any-fail":
		return WhenAnyFail, nil
	case "all_fail", "all-fail":
		return WhenAllFail, nil
	default:
		return 0, fmt.Errorf("must be any_fail or all_fail, got %q", s)
	}
}

// Group is a named set of URLs and a notify policy.
type Group struct {
	Name string
	When When
	URLs []string
}

// Layout defines probe order and grouping.
type Layout struct {
	FlatURLs []string
	Groups   []Group
}

// SingleGroup builds a layout with one implicit group covering all URLs.
func SingleGroup(urls []string, when When) Layout {
	return Layout{
		FlatURLs: append([]string(nil), urls...),
		Groups: []Group{
			{Name: "default", When: when, URLs: append([]string(nil), urls...)},
		},
	}
}

// BuiltinDefaultLayout matches the built-in URL list: Google + GitHub (all must fail) and Baidu (any failure).
// If probe.DefaultURLs has fewer than three entries, falls back to a single any_fail group.
func BuiltinDefaultLayout() Layout {
	u := probe.DefaultURLs
	if len(u) < 3 {
		return SingleGroup(append([]string(nil), u...), WhenAnyFail)
	}
	flat := append([]string(nil), u[:3]...)
	return Layout{
		FlatURLs: flat,
		Groups: []Group{
			{Name: "Google+GitHub", When: WhenAllFail, URLs: []string{u[0], u[1]}},
			{Name: "百度", When: WhenAnyFail, URLs: []string{u[2]}},
		},
	}
}

// Evaluate runs each group's policy; alerting is true if any group fires.
func Evaluate(layout Layout, results []probe.Result) (alerting bool, triggered []string) {
	if len(results) != len(layout.FlatURLs) {
		return false, nil
	}
	byURL := make(map[string]probe.Result, len(layout.FlatURLs))
	for i, u := range layout.FlatURLs {
		byURL[u] = results[i]
	}
	for _, g := range layout.Groups {
		sub := make([]probe.Result, 0, len(g.URLs))
		for _, u := range g.URLs {
			sub = append(sub, byURL[u])
		}
		var fires bool
		switch g.When {
		case WhenAnyFail:
			fires = probe.AnyFail(sub)
		case WhenAllFail:
			fires = probe.AllFail(sub)
		default:
			fires = probe.AnyFail(sub)
		}
		if fires {
			alerting = true
			triggered = append(triggered, g.Name)
		}
	}
	return alerting, triggered
}
