package policy

import (
	"testing"

	"github.com/gylove1994/net-notify/internal/probe"
)

func TestParseWhen(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want When
	}{
		{"", WhenAnyFail},
		{"any_fail", WhenAnyFail},
		{"any-fail", WhenAnyFail},
		{"ALL_FAIL", WhenAllFail},
		{"all-fail", WhenAllFail},
	} {
		w, err := ParseWhen(tc.in)
		if err != nil {
			t.Fatalf("%q: %v", tc.in, err)
		}
		if w != tc.want {
			t.Fatalf("%q: got %v want %v", tc.in, w, tc.want)
		}
	}
	if _, err := ParseWhen("bogus"); err == nil {
		t.Fatal("expected error")
	}
}

func TestBuiltinDefaultLayout(t *testing.T) {
	layout := BuiltinDefaultLayout()
	want := probe.DefaultURLs
	if len(layout.FlatURLs) < 3 || len(want) < 3 {
		t.Fatal("expected at least 3 default URLs")
	}
	for i := range 3 {
		if layout.FlatURLs[i] != want[i] {
			t.Fatalf("flat[%d]: got %q want %q", i, layout.FlatURLs[i], want[i])
		}
	}
	if len(layout.Groups) != 2 {
		t.Fatalf("groups: %+v", layout.Groups)
	}
	if layout.Groups[0].Name != "Google+GitHub" || layout.Groups[1].Name != "百度" {
		t.Fatalf("names: %+v", layout.Groups)
	}
	if layout.Groups[0].When != WhenAllFail || layout.Groups[1].When != WhenAnyFail {
		t.Fatalf("unexpected when: %+v", layout.Groups)
	}
	r := func(url string, code int) probe.Result {
		return probe.Result{URL: url, StatusCode: code}
	}
	alert, names := Evaluate(layout, []probe.Result{r(want[0], 503), r(want[1], 200), r(want[2], 200)})
	if alert {
		t.Fatalf("single foreign fail should not alert: %v", names)
	}
	alert, names = Evaluate(layout, []probe.Result{r(want[0], 503), r(want[1], 503), r(want[2], 200)})
	if !alert || len(names) != 1 || names[0] != "Google+GitHub" {
		t.Fatalf("both foreign bad: got %v %v", alert, names)
	}
	alert, names = Evaluate(layout, []probe.Result{r(want[0], 200), r(want[1], 200), r(want[2], 503)})
	if !alert || len(names) != 1 || names[0] != "百度" {
		t.Fatalf("baidu bad: got %v %v", alert, names)
	}
}

func TestEvaluate_AnyOfGroups(t *testing.T) {
	u1, u2, u3 := "http://a", "http://b", "http://c"
	layout := Layout{
		FlatURLs: []string{u1, u2, u3},
		Groups: []Group{
			{Name: "G1", When: WhenAllFail, URLs: []string{u1, u2}},
			{Name: "G2", When: WhenAnyFail, URLs: []string{u3}},
		},
	}
	ok := func(u string) probe.Result {
		return probe.Result{URL: u, StatusCode: 200}
	}
	bad := func(u string) probe.Result {
		return probe.Result{URL: u, StatusCode: 502}
	}
	// G1: one bad one ok -> all_fail false; G2: ok -> any_fail false
	alert, names := Evaluate(layout, []probe.Result{bad(u1), ok(u2), ok(u3)})
	if alert {
		t.Fatalf("unexpected alert: %v", names)
	}
	// G1 all fail, G2 ok
	alert, names = Evaluate(layout, []probe.Result{bad(u1), bad(u2), ok(u3)})
	if !alert || len(names) != 1 || names[0] != "G1" {
		t.Fatalf("want G1 only, got %v %v", alert, names)
	}
	// G2 any fail
	alert, names = Evaluate(layout, []probe.Result{ok(u1), ok(u2), bad(u3)})
	if !alert || len(names) != 1 || names[0] != "G2" {
		t.Fatalf("want G2 only, got %v %v", alert, names)
	}
}

func TestEvaluate_SharedURL(t *testing.T) {
	u := "http://shared"
	layout := Layout{
		FlatURLs: []string{u},
		Groups: []Group{
			{Name: "A", When: WhenAllFail, URLs: []string{u}},
			{Name: "B", When: WhenAnyFail, URLs: []string{u}},
		},
	}
	bad := probe.Result{URL: u, StatusCode: 503}
	alert, _ := Evaluate(layout, []probe.Result{bad})
	if !alert {
		t.Fatal("both groups should fire on single bad URL")
	}
}

func TestEvaluate_BadResultLength(t *testing.T) {
	layout := Layout{FlatURLs: []string{"http://x"}, Groups: []Group{{Name: "g", When: WhenAnyFail, URLs: []string{"http://x"}}}}
	a, n := Evaluate(layout, nil)
	if a || len(n) > 0 {
		t.Fatal("mismatched length should not alert")
	}
	out, alert := EvaluateGroups(layout, nil)
	if alert || out != nil {
		t.Fatal("EvaluateGroups: mismatched length should return nil, false")
	}
}

func TestEvaluateGroups_ResultOrderMatchesGroupURLs(t *testing.T) {
	u1, u2 := "http://first", "http://second"
	layout := Layout{
		FlatURLs: []string{u1, u2},
		Groups: []Group{
			{Name: "rev", When: WhenAnyFail, URLs: []string{u2, u1}},
		},
	}
	r1 := probe.Result{URL: u1, StatusCode: 200}
	r2 := probe.Result{URL: u2, StatusCode: 503}
	// Flat probe order u1 then u2
	out, alert := EvaluateGroups(layout, []probe.Result{r1, r2})
	if !alert || len(out) != 1 {
		t.Fatalf("got %+v alert=%v", out, alert)
	}
	if len(out[0].Results) != 2 || out[0].Results[0].URL != u2 || out[0].Results[1].URL != u1 {
		t.Fatalf("results order: %+v", out[0].Results)
	}
}

func TestEvaluateGroups_AllOutcomesPresentWhenQuiet(t *testing.T) {
	u1, u2 := "http://a", "http://b"
	layout := Layout{
		FlatURLs: []string{u1, u2},
		Groups: []Group{
			{Name: "allbad", When: WhenAllFail, URLs: []string{u1, u2}},
			{Name: "any", When: WhenAnyFail, URLs: []string{u1}},
		},
	}
	ok := func(u string) probe.Result { return probe.Result{URL: u, StatusCode: 200} }
	out, alert := EvaluateGroups(layout, []probe.Result{ok(u1), ok(u2)})
	if alert {
		t.Fatal("expected no alert")
	}
	if len(out) != 2 || out[0].Fires || out[1].Fires {
		t.Fatalf("expected two non-firing outcomes: %+v", out)
	}
}
