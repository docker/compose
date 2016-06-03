package pprof

import (
	// expvar init routine adds the "/debug/vars" handler
	_ "expvar"
	"net/http"
	"net/http/pprof"

	"github.com/Sirupsen/logrus"
)

// Enable registers the "/debug/pprof" handler
func Enable(address string) {
	http.Handle("/", http.RedirectHandler("/debug/pprof", http.StatusMovedPermanently))

	http.Handle("/debug/pprof/block", pprof.Handler("block"))
	http.Handle("/debug/pprof/heap", pprof.Handler("heap"))
	http.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
	http.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))

	go http.ListenAndServe(address, nil)
	logrus.Debug("pprof listening in address %s", address)
}
