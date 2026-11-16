package infra

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"runtime"
)

// DebugAddrEnv turns on the debug server for measuring Phonon: PHONON_DEBUG_ADDR=127.0.0.1:6061.
// It is off when unset and only listens on a loopback address.
const DebugAddrEnv = "PHONON_DEBUG_ADDR"

// debugHooks are what the debug server can trigger, the same as the menu bar items.
type debugHooks struct {
	StartRecording func()
	StopRecording  func()
}

// debugListenAddr is the address to serve on, or "" when the debug server stays off.
func debugListenAddr(v string) (string, error) {
	if v == "" {
		return "", nil
	}
	host, _, err := net.SplitHostPort(v)
	if err != nil {
		return "", fmt.Errorf("%s=%q: %w", DebugAddrEnv, v, err)
	}
	if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return "", fmt.Errorf("%s=%q: only a loopback address is allowed", DebugAddrEnv, v)
	}
	return v, nil
}

// debugMux serves pprof, the Go memory numbers and recording start and stop.
func debugMux(h debugHooks) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	mux.HandleFunc("/debug/mem", func(w http.ResponseWriter, _ *http.Request) {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		_ = json.NewEncoder(w).Encode(map[string]uint64{
			"heapAlloc": m.HeapAlloc, "heapInuse": m.HeapInuse, "heapIdle": m.HeapIdle,
			"heapReleased": m.HeapReleased, "sys": m.Sys, "numGC": uint64(m.NumGC),
		})
	})
	action := func(fn func()) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "POST only", http.StatusMethodNotAllowed)
				return
			}
			if fn != nil {
				go fn()
			}
			w.WriteHeader(http.StatusAccepted)
		}
	}
	mux.HandleFunc("/debug/record/start", action(h.StartRecording))
	mux.HandleFunc("/debug/record/stop", action(h.StopRecording))
	return mux
}

// startDebugServer serves debugMux when PHONON_DEBUG_ADDR names a loopback address.
func startDebugServer(h debugHooks) {
	addr, err := debugListenAddr(os.Getenv(DebugAddrEnv))
	if err != nil {
		log.Printf("debug server: %v", err)
		return
	}
	if addr == "" {
		return
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Printf("debug server: %v", err)
		return
	}
	log.Printf("debug server on %s", ln.Addr())
	go func() { _ = http.Serve(ln, debugMux(h)) }()
}
