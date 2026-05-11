package probe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProbeOne_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	client := newClient()
	r := ProbeOne(context.Background(), client, srv.URL, 2*time.Second)
	if !r.OK() {
		t.Fatalf("expected ok, got %#v", r)
	}
}

func TestProbeOne_StatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	client := newClient()
	r := ProbeOne(context.Background(), client, srv.URL, 2*time.Second)
	if r.OK() {
		t.Fatalf("expected failure")
	}
	if r.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status: %d", r.StatusCode)
	}
}

func TestAnyFail(t *testing.T) {
	srvOK := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srvOK.Close)
	srvBad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(srvBad.Close)

	rs := ProbeAll(context.Background(), []string{srvOK.URL, srvBad.URL}, 2*time.Second)
	if !AnyFail(rs) {
		t.Fatal("expected any_fail")
	}
	rs2 := ProbeAll(context.Background(), []string{srvOK.URL}, 2*time.Second)
	if AnyFail(rs2) {
		t.Fatal("expected all ok")
	}
}

func TestAnyFail_Empty(t *testing.T) {
	if !AnyFail(nil) {
		t.Fatal("empty should fail policy")
	}
}

func TestAllFail(t *testing.T) {
	srvOK := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srvOK.Close)
	srvBad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(srvBad.Close)

	ctx := context.Background()
	client := newClient()
	ok := ProbeOne(ctx, client, srvOK.URL, 2*time.Second)
	bad := ProbeOne(ctx, client, srvBad.URL, 2*time.Second)

	if AllFail([]Result{ok, bad}) {
		t.Fatal("expected not all fail")
	}
	if !AllFail([]Result{bad, bad}) {
		t.Fatal("expected all fail")
	}
	if AllFail([]Result{ok}) {
		t.Fatal("single ok should not be all fail")
	}
}

func TestAllFail_Empty(t *testing.T) {
	if !AllFail(nil) {
		t.Fatal("empty should match any_fail empty semantics")
	}
}

func TestProbeAll_NoCacheHeaders(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Cache-Control")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	_ = ProbeAll(context.Background(), []string{srv.URL}, 2*time.Second)
	if got == "" {
		t.Fatal("expected Cache-Control header")
	}
}
