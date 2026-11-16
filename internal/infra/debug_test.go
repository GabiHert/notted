package infra

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDebugListenAddrIsOffByDefaultAndLoopbackOnly(t *testing.T) {
	for v, want := range map[string]string{"": "", "127.0.0.1:6061": "127.0.0.1:6061", "localhost:6061": "localhost:6061", "[::1]:6061": "[::1]:6061"} {
		got, err := debugListenAddr(v)
		if err != nil || got != want {
			t.Errorf("debugListenAddr(%q) = %q, %v; want %q", v, got, err, want)
		}
	}
	for _, v := range []string{"0.0.0.0:6061", ":6061", "192.168.1.2:6061", "example.com:6061", "6061"} {
		if got, err := debugListenAddr(v); err == nil {
			t.Errorf("debugListenAddr(%q) = %q, want an error", v, got)
		}
	}
}

func TestDebugMuxStartsAndStopsRecordingOnPOSTOnly(t *testing.T) {
	started, stopped := make(chan struct{}, 1), make(chan struct{}, 1)
	mux := debugMux(debugHooks{StartRecording: func() { started <- struct{}{} }, StopRecording: func() { stopped <- struct{}{} }})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/debug/record/start", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET start = %d, want 405", rec.Code)
	}
	for path, ch := range map[string]chan struct{}{"/debug/record/start": started, "/debug/record/stop": stopped} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, nil))
		if rec.Code != http.StatusAccepted {
			t.Fatalf("POST %s = %d, want 202", path, rec.Code)
		}
		select {
		case <-ch:
		case <-time.After(time.Second):
			t.Fatalf("POST %s did not run its hook", path)
		}
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/debug/mem", nil))
	if rec.Code != http.StatusOK || rec.Body.Len() == 0 {
		t.Fatalf("GET /debug/mem = %d %q", rec.Code, rec.Body.String())
	}
}
