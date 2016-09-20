package pprof

import (
	// expvar init routine adds the "/debug/vars" handler
	_ "expvar"
	"net/http"
	"net/http/pprof"
)

// New returns a new handler serving pprof information
func New() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/pprof/block", pprof.Handler("block"))
	mux.Handle("/pprof/heap", pprof.Handler("heap"))
	mux.Handle("/pprof/goroutine", pprof.Handler("goroutine"))
	mux.Handle("/pprof/threadcreate", pprof.Handler("threadcreate"))
	return mux
}
