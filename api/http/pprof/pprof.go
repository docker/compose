package pprof

import (
	"expvar"
	"fmt"
	"net/http"
	"net/http/pprof"

	"github.com/Sirupsen/logrus"
)

func Enable(address string) {
	http.Handle("/", http.RedirectHandler("/debug/pprof", http.StatusMovedPermanently))

	http.Handle("/debug/vars", http.HandlerFunc(expVars))
	http.Handle("/debug/pprof/", http.HandlerFunc(pprof.Index))
	http.Handle("/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
	http.Handle("/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
	http.Handle("/debug/pprof/symbol", http.HandlerFunc(pprof.Symbol))
	http.Handle("/debug/pprof/block", pprof.Handler("block"))
	http.Handle("/debug/pprof/heap", pprof.Handler("heap"))
	http.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
	http.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))

	go http.ListenAndServe(address, nil)
	logrus.Debug("pprof listening in address %s", address)
}

// Replicated from expvar.go as not public.
func expVars(w http.ResponseWriter, r *http.Request) {
	first := true
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	fmt.Fprintf(w, "{\n")
	expvar.Do(func(kv expvar.KeyValue) {
		if !first {
			fmt.Fprintf(w, ",\n")
		}
		first = false
		fmt.Fprintf(w, "%q: %s", kv.Key, kv.Value)
	})
	fmt.Fprintf(w, "\n}\n")
}
