// Copyright 2012-2016 Apcera Inc. All rights reserved.

package server

import (
	"os"
	"os/signal"
)

// Signal Handling
func (s *Server) handleSignals() {
	if s.opts.NoSigs {
		return
	}
	c := make(chan os.Signal, 1)

	signal.Notify(c, os.Interrupt)

	go func() {
		for sig := range c {
			Debugf("Trapped %q signal", sig)
			Noticef("Server Exiting..")
			os.Exit(0)
		}
	}()
}
